package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

type TelemtWebIngress struct {
	Enabled    bool  `json:"enabled"`
	InboundIDs []int `json:"inboundIds,omitempty"`
	// Legacy single-inbound setting; read for migration only.
	NaiveInboundID int `json:"naiveInboundId,omitempty"`
}

const telemtIngressMaxInbounds = 64

func telemtIngressInboundIDs(o *TelemtWebIngress) ([]int, error) {
	if o == nil || !o.Enabled {
		return nil, nil
	}
	ids := slices.Clone(o.InboundIDs)
	if o.InboundIDs == nil && o.NaiveInboundID != 0 {
		ids = []int{o.NaiveInboundID}
	}
	if len(ids) > telemtIngressMaxInbounds {
		return nil, fmt.Errorf("shared ingress supports up to %d inbounds", telemtIngressMaxInbounds)
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, errors.New("shared ingress requires positive inbound IDs")
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

func telemtWebIngressEqual(a, b *TelemtWebIngress) bool {
	aEnabled, bEnabled := a != nil && a.Enabled, b != nil && b.Enabled
	if aEnabled != bEnabled {
		return false
	}
	x, errA := telemtIngressInboundIDs(a)
	y, errB := telemtIngressInboundIDs(b)
	return errA == nil && errB == nil && slices.Equal(x, y)
}

const telemtWebIngressDefaultPath = "telegram/web"

type telemtIngressProxyRoute struct {
	id                                     int
	host, address, publicAddress, security string
	protocol                               model.Protocol
}

type telemtIngressPublication struct {
	subscriptionHost string
	routes           map[int]telemtIngressProxyRoute
}

var publishedTelemtIngress atomic.Pointer[telemtIngressPublication]

func publishTelemtIngress(_ TelemtWebProxyState, plan telemtIngressPlan) {
	routes := make(map[int]telemtIngressProxyRoute, len(plan.proxies))
	for _, route := range plan.proxies {
		routes[route.id] = route
	}
	publishedTelemtIngress.Store(&telemtIngressPublication{
		subscriptionHost: plan.subscriptions.host,
		routes:           routes,
	})
}

func telemtIngressSubscriptionOrigin() string {
	if published := publishedTelemtIngress.Load(); published != nil && published.subscriptionHost != "" {
		return "https://" + published.subscriptionHost
	}
	return ""
}

func telemtIngressProxyHost(inbound *model.Inbound) *model.Host {
	published := publishedTelemtIngress.Load()
	if inbound == nil || !inbound.Enable || inbound.NodeID != nil || published == nil {
		return nil
	}
	route, ok := published.routes[inbound.Id]
	if !ok || route.protocol != inbound.Protocol {
		return nil
	}
	// Do not publish a stale route while an edited inbound awaits reconciliation.
	host, address, publicAddress, security, err := telemtIngressProxyEndpoint(inbound, route.publicAddress)
	if err != nil || host != route.host || address != route.address || publicAddress != route.publicAddress || security != route.security {
		return nil
	}
	entry := &model.Host{InboundId: inbound.Id, Remark: host, Address: publicAddress, Port: 443, Security: security, Sni: host}
	if inbound.Protocol == model.NaiveProxy {
		entry.HostHeader = host
	}
	return entry
}

const telemtWebIngressTLSSocket = "/run/3x-ui-telemt-front/haproxy-web.sock"

type telemtIngressRoute struct {
	host     string
	paths    []string
	address  string
	certFile string
	tlsName  string
}

type telemtIngressPlan struct {
	panel         telemtIngressRoute
	legacyPaths   []string
	subscriptions telemtIngressRoute
	proxies       []telemtIngressProxyRoute
}

var ingressPath = regexp.MustCompile(`^/[-A-Za-z0-9_.~/]+$`)

func appendIngressPaths(route *telemtIngressRoute, paths ...string) error {
	for _, path := range paths {
		if strings.Contains(path, "//") {
			return fmt.Errorf("shared ingress requires a safe non-root URL prefix: %q", path)
		}
		path = strings.TrimSuffix(path, "/")
		segments := strings.Split(path, "/")
		if !ingressPath.MatchString(path) || slices.Contains(segments, ".") || slices.Contains(segments, "..") {
			return fmt.Errorf("shared ingress requires a safe non-root URL prefix: %q", path)
		}
		if !slices.Contains(route.paths, path) {
			route.paths = append(route.paths, path)
		}
	}
	return nil
}

func configureTelemtIngressUpstream(route *telemtIngressRoute, listen string, port int, cert, key string) error {
	// Keep the operator's existing listeners reachable. For a wildcard bind
	// HAProxy can dial the same socket through loopback.
	switch listen {
	case "", "0.0.0.0":
		listen = "127.0.0.1"
	case "::":
		listen = "::1"
	}
	ip := net.ParseIP(listen)
	if ip == nil || !ip.IsLoopback() || port < 1 || port > 65535 || port == 443 {
		return fmt.Errorf("shared ingress backend must use a loopback IP and an internal port: %q:%d", listen, port)
	}
	route.address = net.JoinHostPort(ip.String(), strconv.Itoa(port))
	if cert == "" && key == "" {
		return nil
	}
	if cert == "" || key == "" {
		return fmt.Errorf("shared ingress backend %s requires both TLS certificate and key", route.address)
	}
	name, err := telemtWebCertificateName(cert)
	if err != nil {
		return fmt.Errorf("shared ingress backend %s: %w", route.address, err)
	}
	if net.ParseIP(name) == nil && !telemtWebDomainPattern.MatchString(name) {
		return fmt.Errorf("shared ingress backend %s needs a valid DNS or IP certificate SAN", route.address)
	}
	if strings.ContainsAny(cert, "\r\n\x00") {
		return fmt.Errorf("shared ingress backend %s has an invalid TLS certificate path", route.address)
	}
	route.certFile, route.tlsName = cert, name
	return nil
}

func telemtIngressNaiveEndpoint(inbound *model.Inbound) (host, address string, err error) {
	if inbound == nil {
		return "", "", errors.New("missing Naive inbound")
	}
	ip := net.ParseIP(inbound.Listen)
	if inbound.Protocol != model.NaiveProxy || inbound.NodeID != nil || !inbound.Enable ||
		ip == nil || !ip.IsLoopback() || inbound.Port <= 0 || inbound.Port >= 65536 || inbound.Port == 443 {
		return "", "", fmt.Errorf("Naive ingress requires an enabled local Naive inbound on a loopback IP and an internal port")
	}
	var settings struct {
		Network string `json:"network"`
		TLS     struct {
			ServerName string `json:"serverName"`
		} `json:"tls"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return "", "", fmt.Errorf("decode Naive inbound settings: %w", err)
	}
	host = normalizeTelemtWebDomain(settings.TLS.ServerName)
	if settings.Network != "tcp" || len(host) > 253 || !telemtWebDomainPattern.MatchString(host) {
		return "", "", fmt.Errorf("Naive ingress requires TCP (HTTPS) and a dedicated public TLS SNI hostname")
	}
	return host, net.JoinHostPort(ip.String(), strconv.Itoa(inbound.Port)), nil
}

// TLS passthrough preserves the selected inbound's own certificate and protocol.
// The SNI must be unambiguous; a TCP listener cannot share this route with UDP.
// Native TLS protocols keep their SNI and transport in protocol settings,
// not in the Xray streamSettings object. Only TCP profiles are publishable
// through the HAProxy TLS passthrough listener.
func telemtIngressNativeTLS(inbound *model.Inbound) (string, error) {
	var settings struct {
		Hostname  string          `json:"hostname"`
		Version   json.RawMessage `json:"version"`
		Handshake struct {
			Server string `json:"server"`
		} `json:"handshake"`
		HandshakeForServerName map[string]json.RawMessage `json:"handshakeForServerName"`
		WildcardSNI            string                     `json:"wildcardSni"`
		TLS                    struct {
			Enabled    *bool  `json:"enabled"`
			ServerName string `json:"serverName"`
		} `json:"tls"`
		Vetroff struct {
			Transport    string `json:"transport"`
			PublicServer string `json:"publicServer"`
			BondHub      bool   `json:"bondHub"`
			Discover     bool   `json:"discover"`
		} `json:"vetroff"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return "", fmt.Errorf("decode native TLS settings: %w", err)
	}
	host := normalizeTelemtWebDomain(settings.Hostname)
	switch inbound.Protocol {
	case model.AnyTLS, model.MASQUE:
		if settings.TLS.Enabled != nil && !*settings.TLS.Enabled {
			return "", errors.New("shared ingress requires enabled TLS")
		}
		host = normalizeTelemtWebDomain(settings.TLS.ServerName)
		if inbound.Protocol == model.MASQUE {
			var versions []int
			if err := json.Unmarshal(settings.Version, &versions); err != nil || len(versions) == 0 {
				return "", errors.New("MASQUE shared ingress requires explicit HTTP/1 or HTTP/2 versions")
			}
			for _, version := range versions {
				if version != 1 && version != 2 {
					return "", errors.New("MASQUE HTTP/3 uses UDP; select only HTTP/1 and HTTP/2 for shared TCP ingress")
				}
			}
		}
	case model.ShadowTLS:
		if settings.WildcardSNI != "" && settings.WildcardSNI != "off" || len(settings.HandshakeForServerName) != 0 {
			return "", errors.New("shared ShadowTLS ingress requires one fixed handshake SNI without wildcard or alternate names")
		}
		host = normalizeTelemtWebDomain(settings.Handshake.Server)
	case model.VetroffProxy:
		switch settings.Vetroff.Transport {
		case "", "h2-connect", "h2-post", "wss", "xhttp-stream-one", "xhttp-stream-up", "xhttp-packet-up":
		default:
			return "", errors.New("VetroffProxy shared TCP ingress requires a TCP transport; HTTP/3 uses UDP")
		}
		// Explicit public origins override the endpoint during link export.
		// Do not publish a misleading 443 route if that override disagrees.
		if origin := strings.TrimSpace(settings.Vetroff.PublicServer); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), host) ||
				(u.Port() != "" && u.Port() != "443") {
				return "", errors.New("VetroffProxy public HTTPS origin must match the shared SNI and port 443")
			}
		} else if settings.Vetroff.BondHub || settings.Vetroff.Discover {
			return "", errors.New("VetroffProxy Bond hub/discovery requires an explicit public HTTPS origin on port 443")
		}
	case model.TrustTunnel:
	default:
		return "", errors.New("unsupported native TLS protocol")
	}
	if len(host) > 253 || !telemtWebDomainPattern.MatchString(host) {
		return "", errors.New("shared TCP ingress requires a dedicated valid SNI hostname")
	}
	return host, nil
}

func telemtIngressProxyEndpoint(inbound *model.Inbound, publicDomain string) (host, address, publicAddress, security string, err error) {
	if inbound == nil {
		return "", "", "", "", errors.New("missing shared ingress inbound")
	}
	if inbound.Protocol == model.NaiveProxy {
		host, address, err = telemtIngressNaiveEndpoint(inbound)
		return host, address, host, "tls", err
	}
	ip := net.ParseIP(inbound.Listen)
	if inbound.NodeID != nil || !inbound.Enable || ip == nil || !ip.IsLoopback() ||
		inbound.Port <= 0 || inbound.Port >= 65536 || inbound.Port == 443 {
		return "", "", "", "", errors.New("shared ingress requires an enabled local inbound on a loopback IP and an internal port")
	}
	if model.ShadowTLSTransport(inbound.Settings) != nil {
		return "", "", "", "", errors.New("shared TLS ingress does not support an additional ShadowTLS wrapper")
	}
	switch inbound.Protocol {
	case model.VetroffProxy, model.AnyTLS, model.TrustTunnel, model.MASQUE, model.ShadowTLS:
		host, err = telemtIngressNativeTLS(inbound)
		if err != nil {
			return "", "", "", "", err
		}
		publicAddress = host
		if inbound.Protocol == model.ShadowTLS {
			publicAddress = publicDomain
		}
		return host, net.JoinHostPort(ip.String(), strconv.Itoa(inbound.Port)), publicAddress, "tls", nil
	}
	if inboundTransports(inbound.Protocol, inbound.StreamSettings, inbound.Settings)&transportTCP == 0 {
		return "", "", "", "", errors.New("shared TLS ingress requires a TCP listener")
	}
	switch string(inbound.Protocol) {
	case "vless", "vmess", "trojan", "shadowsocks":
	default:
		return "", "", "", "", fmt.Errorf("protocol %q does not support shared TLS ingress", inbound.Protocol)
	}
	var stream struct {
		Network  string `json:"network"`
		Security string `json:"security"`
		TLS      struct {
			ServerName string `json:"serverName"`
		} `json:"tlsSettings"`
		Reality struct {
			ServerNames []string `json:"serverNames"`
		} `json:"realitySettings"`
	}
	if err = json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
		return "", "", "", "", fmt.Errorf("decode shared ingress stream settings: %w", err)
	}
	switch stream.Network {
	case "tcp", "raw", "ws", "grpc", "httpupgrade", "xhttp", "splithttp", "http", "h2":
	default:
		return "", "", "", "", fmt.Errorf("transport %q does not support shared TCP ingress", stream.Network)
	}
	switch stream.Security {
	case "tls":
		host = normalizeTelemtWebDomain(stream.TLS.ServerName)
		publicAddress = host
	case "reality":
		if len(stream.Reality.ServerNames) != 1 {
			return "", "", "", "", errors.New("shared REALITY ingress requires exactly one dedicated SNI")
		}
		host = normalizeTelemtWebDomain(stream.Reality.ServerNames[0])
		publicAddress = publicDomain
	default:
		return "", "", "", "", errors.New("shared TCP ingress requires TLS or REALITY")
	}
	if len(host) > 253 || !telemtWebDomainPattern.MatchString(host) {
		return "", "", "", "", errors.New("shared TCP ingress requires a dedicated valid SNI hostname")
	}
	return host, net.JoinHostPort(ip.String(), strconv.Itoa(inbound.Port)), publicAddress, stream.Security, nil
}

func buildTelemtWebIngress(state TelemtWebProxyState) (telemtIngressPlan, error) {
	return planTelemtWebIngress(state, true)
}

func planTelemtWebIngress(state TelemtWebProxyState, strict bool) (telemtIngressPlan, error) {
	var plan telemtIngressPlan
	if state.SharedIngress == nil || !state.SharedIngress.Enabled {
		return plan, errors.New("shared ingress is not enabled")
	}
	service := &SettingService{}
	settings, err := service.GetAllSetting()
	if err != nil {
		return plan, err
	}
	gateway, err := service.domainFrontConfig(settings)
	if err != nil {
		return plan, err
	}
	hosts := telemtWebHosts(state)
	panelHost := normalizeTelemtWebDomain(settings.WebDomain)
	if panelHost == "" {
		panelHost = state.Domain
	}
	if !slices.Contains(hosts, panelHost) {
		return plan, fmt.Errorf("panel domain %q must be a Telemt WEB domain or alias", panelHost)
	}
	plan.panel.host = panelHost
	if settings.WebCertFile == "" || settings.WebKeyFile == "" {
		return plan, fmt.Errorf("shared ingress requires HTTPS on the local panel listener")
	}
	port := settings.WebPort
	if override, set, err := config.GetPortOverride(); err != nil {
		return plan, err
	} else if set {
		port = override
	}
	if err := configureTelemtIngressUpstream(&plan.panel, settings.WebListen, port, settings.WebCertFile, settings.WebKeyFile); err != nil {
		return plan, err
	}
	if err := appendIngressPaths(&plan.panel, settings.WebBasePath); err != nil {
		return plan, fmt.Errorf("panel: %w", err)
	}
	aliases, err := service.GetHiddifyLegacySubscriptionAliases()
	if err != nil {
		return plan, err
	}
	var legacy telemtIngressRoute
	for _, alias := range aliases {
		if path := strings.Trim(alias.Path, "/"); path != "" {
			if err := appendIngressPaths(&legacy, "/"+path); err != nil {
				return plan, err
			}
		}
	}
	// Stable ordering avoids a reload if equivalent alias settings are reordered.
	slices.Sort(legacy.paths)
	plan.legacyPaths = legacy.paths

	if settings.SubEnable {
		publicURL := subscriptionPublicURL(gateway, settings)
		if publicURL == "" {
			publicURL = "https://" + state.Domain
		}
		for _, host := range hosts {
			if publicURL == "https://"+host {
				plan.subscriptions.host = host
				break
			}
		}
		if plan.subscriptions.host == "" {
			return plan, fmt.Errorf("subscription public URL must be https:// followed by a Telemt WEB domain or alias")
		}
		if err := configureTelemtIngressUpstream(&plan.subscriptions, settings.SubListen, settings.SubPort, settings.SubCertFile, settings.SubKeyFile); err != nil {
			return plan, err
		}
		if err := appendIngressPaths(&plan.subscriptions, settings.SubPath, "/assets"); err != nil {
			return plan, err
		}
		if settings.SubJsonEnable {
			if err := appendIngressPaths(&plan.subscriptions, settings.SubJsonPath); err != nil {
				return plan, err
			}
		}
		if settings.SubClashEnable {
			if err := appendIngressPaths(&plan.subscriptions, settings.SubClashPath, "/mihomo", "/clash-legacy"); err != nil {
				return plan, err
			}
		}
	}

	if state.BasePath == "" {
		return plan, fmt.Errorf("shared ingress requires a non-empty Telemt WEB base path")
	}
	for _, path := range slices.Concat(plan.panel.paths, plan.legacyPaths, plan.subscriptions.paths) {
		if domainPathOverlap(path, "/"+state.BasePath) {
			return plan, fmt.Errorf("Telemt WEB base path conflicts with %s", path)
		}
	}
	protected := plan.legacyPaths
	if plan.panel.host == plan.subscriptions.host {
		protected = slices.Concat(protected, plan.panel.paths)
	}
	for _, path := range protected {
		for _, subPath := range plan.subscriptions.paths {
			if domainPathOverlap(path, subPath) {
				return plan, fmt.Errorf("panel and subscription paths overlap: %s and %s", path, subPath)
			}
		}
	}

	ids, err := telemtIngressInboundIDs(state.SharedIngress)
	if err != nil {
		return plan, err
	}
	if len(ids) == 0 {
		return plan, nil
	}
	var inbounds []model.Inbound
	if err := database.GetDB().Where("id IN ?", ids).Order("id").Find(&inbounds).Error; err != nil {
		return plan, fmt.Errorf("load shared ingress inbounds: %w", err)
	}
	if strict && len(inbounds) != len(ids) {
		return plan, errors.New("one or more shared ingress inbounds no longer exist")
	}
	plan.proxies, err = planTelemtIngressProxies(inbounds, hosts, state.Domain, strict)
	if err != nil {
		return plan, err
	}
	return plan, nil
}

// Strict preflight rejects collisions. Routine refresh removes every ambiguous
// route instead of silently selecting a winner and misdirecting credentials.
func planTelemtIngressProxies(inbounds []model.Inbound, webHosts []string, publicDomain string, strict bool) ([]telemtIngressProxyRoute, error) {
	var routes []telemtIngressProxyRoute
	counts := make(map[string]int)
	for _, inbound := range inbounds {
		host, address, publicAddress, security, err := telemtIngressProxyEndpoint(&inbound, publicDomain)
		if err == nil && slices.Contains(webHosts, host) {
			err = fmt.Errorf("inbound SNI %q must differ from panel and Telemt WEB hostnames", host)
		}
		if err != nil {
			if strict {
				return nil, fmt.Errorf("shared ingress inbound %d: %w", inbound.Id, err)
			}
			continue
		}
		counts[host]++
		routes = append(routes, telemtIngressProxyRoute{id: inbound.Id, host: host, address: address, publicAddress: publicAddress, security: security, protocol: inbound.Protocol})
	}
	if strict {
		for _, route := range routes {
			if counts[route.host] > 1 {
				return nil, fmt.Errorf("duplicate shared ingress SNI %q", route.host)
			}
		}
	}
	routes = slices.DeleteFunc(routes, func(route telemtIngressProxyRoute) bool { return counts[route.host] > 1 })
	slices.SortFunc(routes, func(a, b telemtIngressProxyRoute) int {
		return strings.Compare(a.host, b.host)
	})
	return routes, nil
}

func renderTelemtWebIngress(state TelemtWebProxyState, plan telemtIngressPlan) []byte {
	bind := ":443"
	if telemtWebNeedsIPv6(state) {
		bind = "[::]:443 v4v6"
	}
	var cfg strings.Builder
	cfg.WriteString("global\n    maxconn 4096\n\ndefaults\n    mode http\n    timeout connect 5s\n    timeout client 65s\n    timeout server 65s\n    timeout tunnel 1h\n")
	if len(plan.proxies) > 0 {
		fmt.Fprintf(&cfg, "\nfrontend entry\n    mode tcp\n    bind %s\n    tcp-request inspect-delay 5s\n    tcp-request content accept if { req.ssl_hello_type 1 }\n", bind)
		for _, route := range plan.proxies {
			fmt.Fprintf(&cfg, "    use_backend proxy_%d if { req.ssl_sni -i %s }\n", route.id, route.host)
		}
		cfg.WriteString("    default_backend web_tls\n")
		for _, route := range plan.proxies {
			fmt.Fprintf(&cfg, "\nbackend proxy_%d\n    mode tcp\n    server local %s\n", route.id, route.address)
		}
		fmt.Fprintf(&cfg, "\nbackend web_tls\n    mode tcp\n    server web %s send-proxy-v2\n", telemtWebIngressTLSSocket)
		bind = telemtWebIngressTLSSocket + " accept-proxy"
	}
	fmt.Fprintf(&cfg, "\nfrontend web\n    bind %s ssl crt %s/tls.pem alpn h2,http/1.1\n", bind, telemtWebFrontDir)
	cfg.WriteString("    http-request set-var(txn.host) req.hdr(host),field(1,:),lower\n    http-request set-var(txn.sni) ssl_fc_sni,lower\n")
	fmt.Fprintf(&cfg, "    http-request deny deny_status 404 unless { var(txn.host) -m str %s }\n", strings.Join(telemtWebHosts(state), " "))
	cfg.WriteString("    http-request return status 421 unless { var(txn.sni),strcmp(txn.host) eq 0 }\n    http-request deny if METH_CONNECT\n")
	cfg.WriteString("    http-request del-header Forwarded\n    http-request set-header X-Real-IP %[src]\n    http-request set-header X-Forwarded-For %[src]\n    http-request set-header X-Forwarded-Proto https\n    http-request set-header X-Forwarded-Host %[var(txn.host)]\n    http-request set-header X-Forwarded-Port 443\n")
	for _, path := range plan.legacyPaths {
		fmt.Fprintf(&cfg, "    acl legacy_path path -m str %s\n    acl legacy_path path_beg %s/\n", path, path)
	}
	if len(plan.legacyPaths) > 0 {
		cfg.WriteString("    use_backend panel if legacy_path\n")
	}
	routes := []struct {
		name string
		telemtIngressRoute
	}{
		{"panel", plan.panel}, {"subscriptions", plan.subscriptions},
	}
	for _, route := range routes {
		for _, path := range route.paths {
			fmt.Fprintf(&cfg, "    acl %s_path path -m str %s\n    acl %s_path path_beg %s/\n", route.name, path, route.name, path)
		}
		if len(route.paths) > 0 {
			fmt.Fprintf(&cfg, "    use_backend %s if %s_path { var(txn.host) -m str %s }\n", route.name, route.name, route.host)
		}
	}
	cfg.WriteString("    default_backend telemt\n")
	fmt.Fprintf(&cfg, "\nbackend telemt\n    http-reuse safe\n    server telemt %s\n", net.JoinHostPort(telemtWebListenIP, strconv.Itoa(state.ListenPort)))
	cfg.WriteString("    http-response add-header Content-Security-Policy \"sandbox allow-scripts; connect-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'\" if { res.hdr(Content-Type) -m beg text/html }\n")
	for _, route := range routes {
		if len(route.paths) == 0 {
			continue
		}
		fmt.Fprintf(&cfg, "\nbackend %s\n    server local %s", route.name, route.address)
		if route.certFile != "" {
			fmt.Fprintf(&cfg, " ssl verify required ca-file %s verifyhost %s sni str(%s)", strconv.Quote(route.certFile), route.tlsName, route.tlsName)
		}
		cfg.WriteByte('\n')
	}
	return []byte(cfg.String())
}

// refreshTelemtIngress runs with telemtWebOperationMu held. Continuous
// reconciliation updates routes independently of the Telemt user list.
func refreshTelemtIngress(state TelemtWebProxyState) error {
	if !state.Enabled || state.SharedIngress == nil || !state.SharedIngress.Enabled {
		publishedTelemtIngress.Store(nil)
		return nil
	}
	plan, err := planTelemtWebIngress(state, false)
	if err != nil {
		publishedTelemtIngress.Store(nil)
		return err
	}
	path := telemtIngressFrontendPath(state)
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := renderTelemtIngressFrontend(state, plan)
	if err != nil {
		return err
	}
	var previousRouter, nextRouter []byte
	routerChanged := false
	if telemtIngressUsesRouter(state) {
		previousRouter, err = os.ReadFile(telemtIngressRouterConfig)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		nextRouter, err = telemtIngressRouterData(state, plan)
		if err != nil {
			return err
		}
		routerChanged = !bytes.Equal(previousRouter, nextRouter)
	}
	webChanged := !bytes.Equal(old, data)
	active := telemtIngressEntryActive(state)
	if webChanged || routerChanged || !active {
		rollback := func(cause error) error {
			publishedTelemtIngress.Store(nil)
			var restoreErr error
			if webChanged || !active {
				restoreErr = telemtIngressRestoreConfig(path, old)
			}
			if telemtIngressUsesRouter(state) {
				restoreErr = errors.Join(restoreErr, telemtIngressRestoreConfig(telemtIngressRouterConfig, previousRouter))
			}
			if restoreErr == nil && old != nil && active {
				if webChanged {
					restoreErr = telemtWebReloadFrontend(state)
				} else if routerChanged {
					restoreErr = telemtIngressEnsureRouter(state)
				}
			}
			return errors.Join(cause, restoreErr)
		}
		if !webChanged && active && routerChanged {
			// A VPN-only change need not restart the HTTP server or drop its
			// established WebSocket lanes.
			if err := telemtIngressWriteRouterConfig(state, nextRouter); err != nil {
				return err
			}
			if err := telemtIngressEnsureRouter(state); err != nil {
				return rollback(err)
			}
		} else {
			if err := writeTelemtIngressFrontend(state, plan); err != nil {
				return rollback(err)
			}
			if err := telemtWebEnsureFrontendRunning(state); err != nil {
				return rollback(err)
			}
			if err := telemtWebReloadFrontend(state); err != nil {
				return rollback(err)
			}
		}
		// A successful reload does not prove the new router actually owns TCP
		// 443. Keep the rollback snapshot until ownership is verified.
		if err := telemtIngressWaitPublicOwner(state); err != nil {
			return rollback(err)
		}
	} else if err := telemtIngressWaitPublicOwner(state); err != nil {
		publishedTelemtIngress.Store(nil)
		return err
	}
	publishTelemtIngress(state, plan)
	return nil
}

func telemtIngressRestoreConfig(path string, previous []byte) error {
	if previous == nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return telemtWebFrontendRestoreFile(path, previous)
}
