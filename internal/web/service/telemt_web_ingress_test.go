package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestTelemtWebSharedNaiveRequiresTCPAndLoopback(t *testing.T) {
	naive := model.Inbound{
		Protocol: model.NaiveProxy,
		Listen:   "127.0.0.1",
		Port:     9443,
		Enable:   true,
		Settings: `{"network":"tcp","tls":{"serverName":"naive.example.com"}}`,
	}
	host, address, err := telemtIngressNaiveEndpoint(&naive)
	if err != nil || host != "naive.example.com" || address != "127.0.0.1:9443" {
		t.Fatalf("Naive endpoint = %s %s, %v", host, address, err)
	}
	for _, mutate := range []func(*model.Inbound){
		func(ib *model.Inbound) { ib.Listen = "0.0.0.0" },
		func(ib *model.Inbound) { ib.Port = 443 },
		func(ib *model.Inbound) { ib.Settings = `{"network":"udp","tls":{"serverName":"naive.example.com"}}` },
		func(ib *model.Inbound) { ib.Settings = `{"network":"tcp","tls":{"serverName":"malicious host"}}` },
	} {
		invalid := naive
		mutate(&invalid)
		if _, _, err := telemtIngressNaiveEndpoint(&invalid); err == nil {
			t.Fatalf("unsafe or unsupported Naive endpoint accepted: %+v", invalid)
		}
	}
}

func TestTelemtWebSharedIngressPathSafety(t *testing.T) {
	for _, path := range []string{"/", "/path/../other", "/path with space", "/path//nested", "/path//", "//", "/path%2fother"} {
		if err := appendIngressPaths(&telemtIngressRoute{}, path); err == nil {
			t.Fatalf("invalid shared ingress path %q accepted", path)
		}
	}
}

func TestTelemtWebSharedIngressBuildsFromPanelSettings(t *testing.T) {
	setupConflictDB(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile := telemtWebTestCertificate(t, key, time.Now().Add(time.Hour))
	service := &SettingService{}
	for k, v := range map[string]string{
		"webDomain": "web.example.com", "webListen": "127.0.0.1", "webPort": "8443",
		"webBasePath": "/admin-path/", "webCertFile": certFile, "webKeyFile": keyFile,
		"subEnable": "true", "subDomain": "web.example.com", "subListen": "127.0.0.1", "subPort": "2096",
		"subPublicURL": "https://web.example.com", "subPath": "/sub-path/",
	} {
		if err := service.setString(k, v); err != nil {
			t.Fatal(err)
		}
	}
	state := TelemtWebProxyState{
		Domain: "web.example.com", BasePath: "telegram/web",
		SharedIngress: &TelemtWebIngress{Enabled: true},
	}
	plan, err := buildTelemtWebIngress(state)
	if err != nil {
		t.Fatal(err)
	}
	if plan.panel.address != "127.0.0.1:8443" || plan.subscriptions.address != "127.0.0.1:2096" ||
		plan.panel.tlsName != "web.example.com" || plan.subscriptions.tlsName != "" {
		t.Fatalf("unexpected routing plan: %+v", plan)
	}
	state.BasePath = "admin-path/subpath"
	if _, err := buildTelemtWebIngress(state); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("overlapping Telemt path must be rejected, got %v", err)
	}
}

func TestTelemtWebSharedIngressPublishesAutomaticAddresses(t *testing.T) {
	setupConflictDB(t)
	t.Cleanup(func() { publishedTelemtIngress.Store(nil) })
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile := telemtWebTestCertificate(t, key, time.Now().Add(time.Hour))
	settings := &SettingService{}
	for k, v := range map[string]string{
		"webListen": "0.0.0.0", "webPort": "8443", "webBasePath": "/admin-path/",
		"webCertFile": certFile, "webKeyFile": keyFile,
		"subEnable": "true", "subListen": "::", "subPort": "2096",
		"subPublicURL": "", "subPath": "/sub-path/",
	} {
		if err := settings.setString(k, v); err != nil {
			t.Fatal(err)
		}
	}
	inbound := model.Inbound{
		Protocol: model.NaiveProxy, Tag: "naive-web", Listen: "127.0.0.1", Port: 9443, Enable: true,
		Settings: `{"network":"tcp","tls":{"serverName":"naive.example.com"}}`,
	}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	opts, err := normalizeTelemtWebOptions(TelemtWebProxyOptions{
		Frontend: "haproxy", SharedIngress: &TelemtWebIngress{Enabled: true, NaiveInboundID: inbound.Id},
	})
	if err != nil || opts.BasePath == nil || *opts.BasePath != telemtWebIngressDefaultPath {
		t.Fatalf("automatic WEB path: %+v, %v", opts, err)
	}
	state := TelemtWebProxyState{Domain: "web.example.com", BasePath: *opts.BasePath, SharedIngress: opts.SharedIngress}
	plan, err := buildTelemtWebIngress(state)
	if err != nil {
		t.Fatal(err)
	}
	if plan.panel.address != "127.0.0.1:8443" || plan.subscriptions.address != "[::1]:2096" || len(plan.proxies) != 1 || plan.proxies[0].address != "127.0.0.1:9443" {
		t.Fatalf("unexpected routing plan: %+v", plan)
	}
	publishTelemtIngress(state, plan)
	if origin, err := settings.GetSubPublicURL(); err != nil || origin != "https://web.example.com" {
		t.Fatalf("automatic subscription origin: %q, %v", origin, err)
	}
	if endpoint := telemtIngressProxyHost(&inbound); endpoint == nil || endpoint.Port != 443 || endpoint.Sni != "naive.example.com" {
		t.Fatalf("automatic Naive endpoint: %+v", endpoint)
	}
	if err := database.GetDB().Model(&inbound).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	plan, err = planTelemtWebIngress(state, false)
	if err != nil || len(plan.proxies) != 0 {
		t.Fatalf("disabled Naive backend remained in plan: %+v, %v", plan, err)
	}
	publishTelemtIngress(state, plan)
	if telemtIngressProxyHost(&inbound) != nil {
		t.Fatal("disabled Naive endpoint is still published")
	}
}

func TestTelemtIngressRejectsMissingOptionsAndInbound(t *testing.T) {
	if _, err := buildTelemtWebIngress(TelemtWebProxyState{}); err == nil {
		t.Fatal("accepted missing shared ingress options")
	}
	if _, _, err := telemtIngressNaiveEndpoint(nil); err == nil {
		t.Fatal("accepted missing Naive inbound")
	}
	if telemtIngressProxyHost(nil) != nil {
		t.Fatal("published a missing inbound")
	}
}

func TestTelemtIngressNormalizesAndDeduplicatesPaths(t *testing.T) {
	var route telemtIngressRoute
	if err := appendIngressPaths(&route, "/admin/", "/admin", "/sub/"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(route.paths, ","); got != "/admin,/sub" {
		t.Fatalf("paths = %q", got)
	}
}

func TestTelemtWebSharedProxyEndpoint(t *testing.T) {
	inbound := model.Inbound{
		Protocol: model.VLESS, Listen: "127.0.0.1", Port: 9443, Enable: true,
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"vpn.example.com"}}`,
	}
	host, address, publicAddress, security, err := telemtIngressProxyEndpoint(&inbound, "web.example.com")
	if err != nil || host != "vpn.example.com" || address != "127.0.0.1:9443" || publicAddress != host || security != "tls" {
		t.Fatalf("TLS endpoint = %q %q %q %q, %v", host, address, publicAddress, security, err)
	}
	inbound.StreamSettings = `{"network":"tcp","security":"reality","realitySettings":{"serverNames":["example.org"]}}`
	host, _, publicAddress, security, err = telemtIngressProxyEndpoint(&inbound, "web.example.com")
	if err != nil || host != "example.org" || publicAddress != "web.example.com" || security != "reality" {
		t.Fatalf("REALITY endpoint = %q %q %q, %v", host, publicAddress, security, err)
	}
	for _, stream := range []string{
		`{"network":"quic","security":"tls","tlsSettings":{"serverName":"vpn.example.com"}}`,
		`{"network":"tcp","security":"none"}`,
		`{"network":"tcp","security":"reality","realitySettings":{"serverNames":["example.org","example.net"]}}`,
		`{"network":"tcp","security":"tls","tlsSettings":{"serverName":"bad name"}}`,
	} {
		inbound.StreamSettings = stream
		if _, _, _, _, err := telemtIngressProxyEndpoint(&inbound, "web.example.com"); err == nil {
			t.Fatalf("unsupported stream accepted: %s", stream)
		}
	}
}

func TestTelemtWebNativeTLSSharedProxy(t *testing.T) {
	for _, tc := range []struct {
		protocol model.Protocol
		settings string
		ok       bool
	}{
		{model.VetroffProxy, `{"hostname":"vpn.example.com","vetroff":{"transport":"h2-connect"}}`, true},
		{model.VetroffProxy, `{"hostname":"vpn.example.com","vetroff":{"transport":"wss"}}`, true},
		{model.VetroffProxy, `{"hostname":"vpn.example.com","vetroff":{"transport":"xhttp-stream-up"}}`, true},
		{model.VetroffProxy, `{"hostname":"vpn.example.com","vetroff":{"transport":"h3-connect"}}`, false},
		{model.VetroffProxy, `{"hostname":"vpn.example.com","vetroff":{"publicServer":"https://vpn.example.com:9443"}}`, false},
		{model.VetroffProxy, `{"hostname":"vpn.example.com","vetroff":{"publicServer":"https://vpn.example.com:443","bondHub":true}}`, true},
		{model.VetroffProxy, `{"hostname":"vpn.example.com","vetroff":{"bondHub":true}}`, false},
		{model.AnyTLS, `{"tls":{"enabled":true,"serverName":"vpn.example.com"}}`, true},
		{model.AnyTLS, `{"tls":{"enabled":false,"serverName":"vpn.example.com"}}`, false},
		{model.AnyTLS, `{"tls":{"enabled":true,"serverName":"vpn.example.com"},"shadowTls":{"enabled":true}}`, false},
		{model.TrustTunnel, `{"hostname":"vpn.example.com"}`, true},
	} {
		t.Run(string(tc.protocol)+tc.settings, func(t *testing.T) {
			inbound := &model.Inbound{Id: 42, Protocol: tc.protocol, Enable: true, Listen: "127.0.0.1", Port: 9443, Settings: tc.settings}
			host, addr, public, security, err := telemtIngressProxyEndpoint(inbound, "web.example.com")
			if !tc.ok {
				if err == nil {
					t.Fatal("unsupported native endpoint was accepted")
				}
				return
			}
			if err != nil || host != "vpn.example.com" || addr != "127.0.0.1:9443" || public != host || security != "tls" {
				t.Fatalf("native endpoint %q %q %q %q: %v", host, addr, public, security, err)
			}
			t.Cleanup(func() { publishedTelemtIngress.Store(nil) })
			publishTelemtIngress(TelemtWebProxyState{SharedIngress: &TelemtWebIngress{Enabled: true, NaiveInboundID: inbound.Id}},
				telemtIngressPlan{proxies: []telemtIngressProxyRoute{{id: inbound.Id, host: host, address: addr, publicAddress: public, security: security, protocol: inbound.Protocol}}})
			if entry := telemtIngressProxyHost(inbound); entry == nil || entry.Port != 443 || entry.Address != public || entry.Sni != host {
				t.Fatalf("native public route: %+v", entry)
			}
		})
	}
}

func TestTelemtIngressMultipleRoutes(t *testing.T) {
	t.Cleanup(func() { publishedTelemtIngress.Store(nil) })
	one := model.Inbound{Id: 1, Protocol: model.VetroffProxy, Enable: true, Listen: "127.0.0.1", Port: 9443, Settings: `{"hostname":"vetroff.example.com"}`}
	two := model.Inbound{Id: 2, Protocol: model.AnyTLS, Enable: true, Listen: "127.0.0.1", Port: 9444, Settings: `{"tls":{"serverName":"anytls.example.com"}}`}
	state := TelemtWebProxyState{Domain: "web.example.com", SharedIngress: &TelemtWebIngress{Enabled: true, InboundIDs: []int{1, 2}}}
	routes, err := planTelemtIngressProxies([]model.Inbound{one, two}, []string{state.Domain}, state.Domain, true)
	if err != nil || len(routes) != 2 {
		t.Fatalf("multiple routes: %+v, %v", routes, err)
	}
	plan := telemtIngressPlan{proxies: routes}
	config := string(renderTelemtWebIngress(state, plan))
	for _, expected := range []string{"use_backend proxy_1 if { req.ssl_sni -i vetroff.example.com }", "use_backend proxy_2 if { req.ssl_sni -i anytls.example.com }", "server local 127.0.0.1:9443", "server local 127.0.0.1:9444"} {
		if !strings.Contains(config, expected) {
			t.Fatalf("missing %q in config", expected)
		}
	}
	publishTelemtIngress(state, plan)
	for _, inbound := range []model.Inbound{one, two} {
		if endpoint := telemtIngressProxyHost(&inbound); endpoint == nil || endpoint.Port != 443 {
			t.Fatalf("missing published endpoint for %d", inbound.Id)
		}
	}
	two.Port++
	if telemtIngressProxyHost(&two) != nil {
		t.Fatal("published stale endpoint after port change")
	}
	two.Settings = `{"tls":{"serverName":"vetroff.example.com"}}`
	if _, err := planTelemtIngressProxies([]model.Inbound{one, two}, nil, state.Domain, true); err == nil {
		t.Fatal("accepted duplicate SNI")
	}
	if routes, err := planTelemtIngressProxies([]model.Inbound{one, two}, nil, state.Domain, false); err != nil || len(routes) != 0 {
		t.Fatalf("ambiguous routes survived refresh: %+v, %v", routes, err)
	}
}

func TestTelemtIngressLegacySelectionMigration(t *testing.T) {
	legacy := &TelemtWebIngress{Enabled: true, NaiveInboundID: 42}
	current := &TelemtWebIngress{Enabled: true, InboundIDs: []int{42, 42}}
	if !telemtWebIngressEqual(legacy, current) {
		t.Fatal("legacy and equivalent current settings differ")
	}
	opts, err := normalizeTelemtWebOptions(TelemtWebProxyOptions{Frontend: "haproxy", SharedIngress: legacy})
	if err != nil || opts.SharedIngress.NaiveInboundID != 0 || len(opts.SharedIngress.InboundIDs) != 1 || opts.SharedIngress.InboundIDs[0] != 42 {
		t.Fatalf("legacy migration: %+v, %v", opts, err)
	}
	ids, err := telemtIngressInboundIDs(&TelemtWebIngress{Enabled: true, InboundIDs: []int{}, NaiveInboundID: 42})
	if err != nil || len(ids) != 0 {
		t.Fatalf("explicit empty selection must override legacy ID: %v %v", ids, err)
	}
	if _, err := telemtIngressInboundIDs(&TelemtWebIngress{Enabled: true, InboundIDs: []int{-1}}); err == nil {
		t.Fatal("accepted invalid ID")
	}
}

func TestTelemtSharedIngressAlternativeHTTPFrontends(t *testing.T) {
	plan := telemtIngressPlan{panel: telemtIngressRoute{host: "web.example.com", address: "127.0.0.1:8443", paths: []string{"/admin"}, certFile: "/cert.pem", tlsName: "web.example.com"}, subscriptions: telemtIngressRoute{host: "web.example.com", address: "127.0.0.1:2096", paths: []string{"/sub"}}, proxies: []telemtIngressProxyRoute{{id: 42, host: "vpn.example.com", address: "127.0.0.1:9443"}}}
	for _, front := range []string{"nginx", "caddy", "haproxy"} {
		state := TelemtWebProxyState{Frontend: front, Domain: "web.example.com", ListenPort: 15080, CertFile: "/cert.pem", KeyFile: "/key.pem", SharedIngress: &TelemtWebIngress{Enabled: true, InboundIDs: []int{42}}}
		data, err := renderTelemtIngressFrontend(state, plan)
		if err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{"/admin", "/sub", "127.0.0.1:8443", "127.0.0.1:2096", "127.0.0.1:15080"} {
			if !strings.Contains(string(data), expected) {
				t.Fatalf("%s missing %s", front, expected)
			}
		}
		if front != "haproxy" {
			if !strings.Contains(string(data), telemtIngressWebSocket) {
				t.Fatalf("%s missing private socket", front)
			}
			router := string(renderTelemtIngressRouter(state, plan))
			if !strings.Contains(router, "req.ssl_sni -i vpn.example.com") || !strings.Contains(router, "send-proxy-v2") {
				t.Fatal(router)
			}
		}
		state.SharedIngress.InboundIDs = nil
		if telemtIngressUsesRouter(state) || telemtIngressPublicOwner(state) != front {
			t.Fatalf("web-only mode should not use separate router: %s", front)
		}
	}
}

func TestTelemtIngressMASQUEAndShadowTLS(t *testing.T) {
	for _, tc := range []struct {
		protocol model.Protocol
		settings string
		public   string
		ok       bool
	}{
		{model.MASQUE, `{"version":[1,2],"tls":{"enabled":true,"serverName":"vpn.example.com"}}`, "vpn.example.com", true},
		{model.MASQUE, `{"version":[3,2],"tls":{"serverName":"vpn.example.com"}}`, "", false},
		{model.MASQUE, `{"tls":{"serverName":"vpn.example.com"}}`, "", false},
		{model.ShadowTLS, `{"version":3,"handshake":{"server":"vpn.example.com"},"wildcardSni":"off"}`, "web.example.com", true},
		{model.ShadowTLS, `{"handshake":{"server":"vpn.example.com"},"wildcardSni":"all"}`, "", false},
		{model.ShadowTLS, `{"handshake":{"server":"vpn.example.com"},"handshakeForServerName":{"other.example.com":{"server":"other.example.com"}}}`, "", false},
	} {
		inbound := &model.Inbound{Id: 42, Protocol: tc.protocol, Enable: true, Listen: "127.0.0.1", Port: 9443, Settings: tc.settings}
		host, _, public, _, err := telemtIngressProxyEndpoint(inbound, "web.example.com")
		if (err == nil) != tc.ok || tc.ok && (host != "vpn.example.com" || public != tc.public) {
			t.Fatalf("%s endpoint %q %q: %v", tc.protocol, host, public, err)
		}
	}
}

func TestTelemtUDP443IndependentReservations(t *testing.T) {
	for _, tc := range []struct {
		protocol model.Protocol
		port     int
		settings string
		want     bool
	}{
		{model.Hysteria, 443, "{}", true},
		{model.TUIC, 443, "{}", true},
		{model.Hysteria, 8443, "{}", false},
		{model.MASQUE, 443, `{"version":[1,2]}`, false},
		{model.MASQUE, 443, `{"version":[3]}`, true},
		{model.VetroffProxy, 8443, `{"vetroff":{"http3":true,"h3Listen":"0.0.0.0:443"}}`, true},
		{model.VetroffProxy, 443, `{"vetroff":{"http3":false}}`, false},
	} {
		inbound := &model.Inbound{Protocol: tc.protocol, Port: tc.port, Settings: tc.settings}
		if got := telemtInboundUsesUDP443(inbound); got != tc.want {
			t.Errorf("%s/%d: UDP 443 = %v, want %v", tc.protocol, tc.port, got, tc.want)
		}
	}
}

func TestTelemtIngressSuggestedPath(t *testing.T) {
	for _, tc := range []struct {
		path      string
		protected []string
		want      string
	}{
		{"custom/web", []string{"/panel", "/sub"}, "custom/web"},
		{"", []string{"/panel", "/sub"}, "telegram/web"},
		{"telegram/web", []string{"/telegram", "/telegram-web-1"}, "telegram-web-2"},
		{"telegram/web", []string{"/telegram/web/client"}, "telegram-web-1"},
	} {
		if got := telemtIngressSuggestedPath(tc.path, tc.protected); got != tc.want {
			t.Errorf("path %q protected %v: got %q, want %q", tc.path, tc.protected, got, tc.want)
		}
	}
}

func TestTelemtIngressPreviewPreservesSettingsAndReservations(t *testing.T) {
	setupConflictDB(t)
	t.Setenv("XUI_PORT", "")
	service := &SettingService{}
	for k, v := range map[string]string{
		"webDomain": "panel.example.com", "webPort": "443", "webListen": "127.0.0.1",
		"webBasePath": "/telegram/", "subEnable": "true", "subPort": "443",
		"subPublicURL": "https://sub.example.com", "subPath": "/sub/",
	} {
		if err := service.setString(k, v); err != nil {
			t.Fatal(err)
		}
	}
	seedInboundConflict(t, "reserved-15080", "127.0.0.1", 15080, model.VLESS, "{}", "{}")
	backendPort, err := pickTelemtWebManagedPort(context.Background(), 8443)
	if err != nil {
		t.Skipf("no available local test socket: %v", err)
	}
	if backendPort == 15080 {
		t.Fatal("allocator ignored saved reservation")
	}
	state := TelemtWebProxyState{Domain: "web.example.com", BasePath: "telegram/web", SharedIngress: &TelemtWebIngress{Enabled: true}}
	preview, err := buildTelemtIngressPreview(context.Background(), state, TelemtWebProxyState{}, "", "", 8443, backendPort)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(preview.SuggestedAliases, ",") != "panel.example.com,sub.example.com" || preview.SuggestedBasePath != "telegram-web-1" {
		t.Fatalf("incorrect recommendations: %+v", preview)
	}
	if preview.SuggestedPanelPort != 0 && preview.SuggestedPanelPort == preview.SuggestedSubscriptionPort {
		t.Fatal("suggested the same port twice")
	}
	if port, err := service.GetPort(); err != nil || port != 443 {
		t.Fatalf("preview changed actual panel port: %d %v", port, err)
	}
	if path, err := service.GetBasePath(); err != nil || path != "/telegram/" {
		t.Fatalf("preview changed actual panel path: %s %v", path, err)
	}
}

func TestTelemtIngressPreviewDoesNotSuggestOverriddenPanelPort(t *testing.T) {
	setupConflictDB(t)
	t.Setenv("XUI_PORT", "443")
	state := TelemtWebProxyState{Domain: "web.example.com", BasePath: "telegram/web", SharedIngress: &TelemtWebIngress{Enabled: true}}
	preview, err := buildTelemtIngressPreview(t.Context(), state, TelemtWebProxyState{}, "", "", 8443, 15080)
	if err != nil {
		t.Fatal(err)
	}
	if preview.PanelPort != 443 || preview.SuggestedPanelPort != 0 {
		t.Fatalf("cannot migrate an environment-controlled port: %+v", preview)
	}
	if !strings.Contains(strings.Join(preview.Manual, ","), "panel-env-port") {
		t.Fatal("missing instruction to change the environment override")
	}
}
