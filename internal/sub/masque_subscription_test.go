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
