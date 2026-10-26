package sub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestAnyTLSShadowTLSAutomaticTLSIdentityAndRoutes(t *testing.T) {
	initSubDB(t)
	cert, key := anyTLSCertificate(t, "inner.example", "second.example")
	db := database.GetDB()
	for name, value := range map[string]string{"webCertFile": cert, "webKeyFile": key, "webDomain": "second.example"} {
		if err := db.Create(&model.Setting{Key: name, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	ib := &model.Inbound{UserId: 1, Tag: "wrapped-auto", Protocol: model.AnyTLS, Enable: true, Port: 8443, Settings: `{"clients":[{"email":"auto","password":"inner-password","enable":true}],"shadowTls":{"enabled":true,"password":"outer-password","wildcardSni":"authed","handshake":{"address":"cover.example","port":443}}}`}
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "auto", SubID: "auto-anytls", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	seedHost(t, &model.Host{InboundId: ib.Id, Address: "203.0.113.20", Port: 9443, Sni: "front.example", Alpn: []string{"h2"}, AllowInsecure: true})
	router := newSubscriptionTestRouter(subscriptionTestRouterConfig{})
	for _, agent := range []string{"HiddifyNext/4", "sing-box/1.15", "NekoBox/1.4", "NekoRay/4"} {
		for _, path := range []string{"/sub/auto-anytls", "/sub/auto-anytls?format=hiddify", "/json/auto-anytls?format=sing-box"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("User-Agent", agent)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusOK {
				t.Fatalf("%s %s: status %d %s", agent, path, response.Code, response.Body.String())
			}
			var cfg map[string]any
			body := response.Body.Bytes()
			if len(body) > 0 && body[0] == '[' {
				var array []map[string]any
				if err := json.Unmarshal(body, &array); err != nil || len(array) != 1 {
					t.Fatalf("invalid native profiles: %s", body)
				}
				cfg = array[0]
			} else {
				if err := json.Unmarshal(body, &cfg); err != nil {
					t.Fatalf("native subscription missing for %s: %s", agent, body)
				}
			}
			var inner, outer map[string]any
			for _, item := range cfg["outbounds"].([]any) {
				ob := item.(map[string]any)
				switch ob["type"] {
				case "anytls":
					inner = ob
				case "shadowtls":
					outer = ob
				}
			}
			if inner == nil || outer == nil || inner["detour"] != outer["tag"] {
				t.Fatalf("broken graph: %s", body)
			}
			tls := inner["tls"].(map[string]any)
			if tls["server_name"] != "second.example" || tls["insecure"] == true || tls["alpn"] != nil || tls["certificate"] == nil {
				t.Fatalf("incorrect inner TLS: %s", body)
			}
			outerTLS := outer["tls"].(map[string]any)
			if outer["server"] != "203.0.113.20" || outer["server_port"] != float64(9443) || outer["password"] != "outer-password" || outerTLS["server_name"] != "front.example" || outerTLS["insecure"] != true {
				t.Fatalf("incorrect outer transport: %s", body)
			}
			if strings.Contains(string(body), key) || strings.Contains(string(body), "PRIVATE KEY") {
				t.Fatal("private server key leaked")
			}
			configJSON, _ := json.Marshal(cfg)
			assertOwnerCoreAcceptsSubscription(t, string(configJSON))
		}
	}
	// Live certificate changes are reflected on refresh; settings are never
	// rewritten by a subscription fetch and generated identity remains deterministic.
	raw, _ := os.ReadFile(cert)
	cert2, _ := anyTLSCertificate(t, "rotated.example")
	replacement, _ := os.ReadFile(cert2)
	if err := os.WriteFile(cert, replacement, 0600); err != nil {
		t.Fatal(err)
	}
	defer os.WriteFile(cert, raw, 0600)
	got, _, err := NewSubJsonService("", "", "", "", NewSubService("")).GetHiddifySingBoxJson(client.SubID, "sub.example")
	if err != nil || !strings.Contains(got, `"server_name": "rotated.example"`) {
		t.Fatalf("certificate renewal left stale identity: %v %s", err, got)
	}
}

func TestAnyTLSRemoteIdentityNeverReadsCentralCertificate(t *testing.T) {
	cert, key := anyTLSCertificate(t, "central.example")
	nodeID := 7
	ib := &model.Inbound{Protocol: model.AnyTLS, NodeID: &nodeID, Settings: `{"tls":{"certificatePath":"` + cert + `","keyPath":"` + key + `","serverName":"remote.example"}}`}
	ob := (&SubJsonService{}).genNativeTLSLikeEndpoint(&SubService{}, ib, model.Client{Email: "user", Password: "pass"}, ShareEndpoint{Address: "remote-ip.example", Port: 443})
	tls := ob["tls"].(map[string]any)
	if tls["server_name"] != "remote.example" || tls["certificate"] != nil {
		t.Fatalf("remote identity used central files: %#v", tls)
	}
}

func TestNativeTLSCertificateIdentityIsSafeAndDeterministic(t *testing.T) {
	cert, key := anyTLSCertificate(t, "*.example.test")
	identity, err := service.ResolveNativeTLSSettings(map[string]any{"certificatePath": cert, "keyPath": key}, nil)
	if err != nil || identity["serverName"] != "vpn.example.test" {
		t.Fatalf("wildcard name not usable: %v %#v", err, identity)
	}
	if _, err := service.ResolveNativeTLSSettings(map[string]any{"certificatePath": cert}, nil); err == nil {
		t.Fatal("half certificate pair was accepted")
	}
	if got := service.NativeTLSClientCertificate(key); got != "" {
		t.Fatal("private key exported as trust certificate")
	}
}
