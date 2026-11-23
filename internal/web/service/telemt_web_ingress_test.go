package service

import (
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
	for _, path := range []string{"/", "/path/../other", "/path with space", "/path//nested", "/path%2fother"} {
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
	inbound := model.Inbound{Protocol: model.NaiveProxy, Tag: "naive-web", Listen: "127.0.0.1", Port: 9443, Enable: true,
		Settings: `{"network":"tcp","tls":{"serverName":"naive.example.com"}}`}
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
	if plan.panel.address != "127.0.0.1:8443" || plan.subscriptions.address != "[::1]:2096" || plan.naiveAddress != "127.0.0.1:9443" {
		t.Fatalf("unexpected routing plan: %+v", plan)
	}
	publishTelemtIngress(state, plan)
	if origin, err := settings.GetSubPublicURL(); err != nil || origin != "https://web.example.com" {
		t.Fatalf("automatic subscription origin: %q, %v", origin, err)
	}
	if endpoint := telemtIngressNaiveHost(&inbound); endpoint == nil || endpoint.Port != 443 || endpoint.Sni != "naive.example.com" {
		t.Fatalf("automatic Naive endpoint: %+v", endpoint)
	}
	if err := database.GetDB().Model(&inbound).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	plan, err = planTelemtWebIngress(state, false)
	if err != nil || plan.naiveAddress != "" {
		t.Fatalf("disabled Naive backend remained in plan: %+v, %v", plan, err)
	}
	publishTelemtIngress(state, plan)
	if telemtIngressNaiveHost(&inbound) != nil {
		t.Fatal("disabled Naive endpoint is still published")
	}
}
