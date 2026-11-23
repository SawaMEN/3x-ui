package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
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
	Enabled        bool `json:"enabled"`
	NaiveInboundID int  `json:"naiveInboundId,omitempty"`
}

func telemtWebIngressEqual(a, b *TelemtWebIngress) bool {
	aEnabled, bEnabled := a != nil && a.Enabled, b != nil && b.Enabled
	return aEnabled == bEnabled && (!aEnabled || *a == *b)
}

const telemtWebIngressDefaultPath = "telegram/web"

type telemtIngressPublication struct {
	subscriptionHost string
	naiveHost        string
	naiveInboundID   int
}

var publishedTelemtIngress atomic.Pointer[telemtIngressPublication]

func publishTelemtIngress(state TelemtWebProxyState, plan telemtIngressPlan) {
	publishedTelemtIngress.Store(&telemtIngressPublication{
		subscriptionHost: plan.subscriptions.host,
		naiveHost:        plan.naiveHost,
		naiveInboundID:   state.SharedIngress.NaiveInboundID,
	})
}

func telemtIngressSubscriptionOrigin() string {
	if published := publishedTelemtIngress.Load(); published != nil && published.subscriptionHost != "" {
		return "https://" + published.subscriptionHost
	}
	return ""
}

func telemtIngressNaiveHost(inbound *model.Inbound) *model.Host {
	published := publishedTelemtIngress.Load()
	if published == nil || published.naiveInboundID != inbound.Id || published.naiveHost == "" {
		return nil
	}
	host := published.naiveHost
	return &model.Host{InboundId: inbound.Id, Remark: host, Address: host, Port: 443, Security: "tls", Sni: host, HostHeader: host}
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
	naiveHost     string
	naiveAddress  string
}

var ingressPath = regexp.MustCompile(`^/[-A-Za-z0-9_.~/]+$`)

func appendIngressPaths(route *telemtIngressRoute, paths ...string) error {
	for _, path := range paths {
		path = strings.TrimRight(path, "/")
		if !ingressPath.MatchString(path) || strings.Contains(path, "//") ||
			slices.Contains(strings.Split(path, "/"), ".") || slices.Contains(strings.Split(path, "/"), "..") {
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

func buildTelemtWebIngress(state TelemtWebProxyState) (telemtIngressPlan, error) {
	return planTelemtWebIngress(state, true)
}

func planTelemtWebIngress(state TelemtWebProxyState, requireNaive bool) (telemtIngressPlan, error) {
	var plan telemtIngressPlan
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

	if id := state.SharedIngress.NaiveInboundID; id > 0 {
		var inbound model.Inbound
		if err := database.GetDB().First(&inbound, id).Error; err != nil {
			if !requireNaive && database.IsNotFound(err) {
				return plan, nil
			}
			return plan, fmt.Errorf("load Naive inbound %d: %w", id, err)
		}
		host, address, err := telemtIngressNaiveEndpoint(&inbound)
		if err == nil && slices.Contains(hosts, host) {
			err = fmt.Errorf("Naive SNI %q must differ from the panel and Telemt WEB hostnames", host)
		}
		if err != nil {
			if requireNaive {
				return plan, err
			}
			return plan, nil
		}
		plan.naiveHost, plan.naiveAddress = host, address
	}
	return plan, nil
}

func renderTelemtWebIngress(state TelemtWebProxyState, plan telemtIngressPlan) []byte {
	bind := ":443"
	if host, _, err := net.SplitHostPort(state.PublicAddr); err == nil && net.ParseIP(host) != nil && net.ParseIP(host).To4() == nil {
		bind = "[::]:443 v4v6"
	}
	var cfg strings.Builder
	cfg.WriteString("global\n    maxconn 4096\n\ndefaults\n    mode http\n    timeout connect 5s\n    timeout client 65s\n    timeout server 65s\n    timeout tunnel 1h\n")
	if plan.naiveHost != "" {
		fmt.Fprintf(&cfg, "\nfrontend entry\n    mode tcp\n    bind %s\n    tcp-request inspect-delay 5s\n    tcp-request content accept if { req.ssl_hello_type 1 }\n    use_backend naive if { req.ssl_sni -i %s }\n    default_backend web_tls\n", bind, plan.naiveHost)
		fmt.Fprintf(&cfg, "\nbackend naive\n    mode tcp\n    server naive %s\n", plan.naiveAddress)
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
		return nil
	}
	plan, err := planTelemtWebIngress(state, false)
	if err != nil {
		return err
	}
	path := telemtWebFrontDir + "/config"
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data := renderTelemtWebIngress(state, plan)
	active := telemtWebFrontendActive(state)
	changed := !bytes.Equal(old, data)
	if changed || !active {
		// Routine routing reloads leave certificate renewal to its hook.
		var writeErr error
		if active {
			writeErr = writeTelemtWebFrontendFile(path, data)
		} else {
			writeErr = writeTelemtWebFrontendFiles(state, data)
		}
		if writeErr != nil {
			return writeErr
		}
		if active {
			err = telemtWebReloadFrontend(state)
		} else {
			err = telemtWebEnsureFrontendRunning(state)
		}
		if err != nil {
			if old != nil {
				return errors.Join(err, writeTelemtWebFrontendFile(path, old))
			}
			return err
		}
	}
	publishTelemtIngress(state, plan)
	return nil
}
