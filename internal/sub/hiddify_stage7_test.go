package sub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestHiddifySubscriptionXHTTPPreservesIdentity(t *testing.T) {
	initSubDB(t)
	db := database.GetDB()
	const uuid = "11111111-1111-4111-8111-111111111111"
	record := &model.ClientRecord{Email: "stage7", UUID: uuid, Password: "original-password", SubID: "original-sub-id", HiddifySubURI: "https://old.example/ImportedRoute123/", Enable: true}
	if err := db.Create(record).Error; err != nil {
		t.Fatal(err)
	}
	node := &model.Node{Name: "remote-hiddify", Address: "remote.example", Scheme: "https", Port: 443, CoreType: "hiddify-core"}
	if err := db.Create(node).Error; err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []model.Protocol{model.VLESS, model.Trojan} {
		ib := &model.Inbound{UserId: 1, Enable: true, NodeID: &node.Id, Tag: string(protocol) + "-stage7", Protocol: protocol, Port: 18443, Settings: `{"clients":[{"email":"stage7","id":"stale-uuid","password":"stale-password","enable":true}]}`, StreamSettings: `{"network":"tcp","security":"none"}`}
		if protocol == model.VLESS {
			ib.StreamSettings = `{"network":"xhttp","security":"none","xhttpSettings":{"path":"/original","mode":"stream-up"}}`
		}
		if err := db.Create(ib).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: record.Id, InboundId: ib.Id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewSubJsonService("", "", "", "", NewSubService(""))
	data, _, err := svc.GetHiddifySingBoxJson(record.SubID, "sub.example")
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatal(err)
	}
	var vless, trojan bool
	for _, raw := range cfg["outbounds"].([]any) {
		ob := raw.(map[string]any)
		switch ob["type"] {
		case "vless":
			vless = true
			transport := ob["transport"].(map[string]any)
			if ob["uuid"] != uuid || ob["server"] != "remote.example" || transport["type"] != "xhttp" || transport["path"] != "/original" {
				t.Fatalf("VLESS identity/transport lost: %s", data)
			}
		case "trojan":
			trojan = true
			if ob["password"] != "original-password" {
				t.Fatalf("password lost: %s", data)
			}
		}
	}
	if !vless || !trojan {
		t.Fatalf("missing proxies: %s", data)
	}
	if _, _, err := svc.GetSingBoxJson(record.SubID, "sub.example", false); err == nil {
		t.Fatal("stock sing-box accepted XHTTP")
	}
	links, _, _, _, err := NewSubService("").GetSubs(record.SubID, "sub.example")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(links, "\n"), uuid+"@remote.example") {
		t.Fatalf("URI credentials changed: %v", links)
	}
	// Existing URL plus an explicit native format remains importable for a
	// Hiddify client; ordinary URI requests keep their previous format.
	router := newSubscriptionTestRouter(subscriptionTestRouterConfig{})
	request := httptest.NewRequest(http.MethodGet, "/sub/"+record.SubID+"?format=sing-box", nil)
	request.Header.Set("User-Agent", "Hiddify/2.0")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var served map[string]any
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &served) != nil || !strings.Contains(response.Body.String(), `"type": "xhttp"`) {
		t.Fatalf("existing subscription URL failed: %d %s", response.Code, response.Body.String())
	}
	var after model.ClientRecord
	if err := db.First(&after, record.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.UUID != uuid || after.Password != record.Password || after.SubID != record.SubID || after.HiddifySubURI != record.HiddifySubURI {
		t.Fatalf("subscription refresh changed identity: %#v", after)
	}
	if binary := os.Getenv("HIDDIFY_TEST_BINARY"); binary != "" {
		path := filepath.Join(t.TempDir(), "subscription.json")
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "check", "-c", path)
		cmd.Dir = filepath.Dir(binary)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("owner core rejects subscription: %v: %s", err, out)
		}
	}
}

func TestHiddifySubscriptionDuplicateLabels(t *testing.T) {
	proxies := []map[string]any{{"type": "trojan", "tag": "edge", "password": "one"}, {"type": "trojan", "tag": "edge", "password": "two"}, {"type": "trojan", "tag": "edge-2", "password": "three"}}
	before, _ := json.Marshal(proxies)
	data, err := buildHiddifySingBoxSubscription(proxies)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, raw := range cfg["outbounds"].([]any) {
		tags = append(tags, raw.(map[string]any)["tag"].(string))
	}
	if !reflect.DeepEqual(tags, []string{"edge", "edge-3", "edge-2"}) {
		t.Fatalf("tags = %v", tags)
	}
	after, _ := json.Marshal(proxies)
	if string(before) != string(after) {
		t.Fatal("stored labels mutated")
	}
}
