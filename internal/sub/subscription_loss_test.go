package sub

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	wgutil "github.com/SawaMEN/3x-ui/v3/internal/util/wireguard"
)

func TestExternalSubscriptionKeepsOpaqueTokens(t *testing.T) {
	want := []string{"fptn:eyJ0b2tlbiI6InNlY3JldCJ9", "anytls://secret@vpn.example:443", "sudoku://payload", "custom-vpn://payload", "anytls://secret@vpn.example:443#My node", "http://user:pass@proxy.example:8080"}
	plain := strings.Join(want, "\n")
	for _, body := range []string{plain, base64.StdEncoding.EncodeToString([]byte(plain)), base64.RawURLEncoding.EncodeToString([]byte(plain))} {
		if got := decodeSubscriptionBody([]byte(body)); !reflect.DeepEqual(got, want) {
			t.Fatalf("links = %v, want %v", got, want)
		}
	}
}

func TestExternalSubscriptionInvalidResponseRetainsSnapshot(t *testing.T) {
	for _, body := range []string{`<!doctype html><a href="https://login.example">Sign in</a>`, "provider temporarily unavailable", base64.StdEncoding.EncodeToString([]byte("<html>captcha</html>"))} {
		t.Run(body, func(t *testing.T) {
			resetSubscriptionCache(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer srv.Close()
			want := []string{"anytls://secret@old.example:443"}
			subscriptionCache.Lock()
			subscriptionCache.m[srv.URL] = subscriptionCacheEntry{links: want, fetchedAt: time.Now().Add(-subscriptionCacheTTL)}
			subscriptionCache.Unlock()
			got := fetchSubscriptionLinks(srv.URL)
			if !errors.Is(got.err, errInvalidSubscriptionBody) || !reflect.DeepEqual(got.links, want) {
				t.Fatalf("invalid refresh lost snapshot: %+v", got)
			}
		})
	}
}

func TestHiddifyMixedWireGuardKeepsEveryEndpoint(t *testing.T) {
	initSubDB(t)
	db := database.GetDB()
	serverKey, _, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, _, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	rec := &model.ClientRecord{Email: "mixed-wg", SubID: "mixed-wg", Password: "tls-pass", Enable: true}
	if err := db.Create(rec).Error; err != nil {
		t.Fatal(err)
	}
	for i, protocol := range []model.Protocol{model.AnyTLS, model.WireGuard, model.WireGuard} {
		settings := `{"tls":{"serverName":"vpn.example"},"clients":[{"email":"mixed-wg","password":"tls-pass","enable":true}]}`
		if protocol == model.WireGuard {
			settings = `{"secretKey":"` + serverKey + `","clients":[{"email":"mixed-wg","privateKey":"` + clientKey + `","allowedIPs":["10.0.0.2/32"],"enable":true}]}`
		}
		ib := &model.Inbound{UserId: 1, Tag: strings.Repeat("x", i+1), Protocol: protocol, Enable: true, Port: 51820 + i, Settings: settings}
		if err := db.Create(ib).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: ib.Id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewSubJsonService("", "", "", "", NewSubService(""))
	data, _, err := svc.GetHiddifySingBoxJson(rec.SubID, "vpn.example")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Outbounds []map[string]any `json:"outbounds"`
		Endpoints []map[string]any `json:"endpoints"`
	}
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Endpoints) != 2 || len(cfg.Outbounds) != 1 || cfg.Outbounds[0]["type"] != "anytls" {
		t.Fatalf("missing mixed protocols: %s", data)
	}
	tags := map[any]bool{}
	for _, ep := range cfg.Endpoints {
		if ep["type"] != "wireguard" || ep["private_key"] != clientKey || tags[ep["tag"]] {
			t.Fatalf("lost endpoint identity: %s", data)
		}
		tags[ep["tag"]] = true
		peer := ep["peers"].([]any)[0].(map[string]any)
		if peer["address"] != "vpn.example" {
			t.Fatalf("lost server: %s", data)
		}
	}
	assertOwnerCoreAcceptsSubscription(t, data)
	// Hiddify wireguard-only imports must also be a single native document.
	if err := db.Model(&model.Inbound{}).Where("protocol = ?", model.AnyTLS).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	data, _, err = svc.GetHiddifySingBoxJson(rec.SubID, "vpn.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("Hiddify got an array: %s", data)
	}
	if len(cfg.Endpoints) != 2 {
		t.Fatalf("lost WireGuard-only endpoints: %s", data)
	}
}

func TestHiddifyExtendedProtocolsSurviveMixedSubscription(t *testing.T) {
	initSubDB(t)
	db := database.GetDB()
	secret := strings.Repeat("p", 32)
	serverKey, serverPub, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, _, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	node := &model.Node{Name: "remote", Address: "remote.example", Scheme: "https", Port: 443}
	if err := db.Create(node).Error; err != nil {
		t.Fatal(err)
	}
	record := &model.ClientRecord{Email: "extended", SubID: "extended", Enable: true}
	if err := db.Create(record).Error; err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []model.Protocol{model.Mieru, model.Sudoku, model.TrustTunnel, model.FPTN, model.OpenFlux, model.AmneziaWG} {
		client := map[string]any{"email": record.Email, "password": secret, "enable": true, "privateKey": clientKey, "allowedIPs": []string{"10.0.0.2/32"}, "sudokuPrivateKey": strings.Repeat("c", 128)}
		settings := map[string]any{"clients": []any{client}}
		switch protocol {
		case model.Mieru:
			settings["protocols"] = []string{"TCP", "UDP"}
			settings["additionalPorts"] = []int{2013}
			settings["handshakeMode"] = "HANDSHAKE_STANDARD"
			settings["mtu"] = 1400
		case model.Sudoku:
			settings["httpmask"] = map[string]any{"mode": "auto", "pathRoot": "/mask", "multiplex": "on", "tls": true}
			settings["paddingMin"] = 3
			settings["paddingMax"] = 8
		case model.TrustTunnel:
			settings["hostname"] = "tls.example"
		case model.AmneziaWG:
			settings["server"] = map[string]any{"privateKey": serverKey, "publicKey": serverPub, "jc": 4, "jmin": 40, "jmax": 80, "s1": 12, "s2": 12, "s3": 12, "s4": 12, "h1": "11", "h2": "12", "h3": "13", "h4": "14"}
		}
		raw, _ := json.Marshal(settings)
		ib := &model.Inbound{UserId: 1, Tag: string(protocol), Protocol: protocol, Remark: "Office", Enable: true, Port: 2012, Settings: string(raw), NodeID: &node.Id}
		if protocol == model.FPTN || protocol == model.OpenFlux {
			if err := externalvpn.PrepareAdditional(ib, ""); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Create(ib).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: record.Id, InboundId: ib.Id}).Error; err != nil {
			t.Fatal(err)
		}
		seedHost(t, &model.Host{InboundId: ib.Id, Address: "edge.example", Port: 9443})
	}
	svc := NewSubJsonService("", "", "", "", NewSubService(""))
	data, _, err := svc.GetHiddifySingBoxJson(record.SubID, "sub.example")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Outbounds []map[string]any `json:"outbounds"`
		Endpoints []map[string]any `json:"endpoints"`
	}
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatal(err)
	}
	types := map[any]bool{}
	for _, ob := range cfg.Outbounds {
		types[ob["type"]] = true
	}
	for _, p := range []string{"mieru", "sudoku", "trusttunnel", "fptn", "openflux"} {
		if !types[p] {
			t.Fatalf("missing %s: %s", p, data)
		}
	}
	if len(cfg.Endpoints) != 1 || cfg.Endpoints[0]["type"] != "awg" {
		t.Fatalf("missing AWG: %s", data)
	}
	for _, ob := range cfg.Outbounds {
		if tag, _ := ob["tag"].(string); !strings.HasPrefix(tag, "Office-extended") {
			t.Fatalf("protocol bypassed the shared remark: %s", data)
		}
		if ob["type"] != "openflux" && ob["server"] != "edge.example" {
			t.Fatalf("lost public endpoint: %s", data)
		}
		if ob["type"] == "mieru" {
			for _, raw := range ob["portBindings"].([]any) {
				if raw.(map[string]any)["port"] != float64(9443) {
					t.Fatalf("lost Mieru public port: %s", data)
				}
			}
		}
		if ob["type"] == "sudoku" && (ob["key"] != strings.Repeat("c", 128) || ob["http_mask"].(map[string]any)["path_root"] != "/mask") {
			t.Fatalf("Sudoku options lost: %s", data)
		}
		if ob["type"] == "fptn" && (ob["username"] != externalvpn.FPTNUsername(record.Email) || len(ob["md5_fingerprint"].(string)) != 32) {
			t.Fatalf("FPTN identity lost: %s", data)
		}
	}
	awg := cfg.Endpoints[0]["awg"].(map[string]any)
	if awg["h1"] != "11" || awg["jc"] != float64(4) || cfg.Endpoints[0]["private_key"] != clientKey {
		t.Fatalf("AWG identity/options lost: %s", data)
	}
	seedSubInbound(t, record.SubID, "standard-vless", 9445, 0, dnsTestStream)
	// An advertised AnyTLS endpoint forced to plaintext cannot be rendered.
	// The other Hiddify profiles should still import.
	anyTLS := &model.Inbound{
		UserId: 1, Tag: "tls-unavailable", Protocol: model.AnyTLS, Enable: true, Port: 9446,
		Settings:       `{"clients":[{"email":"extended","enable":true,"password":"test-password"}]}`,
		StreamSettings: `{"externalProxy":[{"dest":"broken.example","port":9446,"forceTls":"none"}]}`,
	}
	if err := db.Create(anyTLS).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: record.Id, InboundId: anyTLS.Id}).Error; err != nil {
		t.Fatal(err)
	}
	router := newSubscriptionTestRouter(subscriptionTestRouterConfig{})
	for _, tc := range []struct {
		path, fallback string
	}{
		{"/sub/extended", "auto"},
		{"/sub/extended", "links"},
		{"/sub/extended", "xray"},
		{"/json/extended", "auto"},
		{"/sub/extended?format=hiddify", "links"},
	} {
		t.Run(tc.path+"/"+tc.fallback, func(t *testing.T) {
			path := tc.path
			importRouter := gin.New()
			NewSUBController(importRouter.Group("/"), WithSUBJsonEnabled(true), WithSUBFormats("both", tc.fallback))
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if strings.HasSuffix(path, "format=hiddify") {
				req.Header.Set("Accept", "text/html")
			} else {
				req.Header.Set("User-Agent", "HiddifyNext/5.9.15 (linux) like ClashMeta v2ray sing-box")
			}
			response := httptest.NewRecorder()
			importRouter.ServeHTTP(response, req)
			var profile struct {
				Outbounds []map[string]any
				Endpoints []map[string]any
			}
			if err := json.Unmarshal(response.Body.Bytes(), &profile); response.Code != http.StatusOK || err != nil {
				t.Fatalf("invalid Hiddify subscription: %d %s", response.Code, response.Body.String())
			}
			want := map[any]bool{"vless": true, "mieru": true, "sudoku": true, "trusttunnel": true}
			for _, proxy := range profile.Outbounds {
				protocol := proxy["type"]
				if !want[protocol] {
					t.Fatalf("unexpected Hiddify outbound %q", protocol)
				}
				delete(want, protocol)
				if protocol == "mieru" && proxy["mtu"] != nil {
					t.Fatalf("Mieru has an unsupported field: %v", proxy)
				}
			}
			if len(want) != 0 || len(profile.Endpoints) != 1 || profile.Endpoints[0]["type"] != "awg" {
				t.Fatalf("missing Hiddify profiles: %s", response.Body.String())
			}
			if path == "/sub/extended" && tc.fallback == "links" {
				assertUpstreamHiddifyParsesSubscription(t, response.Body.Bytes())
			}
		})
	}
	for _, path := range []string{"/sub/extended?format=links", "/sub/extended?format=xray", "/sub/extended?format=sing-box"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if strings.HasSuffix(path, "format=sing-box") {
			req.Header.Set("User-Agent", "sing-box")
		} else {
			req.Header.Set("User-Agent", "HiddifyNext/5.9.15")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		body := response.Body.String()
		if strings.HasSuffix(path, "format=links") {
			body = strings.Join(decodeSubscriptionBody(response.Body.Bytes()), "\n")
		}
		if response.Code != http.StatusOK || !strings.Contains(body, "vless") ||
			strings.Contains(body, "mieru") || strings.Contains(body, "sudoku") {
			t.Fatalf("%s did not keep its selected format: %d %s", path, response.Code, response.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/sub/extended?format=all", nil)
	req.Header.Set("User-Agent", "HiddifyNext/4.0")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	links := decodeSubscriptionBody(response.Body.Bytes())
	if response.Code != http.StatusOK || !strings.Contains(strings.Join(links, "\n"), "sudoku://") {
		t.Fatalf("all profiles omitted Sudoku: %d", response.Code)
	}
	assertOwnerCoreAcceptsSubscription(t, data)
}

func assertOwnerCoreAcceptsSubscription(t *testing.T, data string) {
	t.Helper()
	binary := os.Getenv("HIDDIFY_TEST_BINARY")
	if binary == "" {
		return
	}
	path := filepath.Join(t.TempDir(), "subscription.json")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "check", "-c", path)
	cmd.Dir = filepath.Dir(binary)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("owner core rejected subscription: %v %s", err, out)
	}
}

// Use the official profile parser, which also validates the imported
// configuration. The pinned panel core has a separate check command.
func assertUpstreamHiddifyParsesSubscription(t *testing.T, data []byte) {
	t.Helper()
	binary := os.Getenv("HIDDIFY_UPSTREAM_TEST_BINARY")
	if binary == "" {
		return
	}
	dir := t.TempDir()
	input, output := filepath.Join(dir, "subscription.json"), filepath.Join(dir, "config.json")
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "parse", input, "-o", output)
	cmd.Dir = dir
	if result, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("upstream Hiddify rejected the subscription: %v %s", err, result)
	}
	result, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Outbounds []struct{ Type string }
		Endpoints []struct{ Type string }
	}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}
	imported := make(map[string]bool)
	for _, profile := range append(config.Outbounds, config.Endpoints...) {
		imported[profile.Type] = true
	}
	for _, protocol := range []string{"vless", "mieru", "sudoku", "trusttunnel", "awg"} {
		if !imported[protocol] {
			t.Fatalf("Hiddify lost %s during import", protocol)
		}
	}
}
