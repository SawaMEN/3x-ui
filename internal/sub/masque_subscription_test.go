package sub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestMASQUEFullSubscription(t *testing.T) {
	setupInfoNodeTestDB(t)
	db := database.GetDB()
	inbound := &model.Inbound{Id: 1, UserId: 1, Enable: true, Protocol: model.MASQUE, Port: 18443, Settings: `{"version":[1,2],"path":"/tunnel{?target,ipproto}","tls":{"serverName":"tls.example"},"clients":[{"email":"alice","password":"stale","subId":"masque-sub","enable":true}]}`}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	record := &model.ClientRecord{Id: 1, Email: "alice", Password: "current", SubID: "masque-sub", Enable: true}
	if err := db.Create(record).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{InboundId: 1, ClientId: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&coremodel.ClientTraffic{InboundId: 1, Email: "alice", Enable: true}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewSubJsonService("", "", "", "", NewSubService(""))
	for _, hiddify := range []bool{false, true} {
		var data string
		var err error
		if hiddify {
			data, _, err = svc.GetHiddifySingBoxJson("masque-sub", "vpn.example")
		} else {
			data, _, err = svc.GetSingBoxJson("masque-sub", "vpn.example", false)
		}
		if err != nil {
			t.Fatal(err)
		}
		var cfg map[string]any
		if err := json.Unmarshal([]byte(data), &cfg); err != nil {
			t.Fatalf("invalid subscription %q: %v", data, err)
		}
		eps, ok := cfg["endpoints"].([]any)
		if !ok || len(eps) != 1 {
			t.Fatalf("missing MASQUE endpoint: %s", data)
		}
		ep := eps[0].(map[string]any)
		if ep["password"] != "current" || ep["username"] != "alice" || ep["server"] != "vpn.example" || ep["server_port"] != float64(18443) || ep["version"] != float64(2) || ep["path"] != "/tunnel{?target,ipproto}" {
			t.Fatalf("runtime and subscription disagree: %s", data)
		}
		if ep["tls"].(map[string]any)["server_name"] != "tls.example" {
			t.Fatalf("lost SNI: %s", data)
		}
		if !hiddify && cfg["route"].(map[string]any)["final"] != ep["tag"] {
			t.Fatalf("missing endpoint route: %s", data)
		}
		if binary := os.Getenv("SINGBOX_TEST_BINARY"); binary != "" && !hiddify {
			path := filepath.Join(t.TempDir(), "subscription.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "check", "-c", path)
			cmd.Dir = filepath.Dir(binary)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("generated MASQUE subscription: %v: %s", err, output)
			}
		}
	}
	router := newSubscriptionTestRouter(subscriptionTestRouterConfig{})
	for _, agent := range []string{"Hiddify/4.0", "sing-box/1.15.0", "NekoBox/1.5"} {
		for _, path := range []string{"/sub/masque-sub", "/json/masque-sub"} {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("User-Agent", agent)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("%s %s returned %d: %s", agent, path, response.Code, response.Body.String())
			}
			var configs []map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &configs); err != nil {
				var cfg map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &cfg); err != nil {
					t.Fatalf("%s %s did not return native JSON: %s", agent, path, response.Body.String())
				}
				configs = []map[string]any{cfg}
			}
			if len(configs) != 1 {
				t.Fatalf("%s %s exported %d profiles", agent, path, len(configs))
			}
			endpoints, _ := configs[0]["endpoints"].([]any)
			if len(endpoints) != 1 || endpoints[0].(map[string]any)["type"] != "masque-client" || endpoints[0].(map[string]any)["password"] != "current" {
				t.Fatalf("%s %s lost MASQUE credentials: %s", agent, path, response.Body.String())
			}
		}
	}
	// A MASQUE-only subscriber must still be able to open the subscription
	// page, even though there is no portable raw share URI for the protocol.
	page := httptest.NewRecorder()
	router.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/sub/masque-sub?html=1", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("MASQUE-only subscription page HTTP %d: %s", page.Code, page.Body.String())
	}
	// A disabled shared identity must not retain a CONNECT-IP profile.
	if err := db.Model(record).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	data, _, err := svc.GetSingBoxJson("masque-sub", "vpn.example", false)
	if err != nil {
		t.Fatal(err)
	}
	if data != "" {
		t.Fatalf("disabled identity still exported: %s", data)
	}
}

func TestMASQUEMixedSubscriptionDefaultAndFormats(t *testing.T) {
	setupInfoNodeTestDB(t)
	db := database.GetDB()
	const subID = "masque-mixed"
	const uuid = "11111111-2222-4333-8444-555555555555"
	client := &model.ClientRecord{Email: "alice", SubID: subID, UUID: uuid, Password: "shared-secret", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	inbounds := []*model.Inbound{
		{
			UserId: 1, Tag: "mixed-masque", Enable: true, Protocol: model.MASQUE, Port: 18443,
			ShareAddrStrategy: "custom", ShareAddr: "masque.example.com",
			Settings: `{"version":[1,2],"path":"/tunnel{?target,ipproto}","tls":{"serverName":"masque.example.com"},"clients":[{"email":"alice","password":"shared-secret","subId":"masque-mixed","enable":true}]}`,
		},
		{
			UserId: 1, Tag: "mixed-vless", Enable: true, Protocol: model.VLESS, Port: 18444,
			ShareAddrStrategy: "custom", ShareAddr: "vless.example.com",
			Settings:       `{"decryption":"none","clients":[{"email":"alice","id":"11111111-2222-4333-8444-555555555555","subId":"masque-mixed","enable":true}]}`,
			StreamSettings: `{"network":"tcp","security":"none"}`,
		},
	}
	for _, inbound := range inbounds {
		if err := db.Create(inbound).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: inbound.Id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&coremodel.ClientTraffic{InboundId: inbounds[0].Id, Email: client.Email, Enable: true}).Error; err != nil {
		t.Fatal(err)
	}
	router := newSubscriptionTestRouter(subscriptionTestRouterConfig{})
	for _, path := range []string{
		"/sub/" + subID,
		"/sub/" + subID + "?format=sing-box",
		"/sub/" + subID + "?format=all",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("User-Agent", "curl/8.0")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				t.Fatalf("HTTP %d: %s", resp.Code, resp.Body.String())
			}
			var docs []map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &docs); err != nil {
				t.Fatalf("expected native multi-profile JSON: %v: %s", err, resp.Body.String())
			}
			if len(docs) != 2 {
				t.Fatalf("expected MASQUE and VLESS, got %d: %s", len(docs), resp.Body.String())
			}
			masque, vless := false, false
			for _, doc := range docs {
				endpoints, _ := doc["endpoints"].([]any)
				for _, raw := range endpoints {
					if raw.(map[string]any)["type"] == "masque-client" {
						masque = true
					}
				}
				for _, raw := range doc["outbounds"].([]any) {
					if raw.(map[string]any)["type"] == "vless" {
						vless = true
					}
				}
			}
			if !masque || !vless {
				t.Fatalf("native subscription lost a selected protocol: %s", resp.Body.String())
			}
		})
	}
	req := httptest.NewRequest(http.MethodGet, "/sub/"+subID+"?format=hiddify", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("Hiddify HTTP %d: %s", resp.Code, resp.Body.String())
	}
	var merged map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &merged); err != nil {
		t.Fatalf("Hiddify requires a single native JSON object: %v: %s", err, resp.Body.String())
	}
	endpoints, _ := merged["endpoints"].([]any)
	outbounds, _ := merged["outbounds"].([]any)
	if len(endpoints) != 1 || endpoints[0].(map[string]any)["type"] != "masque-client" {
		t.Fatalf("Hiddify lost MASQUE: %s", resp.Body.String())
	}
	hasVLESS := false
	for _, raw := range outbounds {
		if raw.(map[string]any)["type"] == "vless" {
			hasVLESS = true
		}
	}
	if !hasVLESS {
		t.Fatalf("Hiddify lost VLESS: %s", resp.Body.String())
	}
	for _, path := range []string{"/sub/" + subID + "?format=links", "/sub/" + subID + "?format=xray"} {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		if resp.Code != http.StatusNotAcceptable {
			t.Fatalf("%s returned HTTP %d instead of 406: %s", path, resp.Code, resp.Body.String())
		}
	}

	// A non-native sidecar must not be silently omitted when MASQUE is selected.
	unsupported := &model.Inbound{
		UserId: 1, Tag: "mixed-mtproto", Enable: true, Protocol: model.MTProto, Port: 18445,
		Settings: `{"clients":[{"email":"alice","subId":"masque-mixed","enable":true}]}`,
	}
	if err := db.Create(unsupported).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: unsupported.Id}).Error; err != nil {
		t.Fatal(err)
	}
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/sub/"+subID+"?format=sing-box", nil))
	if resp.Code != http.StatusNotAcceptable {
		t.Fatalf("mixed unsupported format must fail closed, got HTTP %d: %s", resp.Code, resp.Body.String())
	}
}
