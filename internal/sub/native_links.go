package sub

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"maps"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/amneziawg"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"

	"github.com/SawaMEN/3x-ui/v3/internal/tuic"

	"github.com/goccy/go-json"

	wgutil "github.com/SawaMEN/3x-ui/v3/internal/util/wireguard"
)

func (s *SubService) genVKTurnProxyLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.VKTurnProxy {
		return ""
	}
	clients, err := s.inboundService.GetClients(inbound)
	if err != nil {
		return ""
	}
	for _, client := range clients {
		if client.Email != email {
			continue
		}
		endpoints := s.shareEndpointsForInbound(inbound)
		links := make([]string, 0, len(endpoints))
		for _, endpoint := range endpoints {
			var (
				link string
				err  error
			)
			// Keep the legacy exporter for the default endpoint: it resolves a
			// usable IPv4 address from the inbound/request context. Only Host or
			// externalProxy endpoints use the explicit endpoint override.
			if endpoint.ep == nil {
				link, err = s.inboundService.ExportVKTurnProxyClient(inbound.Id, client.ID, s.address)
			} else {
				link, err = s.inboundService.ExportVKTurnProxyClientForEndpoint(inbound.Id, client.ID, endpoint.Address, endpoint.Port)
			}
			if err != nil {
				continue
			}
			links = append(links, link)
		}
		return strings.Join(links, "\n")
	}
	return ""
}

// genNaiveLink builds the canonical NaïveProxy client link for the panel's
// single-link/QR views. The same native URI is used in raw subscriptions.
func (s *SubService) genAnyTlsLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.AnyTLS {
		return ""
	}
	client, ok := s.clientForLink(inbound, email)
	if !ok || client.Password == "" {
		return ""
	}

	settings := s.linkSettings(inbound)
	endpoints := s.naiveShareEndpoints(inbound)
	links := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		forceTLS, _ := ep["forceTls"].(string)
		if strings.EqualFold(strings.TrimSpace(forceTLS), "none") {
			continue
		}
		address, _ := ep["dest"].(string)
		address = strings.TrimSpace(address)
		if address == "" {
			address = s.resolveInboundAddress(inbound)
		}
		port := inbound.Port
		if rawPort, ok := ep["port"].(float64); ok && int(rawPort) > 0 {
			port = int(rawPort)
		} else if rawPort, ok := ep["port"].(int); ok && rawPort > 0 {
			port = rawPort
		}

		params := map[string]string{}
		if sni, ok := externalProxySNI(ep); ok {
			params["sni"] = sni
		}
		if _, ok := params["sni"]; !ok {
			if tls, ok := settings["tls"].(map[string]any); ok {
				if serverName, ok := tls["serverName"].(string); ok && strings.TrimSpace(serverName) != "" {
					params["sni"] = strings.TrimSpace(serverName)
				}
			}
		}
		if host := strings.TrimSpace(address); host != "" && !strings.Contains(host, ":") && net.ParseIP(host) == nil {
			if _, exists := params["sni"]; !exists {
				params["sni"] = host
			}
		}

		if isHostEndpoint(ep) {
			s.renderHostRemark(inbound, client, ep, "")
		}
		remark := s.endpointRemark(inbound, email, ep, "")
		link := fmt.Sprintf("anytls://%s@%s/", encodeUserinfo(client.Password), joinHostPort(address, port))
		links = append(links, buildLinkWithParams(link, params, remark))
	}
	return strings.Join(links, "\n")
}

func (s *SubService) genNaiveLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.NaiveProxy {
		return ""
	}
	return s.genNaiveSubscriptionLink(inbound, email)
}

func (s *SubService) genShadowTlsLink(inbound *model.Inbound, email string) string {
	// There is no standalone share URI for a ShadowTLS transport plus its
	// inner destination protocol. The sing-box subscription emits both layers.
	return ""
}

// genNaiveSubscriptionLink returns Naïve's compatible URI form used by the raw
// subscription. Hiddify's ray2sing parser recognizes naive://; the quic=1
// parameter selects HTTP/3 transport.
func (s *SubService) genNaiveSubscriptionLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.NaiveProxy {
		return ""
	}
	client, ok := s.clientForLink(inbound, email)
	if !ok || client.Password == "" {
		return ""
	}

	settings := s.linkSettings(inbound)
	network, _ := settings["network"].(string)
	linkFormat, _ := settings["shareLinkFormat"].(string)
	useHiddifyFormat := strings.EqualFold(strings.TrimSpace(linkFormat), "hiddify")
	isQUIC := strings.EqualFold(strings.TrimSpace(network), "udp")
	scheme := "naive+https"
	if isQUIC {
		scheme = "naive+quic"
	}
	if useHiddifyFormat {
		scheme = "naive"
	}

	links := make([]string, 0)
	for _, ep := range s.naiveShareEndpoints(inbound) {
		forceTLS, _ := ep["forceTls"].(string)
		if strings.EqualFold(strings.TrimSpace(forceTLS), "none") {
			// NaiveProxy is TLS-only; a host explicitly forcing plaintext cannot
			// represent this protocol and must not receive a misleading link.
			continue
		}

		if isHostEndpoint(ep) {
			ep = maps.Clone(ep)
			s.renderHostRemark(inbound, client, ep, network)
		}

		address, _ := ep["dest"].(string)
		address = strings.TrimSpace(address)
		if address == "" {
			address = s.resolveInboundAddress(inbound)
		}
		port := inbound.Port
		if rawPort, ok := ep["port"].(float64); ok && int(rawPort) > 0 {
			port = int(rawPort)
		} else if rawPort, ok := ep["port"].(int); ok && rawPort > 0 {
			port = rawPort
		}

		params := map[string]string{}
		if useHiddifyFormat {
			// Hiddify's ray2sing parser uses naive:// and reads the TLS
			// hostname/transport from query parameters.
			if isQUIC {
				params["quic"] = "1"
				if cc, ok := settings["quicCongestionControl"].(string); ok && strings.TrimSpace(cc) != "" {
					params["quic_congestion_control"] = strings.TrimSpace(cc)
				}
			}
			if sni, ok := externalProxySNI(ep); ok {
				params["sni"] = sni
			}
			if _, ok := params["sni"]; !ok {
				if tls, ok := ep["tlsSettings"].(map[string]any); ok {
					if serverName, ok := tls["serverName"].(string); ok && strings.TrimSpace(serverName) != "" {
						params["sni"] = strings.TrimSpace(serverName)
					}
				}
			}
			if _, ok := params["sni"]; !ok {
				if tls, ok := settings["tls"].(map[string]any); ok {
					if serverName, ok := tls["serverName"].(string); ok && strings.TrimSpace(serverName) != "" {
						params["sni"] = strings.TrimSpace(serverName)
					}
				}
			}
			if _, ok := params["sni"]; !ok && !strings.Contains(address, ":") && net.ParseIP(strings.TrimSpace(address)) == nil {
				// Hiddify-compatible clients need the TLS SNI in the URI when it
				// is not carried by a standard URL scheme. When no custom SNI is
				// configured, the advertised DNS name is the natural default.
				params["sni"] = strings.TrimSpace(address)
			}
			// `host` is a Hiddify/ray2sing compatibility parameter. Standard
			// NaiveProxy URIs intentionally do not emit non-standard query fields.
			if host, ok := ep["hostHeader"].(string); ok && strings.TrimSpace(host) != "" {
				params["host"] = strings.TrimSpace(host)
			}
		}

		remark := s.endpointRemark(inbound, email, ep, network)
		if desc, _ := ep["serverDescription"].(string); desc != "" {
			remark = appendHappServerDescription(remark, desc)
		}
		link := fmt.Sprintf("%s://%s:%s@%s",
			scheme,
			encodeUserinfo(client.Email),
			encodeUserinfo(client.Password),
			joinHostPort(address, port),
		)
		links = append(links, buildLinkWithParams(link, params, remark))
	}
	return strings.Join(links, "\n")
}

func (s *SubService) genMieruLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.Mieru {
		return ""
	}
	client, ok := s.clientForLink(inbound, email)
	if !ok || client.Password == "" {
		return ""
	}

	settings := s.linkSettings(inbound)
	entries := mieruShareEntries(settings, inbound.Port)
	if len(entries) == 0 {
		return ""
	}
	multiplexing, _ := settings["multiplexing"].(string)
	if multiplexing == "" {
		multiplexing = "MULTIPLEXING_LOW"
	}
	handshakeMode, _ := settings["handshakeMode"].(string)
	if handshakeMode == "" {
		handshakeMode = "HANDSHAKE_STANDARD"
	}
	mtu := 1400
	if raw, ok := settings["mtu"].(float64); ok && int(raw) >= 1280 && int(raw) <= 1400 {
		mtu = int(raw)
	}

	endpoints := s.shareEndpointsForInbound(inbound)
	links := make([]string, 0, len(endpoints)*2)
	for _, endpoint := range endpoints {
		values := url.Values{}
		values.Set("profile", "default")
		values.Set("mtu", strconv.Itoa(mtu))
		values.Set("multiplexing", multiplexing)
		values.Set("handshake-mode", handshakeMode)
		// Hiddify/ray2sing reads handshakemode without the separator, while
		// native Mieru expects handshake-mode. Include both in the simple link.
		values.Set("handshakemode", handshakeMode)

		// A Host/externalProxy endpoint describes the public Mieru listener,
		// so its port replaces the server-side binding port(s). Keep the
		// configured transport protocol set but avoid advertising private
		// port ranges through a public front.
		publicEntries := entries
		if endpoint.ep != nil {
			publicEntries = make([]struct {
				port     string
				protocol string
			}, 0, len(entries))
			seenProtocols := make(map[string]struct{})
			for _, entry := range entries {
				if _, exists := seenProtocols[entry.protocol]; exists {
					continue
				}
				seenProtocols[entry.protocol] = struct{}{}
				values.Add("port", strconv.Itoa(endpoint.Port))
				values.Add("protocol", entry.protocol)
				publicEntries = append(publicEntries, struct {
					port     string
					protocol string
				}{port: strconv.Itoa(endpoint.Port), protocol: entry.protocol})
			}
		} else {
			for _, entry := range entries {
				values.Add("port", entry.port)
				values.Add("protocol", entry.protocol)
			}
		}

		// In the native mierus:// format the authority is only the server
		// address; every listener port belongs in the paired port/protocol
		// query values (including an externally forwarded port).
		host := formatShareHost(endpoint.Address)
		if full, err := mieruFullConfigLink(endpoint.Address, client.Email, client.Password, publicEntries, mtu, multiplexing, handshakeMode); err == nil {
			links = append(links, full)
		}
		link := fmt.Sprintf("mierus://%s:%s@%s?%s",
			encodeUserinfo(client.Email),
			encodeUserinfo(client.Password),
			host,
			values.Encode(),
		)
		links = append(links, link+"#"+strings.ReplaceAll(url.QueryEscape(s.endpointRemark(inbound, email, endpoint.ep, "")), "+", "%20"))
	}
	return strings.Join(links, "\n")
}

func (s *SubService) genTuicLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.TUIC {
		return ""
	}
	inst, ok := tuic.InstanceFromInbound(inbound)
	if !ok {
		return ""
	}
	var client *tuic.TuicClientSettings
	for _, c := range inst.Clients {
		if c.Email == email {
			client = &c
			break
		}
	}
	if client == nil && len(inst.Clients) > 0 && email == "" {
		client = &inst.Clients[0]
	}
	if client == nil || client.UUID == "" || client.Password == "" {
		return ""
	}

	params := make(map[string]string)
	cc := inst.CongestionControl
	if cc == "" {
		cc = "bbr"
	}
	params["congestion_control"] = cc

	if len(inst.ALPN) > 0 {
		params["alpn"] = strings.Join(inst.ALPN, ",")
	}
	if inst.SNI != "" {
		params["sni"] = inst.SNI
	}
	if inst.UDPRelayMode != "" {
		params["udp_relay_mode"] = inst.UDPRelayMode
	}
	params["allow_insecure"] = "0"

	stream := s.streamSettingsForInbound(inbound)
	externalProxies, _ := stream["externalProxy"].([]any)
	if len(externalProxies) > 0 {
		links := make([]string, 0, len(externalProxies))
		for _, externalProxy := range externalProxies {
			ep, ok := externalProxy.(map[string]any)
			if !ok {
				continue
			}
			forceTLS, _ := ep["forceTls"].(string)
			if strings.EqualFold(strings.TrimSpace(forceTLS), "none") {
				// Hysteria is TLS/QUIC only; a plaintext external endpoint
				// cannot be represented by a valid Hysteria URI.
				continue
			}
			dest, _ := ep["dest"].(string)
			portF, okPort := ep["port"].(float64)
			if strings.TrimSpace(dest) == "" {
				dest = s.resolveInboundAddress(inbound)
			}
			if !okPort || int(portF) <= 0 {
				portF = float64(inbound.Port)
			}
			epParams := cloneStringMap(params)
			if sni, ok := externalProxySNI(ep); ok {
				epParams["sni"] = sni
			}
			if alpn, ok := externalProxyALPN(ep["alpn"]); ok {
				epParams["alpn"] = alpn
			}
			if ai, ok := ep["allowInsecure"].(bool); ok && ai {
				epParams["allow_insecure"] = "1"
			}
			link := fmt.Sprintf("tuic://%s:%s@%s", encodeUserinfo(client.UUID), encodeUserinfo(client.Password), joinHostPort(dest, int(portF)))
			links = append(links, buildLinkWithParams(link, epParams, s.endpointRemark(inbound, email, ep, "")))
		}
		return strings.Join(links, "\n")
	}

	host := s.resolveInboundAddress(inbound)
	link := fmt.Sprintf("tuic://%s:%s@%s", encodeUserinfo(client.UUID), encodeUserinfo(client.Password), joinHostPort(host, inbound.Port))
	return buildLinkWithParams(link, params, s.genRemark(inbound, email, "", ""))
}

// genWireguardLink builds a per-client wireguard:// share link mirroring the
// frontend genWireguardLink: the client's private key is the userinfo, the
// server public key (derived from the inbound secretKey) and the client's
// tunnel address ride in the query. Returns "" when the client has no key.
func (s *SubService) genWireguardLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.WireGuard {
		return ""
	}
	settings := s.linkSettings(inbound)
	secretKey, _ := settings["secretKey"].(string)

	resolved, ok := s.clientForLink(inbound, email)
	if !ok || resolved.PrivateKey == "" {
		return ""
	}
	client := &resolved

	params := make(map[string]string)
	if secretKey != "" {
		if pub, err := wgutil.PublicKeyFromPrivate(secretKey); err == nil {
			params["publickey"] = pub
		}
	}
	if joined := strings.Join(client.AllowedIPs, ","); joined != "" {
		params["address"] = joined
	}
	if mtu, ok := settings["mtu"].(float64); ok && mtu > 0 {
		params["mtu"] = strconv.Itoa(int(mtu))
	}
	if dns, ok := settings["dns"].(string); ok && dns != "" {
		params["dns"] = dns
	}
	if client.PreSharedKey != "" {
		params["presharedkey"] = client.PreSharedKey
	}
	if ka := client.KeepAliveSeconds(); ka > 0 {
		params["keepalive"] = strconv.Itoa(ka)
	}

	endpoints := s.shareEndpointsForInbound(inbound)
	links := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		link := fmt.Sprintf("wireguard://%s@%s", encodeUserinfo(client.PrivateKey), joinHostPort(endpoint.Address, endpoint.Port))
		links = append(links, buildLinkWithParams(link, params, s.endpointRemark(inbound, email, endpoint.ep, "")))
	}
	return strings.Join(links, "\n")
}

// genAmneziaWGLink builds a per-client vpn://<base64url .conf text> share
// link matching the real AmneziaVPN app's own share-link scheme (see the
// frontend's genAmneziaWGLink for the confirmed import-path reasoning).
// Returns "" when the client or server has no key.
func (s *SubService) genAmneziaWGLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.AmneziaWG {
		return ""
	}
	var parsed amneziawg.InboundSettings
	if err := json.Unmarshal([]byte(inbound.Settings), &parsed); err != nil || parsed.Server == nil {
		return ""
	}
	server := parsed.Server

	resolved, ok := s.clientForLink(inbound, email)
	if !ok || resolved.PrivateKey == "" {
		return ""
	}
	client := &resolved

	endpoints := s.shareEndpointsForInbound(inbound)
	links := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		text := amneziaWGConfigText(server, client, endpoint.Address, endpoint.Port, s.endpointRemark(inbound, email, endpoint.ep, ""))
		if text == "" {
			continue
		}
		links = append(links, "vpn://"+base64.RawURLEncoding.EncodeToString([]byte(text)))
	}
	return strings.Join(links, "\n")
}

// genMtprotoLink builds one Telegram link per advertised endpoint with the client's FakeTLS secret.
// It omits remarks because lenient parsers fold a fragment into the last query value.
func (s *SubService) genMtprotoLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.MTProto {
		return ""
	}
	resolved, ok := s.clientForLink(inbound, email)
	if !ok || resolved.Secret == "" {
		return ""
	}
	endpoints := []ShareEndpoint{s.inboundDefaultEndpoint(inbound)}
	stream := s.streamSettingsForInbound(inbound)
	if externalProxies, ok := stream["externalProxy"].([]any); ok && len(externalProxies) > 0 {
		overrides := make([]ShareEndpoint, 0, len(externalProxies))
		for _, raw := range externalProxies {
			if ep, ok := raw.(map[string]any); ok {
				overrides = append(overrides, externalProxyToEndpoint(ep))
			}
		}
		if len(overrides) > 0 {
			fallback := s.inboundDefaultEndpoint(inbound)
			for i := range overrides {
				overrides[i] = normalizeShareEndpoint(overrides[i], fallback)
			}
			endpoints = overrides
		}
	}
	var mode struct {
		WebProxy struct {
			Enabled  bool     `json:"enabled"`
			Domain   string   `json:"domain"`
			BasePath string   `json:"basePath"`
			Aliases  []string `json:"aliases"`
		} `json:"webProxy"`
	}
	if json.Unmarshal([]byte(inbound.Settings), &mode) == nil && mode.WebProxy.Enabled {
		secret := strings.TrimSpace(resolved.Secret)
		if len(secret) >= 34 && (strings.HasPrefix(secret, "ee") || strings.HasPrefix(secret, "dd")) {
			secret = secret[2:34]
		}
		raw, err := hex.DecodeString(secret)
		if err != nil || len(raw) != 16 || strings.TrimSpace(mode.WebProxy.Domain) == "" {
			return ""
		}
		basePath := strings.Trim(strings.TrimSpace(mode.WebProxy.BasePath), "/")
		linkSecret := "dd" + secret
		if basePath != "" {
			linkSecret = base64.RawURLEncoding.EncodeToString(append([]byte{0x70, 0xdd}, raw...))
		}
		hosts := append([]string{mode.WebProxy.Domain}, mode.WebProxy.Aliases...)
		seenHosts := map[string]bool{}
		links := make([]string, 0, len(hosts))
		for _, host := range hosts {
			host = strings.ToLower(strings.TrimSpace(host))
			if host == "" || seenHosts[host] {
				continue
			}
			seenHosts[host] = true
			if basePath != "" {
				host += "/" + basePath
			}
			links = append(links, buildLinkWithParams("tg://webproxy", map[string]string{
				"server": host,
				"secret": linkSecret,
			}, ""))
		}
		return strings.Join(links, "\n")
	}
	links := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		links = append(links, buildLinkWithParams("tg://proxy", map[string]string{
			"server": endpoint.Address,
			"port":   fmt.Sprintf("%d", endpoint.Port),
			"secret": resolved.Secret,
		}, ""))
	}
	return strings.Join(links, "\n")
}
