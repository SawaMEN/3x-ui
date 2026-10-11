package controller

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/core"
	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/crypto"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/gin-gonic/gin"
)

// Exercise the actual authenticated node API through the master's Remote
// adapter. Config application is recorded; no privileged core is started.
func TestHiddifyNodeSyncAndTrafficAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	if err := (&service.SettingService{}).SetCoreType(service.CoreTypeHiddify); err != nil {
		t.Fatal(err)
	}
	prev := runtime.GetManager()
	applies := 0
	runtime.SetManager(runtime.NewManager(runtime.LocalDeps{CoreType: func() string { return service.CoreTypeHiddify }, RequestConfigApply: func(context.Context) error { applies++; return nil }}))
	t.Cleanup(func() { runtime.SetManager(prev) })
	token := &model.ApiToken{Name: "stage7-sync", Token: crypto.HashTokenSHA256("stage7-token"), Enabled: true, Scope: model.ApiScopeNodeSync}
	if err := database.GetDB().Create(token).Error; err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	apiController := &APIController{}
	api := engine.Group("/panel/api")
	api.Use(apiController.checkAPIAuth, apiController.enforceTokenScope)
	NewInboundController(api.Group("/inbounds"))
	NewClientController(api.Group("/clients"))
	serverController := &ServerController{}
	statusResponse := httptest.NewRecorder()
	statusContext, _ := gin.CreateTestContext(statusResponse)
	statusContext.Request = httptest.NewRequest("GET", "/panel/api/server/status", nil)
	serverController.status(statusContext)
	var statusEnvelope struct {
		Obj map[string]json.RawMessage `json:"obj"`
	}
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &statusEnvelope); err != nil {
		t.Fatal(err)
	}
	var selected struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(statusEnvelope.Obj["core"], &selected); err != nil {
		t.Fatal(err)
	}
	if selected.Type != "sing-box" || statusEnvelope.Obj["singbox"] == nil || statusEnvelope.Obj["hiddify"] != nil {
		t.Fatalf("owner core must be exposed as sing-box: %s", statusResponse.Body.String())
	}
	api.GET("/server/status", serverController.status)
	server := httptest.NewServer(engine)
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	node := &model.Node{Name: "hiddify", Scheme: "http", Address: u.Hostname(), Port: port, BasePath: "/", ApiToken: "stage7-token", CoreType: service.CoreTypeHiddify, AllowPrivateAddress: true}
	remote := runtime.NewRemote(node, nil)
	patch, err := (&service.NodeService{}).Probe(context.Background(), node)
	if err != nil {
		t.Fatal(err)
	}
	if patch.CoreType != service.CoreTypeHiddify || patch.CoreCapabilities == nil || *patch.CoreCapabilities != core.Hiddify.Capabilities() {
		t.Fatalf("node status capabilities: %#v", patch)
	}
	const uuid = "11111111-1111-4111-8111-111111111111"
	ib := &model.Inbound{Tag: "stage7-xhttp", Enable: true, Port: 45007, Protocol: model.VLESS, Settings: `{"decryption":"none","clients":[{"email":"stage7-node","id":"` + uuid + `","password":"original-password","subId":"original-sub","enable":true}]}`, StreamSettings: `{"network":"xhttp","security":"none","xhttpSettings":{"path":"/original","mode":"stream-up"}}`}
	if err := remote.AddInbound(context.Background(), ib); err != nil {
		t.Fatal(err)
	}
	if applies != 1 {
		t.Fatalf("queued applies = %d", applies)
	}
	var stored model.Inbound
	if err := database.GetDB().Where("tag = ?", ib.Tag).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.Settings, uuid) || !strings.Contains(stored.Settings, "original-sub") || !strings.Contains(stored.StreamSettings, "/original") {
		t.Fatalf("node changed config: %#v", stored)
	}
	var record model.ClientRecord
	if err := database.GetDB().Where("email = ?", "stage7-node").First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.UUID != uuid || record.Password != "original-password" || record.SubID != "original-sub" {
		t.Fatalf("node changed credentials: %#v", record)
	}
	if err := database.GetDB().Model(&coremodel.ClientTraffic{}).Where("inbound_id = ? AND email = ?", stored.Id, record.Email).Updates(map[string]any{"up": 123, "down": 456}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		snapshot, err := remote.FetchTrafficSnapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Inbounds) != 1 || len(snapshot.Inbounds[0].ClientStats) != 1 {
			t.Fatalf("incomplete node stats: %#v", snapshot)
		}
		stats := snapshot.Inbounds[0].ClientStats[0]
		if stats.Up != 123 || stats.Down != 456 {
			t.Fatalf("node counters changed on fetch: %#v", stats)
		}
	}
}
