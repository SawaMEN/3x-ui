package sub

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"

	"github.com/goccy/go-json"
)

// Protocol link generators are intentionally ordered as:
// vmess -> vless -> trojan -> shadowsocks -> hysteria.
func (s *SubService) genVmessLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.VMESS {
		return ""
	}
	address := s.resolveInboundAddress(inbound)
	obj := map[string]any{
		"v":    "2",
		"add":  address,
		"port": inbound.Port,
		"type": "none",
	}
	stream := s.streamSettingsForInbound(inbound)
	network, _ := stream["network"].(string)
	applyVmessNetworkParams(stream, network, obj)
	if finalmask, ok := stream["finalmask"].(map[string]any); ok {
		applyFinalMaskObj(finalmask, obj)
	}
	security, _ := stream["security"].(string)
	obj["tls"] = security
	if security == "tls" {
		applyVmessTLSParams(stream, obj)
	}

	client, ok := s.clientForLink(inbound, email)
	if !ok {
		return ""
	}
	obj["id"] = client.ID
	obj["scy"] = normalizeVmessSecurity(client.Security)

	externalProxies, _ := stream["externalProxy"].([]any)

	if len(externalProxies) > 0 {
		return s.buildVmessExternalProxyLinks(externalProxies, obj, inbound, email, network)
	}

	obj["ps"] = s.genRemark(inbound, email, "", network)
	return buildVmessLink(obj)
}

func (s *SubService) genVlessLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.VLESS {
		return ""
	}
	address := s.resolveInboundAddress(inbound)
	stream := s.streamSettingsForInbound(inbound)
	client, ok := s.clientForLink(inbound, email)
	if !ok {
		return ""
	}
	uuid := client.ID
	port := inbound.Port
	streamNetwork, _ := stream["network"].(string)
	params := make(map[string]string)
	params["type"] = streamNetwork

	// Add encryption parameter for VLESS from inbound settings
	settings := s.linkSettings(inbound)
	// Xray requires VLESS share links to carry an explicit encryption value.
	// Hiddify always emits "none" when the server does not use vlessenc.
	encryption := "none"
	if configured, ok := settings["encryption"].(string); ok && strings.TrimSpace(configured) != "" {
		encryption = strings.TrimSpace(configured)
	}
	params["encryption"] = encryption

	applyShareNetworkParams(stream, streamNetwork, params)
	if finalmask, ok := stream["finalmask"].(map[string]any); ok {
		applyFinalMaskParams(finalmask, params)
	}
	security, _ := stream["security"].(string)
	switch security {
	case "tls":
		applyShareTLSParams(stream, params)
	case "reality":
		applyShareRealityParams(stream, params, subKey(client))
		params["support-x25519mlkem768"] = "true"
	default:
		params["security"] = "none"
	}
	if len(client.Flow) > 0 && !inbound.DisableFlow && vlessFlowAllowed(streamNetwork, security, settings) {
		params["flow"] = client.Flow
	}

	externalProxies, _ := stream["externalProxy"].([]any)

	if len(externalProxies) > 0 {
		return s.buildExternalProxyURLLinks(
			externalProxies,
			params,
			security,
			s.inboundDefaultEndpoint(inbound),
			func(ep map[string]any, dest string, port int) string {
				return fmt.Sprintf("vless://%s@%s", applyVlessRoute(uuid, hostVlessRoute(ep)), joinHostPort(dest, port))
			},
			func(ep map[string]any) string {
				return s.endpointRemark(inbound, email, ep, streamNetwork)
			},
		)
	}

	link := fmt.Sprintf("vless://%s@%s", uuid, joinHostPort(address, port))
	return buildLinkWithParams(link, params, s.genRemark(inbound, email, "", streamNetwork))
}

func (s *SubService) genTrojanLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.Trojan {
		return ""
	}
	address := s.resolveInboundAddress(inbound)
	stream := s.streamSettingsForInbound(inbound)
	client, ok := s.clientForLink(inbound, email)
	if !ok {
		return ""
	}
	password := encodeUserinfo(client.Password)
	port := inbound.Port
	streamNetwork, _ := stream["network"].(string)
	params := make(map[string]string)
	params["type"] = streamNetwork

	applyShareNetworkParams(stream, streamNetwork, params)
	if finalmask, ok := stream["finalmask"].(map[string]any); ok {
		applyFinalMaskParams(finalmask, params)
	}
	security, _ := stream["security"].(string)
	switch security {
	case "tls":
		applyShareTLSParams(stream, params)
	case "reality":
		applyShareRealityParams(stream, params, subKey(client))
		if streamNetwork == "tcp" && len(client.Flow) > 0 && !inbound.DisableFlow {
			params["flow"] = client.Flow
		}
	default:
		params["security"] = "none"
	}

	externalProxies, _ := stream["externalProxy"].([]any)

	if len(externalProxies) > 0 {
		return s.buildExternalProxyURLLinks(
			externalProxies,
			params,
			security,
			s.inboundDefaultEndpoint(inbound),
			func(_ map[string]any, dest string, port int) string {
				return fmt.Sprintf("trojan://%s@%s", password, joinHostPort(dest, port))
			},
			func(ep map[string]any) string {
				return s.endpointRemark(inbound, email, ep, streamNetwork)
			},
		)
	}

	link := fmt.Sprintf("trojan://%s@%s", password, joinHostPort(address, port))
	return buildLinkWithParams(link, params, s.genRemark(inbound, email, "", streamNetwork))
}

func (s *SubService) genShadowsocksLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.Shadowsocks {
		return ""
	}
	address := s.resolveInboundAddress(inbound)
	stream := s.streamSettingsForInbound(inbound)
	client, ok := s.clientForLink(inbound, email)
	if !ok {
		return ""
	}

	settings := s.linkSettings(inbound)
	inboundPassword, _ := settings["password"].(string)
	method, _ := settings["method"].(string)
	streamNetwork, _ := stream["network"].(string)
	params := make(map[string]string)
	params["type"] = streamNetwork

	applyShareNetworkParams(stream, streamNetwork, params)
	if finalmask, ok := stream["finalmask"].(map[string]any); ok {
		applyFinalMaskParams(finalmask, params)
	}

	security, _ := stream["security"].(string)
	if security == "tls" {
		applyShareTLSParams(stream, params)
	}

	// SIP002 clients (v2rayN) ignore the xray-native type/headerType/host/path
	// params and only read `plugin`. Re-encode a TCP http header as obfs-local so
	// they build a matching tcp/http outbound (v2rayN forces request path "/").
	if streamNetwork == "tcp" && params["headerType"] == "http" {
		host := params["host"]
		delete(params, "type")
		delete(params, "headerType")
		delete(params, "host")
		delete(params, "path")
		params["plugin"] = "obfs-local;obfs=http;obfs-host=" + host
	}

	// SIP002 userinfo is base64(method:password). For SIP022 (2022-blake3-*) the
	// userinfo MUST NOT be base64-encoded; method and password are percent-encoded.
	var userInfo string
	if strings.HasPrefix(method, "2022") {
		userInfo = fmt.Sprintf("%s:%s:%s",
			url.QueryEscape(method),
			url.QueryEscape(inboundPassword),
			url.QueryEscape(client.Password))
	} else {
		userInfo = base64.RawURLEncoding.EncodeToString(fmt.Appendf(nil, "%s:%s", method, client.Password))
	}

	externalProxies, _ := stream["externalProxy"].([]any)

	if len(externalProxies) > 0 {
		proxyParams := cloneStringMap(params)
		proxyParams["security"] = security
		return s.buildExternalProxyURLLinks(
			externalProxies,
			proxyParams,
			security,
			s.inboundDefaultEndpoint(inbound),
			func(_ map[string]any, dest string, port int) string {
				return fmt.Sprintf("ss://%s@%s", userInfo, joinHostPort(dest, port))
			},
			func(ep map[string]any) string {
				return s.endpointRemark(inbound, email, ep, streamNetwork)
			},
		)
	}

	link := fmt.Sprintf("ss://%s@%s", userInfo, joinHostPort(address, inbound.Port))
	return buildLinkWithParams(link, params, s.genRemark(inbound, email, "", streamNetwork))
}

func (s *SubService) genHysteriaLink(inbound *model.Inbound, email string) string {
	if inbound.Protocol != model.Hysteria {
		return ""
	}
	var stream map[string]any
	_ = json.Unmarshal([]byte(inbound.StreamSettings), &stream)
	client, ok := s.clientForLink(inbound, email)
	if !ok {
		return ""
	}
	auth := encodeUserinfo(client.Auth)
	params := make(map[string]string)

	params["security"] = "tls"
	tlsSetting, _ := stream["tlsSettings"].(map[string]any)
	alpns, _ := tlsSetting["alpn"].([]any)
	var alpn []string
	for _, a := range alpns {
		if s, ok := a.(string); ok {
			alpn = append(alpn, s)
		}
	}
	if len(alpn) > 0 {
		params["alpn"] = strings.Join(alpn, ",")
	}
	if sniValue, ok := searchKey(tlsSetting, "serverName"); ok {
		params["sni"], _ = sniValue.(string)
	}

	tlsSettings, _ := searchKey(tlsSetting, "settings")
	if tlsSetting != nil {
		if fpValue, ok := searchKey(tlsSettings, "fingerprint"); ok {
			params["fp"], _ = fpValue.(string)
		}
		if echValue, ok := searchKey(tlsSettings, "echConfigList"); ok {
			if ech, _ := echValue.(string); ech != "" {
				params["ech"] = ech
			}
		}
		if vcn, ok := verifyPeerCertByNameValue(tlsSettings); ok {
			params["vcn"] = vcn
		}
		if pins, ok := pinnedSha256List(tlsSettings); ok {
			for i, p := range pins {
				pins[i] = hysteriaPinHex(p)
			}
			params["pinSHA256"] = strings.Join(pins, ",")
		}
	}

	// salamander obfs (Hysteria2): standard URI fields only -- an fm=<json>
	// dump breaks strict clients. packetSize exports as v2rayN's gecko pair.
	if finalmask, ok := stream["finalmask"].(map[string]any); ok {
		if udpMasks, ok := finalmask["udp"].([]any); ok {
			for _, m := range udpMasks {
				mask, _ := m.(map[string]any)
				if mask == nil || mask["type"] != "salamander" {
					continue
				}
				settings, _ := mask["settings"].(map[string]any)
				if pw, ok := settings["password"].(string); ok && pw != "" {
					packetSize, _ := settings["packetSize"].(string)
					gecko := parseHysteriaPacketSize(packetSize)
					if gecko != "" {
						params["obfs"] = "gecko"
						params["minPacketSize"], params["maxPacketSize"] = splitHysteriaPacketSize(gecko)
					}
					// packetSize rides its own URI fields; anything else still
					// breaks standard clients and must warn even when gecko fires.
					if extra := extraSalamanderKeys(settings, gecko != ""); len(extra) > 0 {
						warningKey := fmt.Sprintf("%d:%v", inbound.Id, extra)
						if markSalamanderWarningSeen(warningKey) {
							logger.Warningf("SubService - inbound %d: salamander settings %v cannot be expressed in a hysteria2 URI; standard clients will fail the handshake", inbound.Id, extra)
						}
					}
					if params["obfs"] == "" {
						params["obfs"] = "salamander"
					}
					params["obfs-password"] = pw
					break
				}
			}
		}
	}

	settings := s.linkSettings(inbound)
	version, _ := settings["version"].(float64)
	protocol := "hysteria2"
	if int(version) == 1 {
		protocol = "hysteria"
	}
	for key, value := range hysteriaNativeURIOptions(stream, int(version)) {
		params[key] = value
	}

	// Set before the externalProxy fan-out: a Host overrides only the
	// address, so every endpoint inherits the inbound's UDP hop range.
	if hopPorts := hysteriaHopPorts(stream); hopPorts != "" {
		params["mport"] = hopPorts
	}

	// Fan out one link per External Proxy entry if any. Previously this
	// generator ignored `externalProxy` entirely, so the link kept the
	// server's own IP/port even when the admin configured an alternate
	// endpoint (e.g. a CDN hostname + port that forwards to the node).
	// Matches the behaviour of genVlessLink / genTrojanLink / ….
	externalProxies, _ := stream["externalProxy"].([]any)
	if len(externalProxies) > 0 {
		links := make([]string, 0, len(externalProxies))
		for _, externalProxy := range externalProxies {
			ep, ok := externalProxy.(map[string]any)
			if !ok {
				continue
			}
			if forceTLS, _ := ep["forceTls"].(string); strings.EqualFold(strings.TrimSpace(forceTLS), "none") {
				// Hysteria is TLS/QUIC-only; a plaintext external endpoint
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
			applyExternalProxyHysteriaParams(ep, epParams)

			link := fmt.Sprintf("%s://%s@%s", protocol, auth, joinHostPort(dest, int(portF)))
			links = append(links, buildLinkWithParams(link, epParams, s.endpointRemark(inbound, email, ep, "quic")))
		}
		return strings.Join(links, "\n")
	}

	// No external proxy configured — use the inbound's resolved address so
	// node-managed inbounds get the node's host instead of the central panel's.
	link := fmt.Sprintf("%s://%s@%s", protocol, auth, joinHostPort(s.resolveInboundAddress(inbound), inbound.Port))
	return buildLinkWithParams(link, params, s.genRemark(inbound, email, "", "quic"))
}
