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
		case model.Sudoku:
			settings["httpmask"] = map[string]any{"mode": "auto", "pathRoot": "/mask", "multiplex": "on"}
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
		if ob["type"] == "sudoku" && (ob["key"] != strings.Repeat("c", 128) || ob["httpmask"].(map[string]any)["path_root"] != "/mask") {
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
	// Existing Hiddify URLs must automatically choose the extended JSON renderer.
	router := newSubscriptionTestRouter(subscriptionTestRouterConfig{})
	for _, path := range []string{"/sub/extended", "/json/extended"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("User-Agent", "HiddifyNext/4.0")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"type": "sudoku"`) {
			t.Fatalf("auto-detection lost extended nodes: %d %s", response.Code, response.Body.String())
		}
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
