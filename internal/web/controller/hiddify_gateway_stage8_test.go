package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/gin-gonic/gin"
)

func TestHiddifyGatewayWebStatusUsesSelectedCore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	settings := &service.SettingService{}
	if err := settings.SetCoreType(service.CoreTypeHiddify); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetSingBoxConfigTemplate(`{"inbounds":[{"type":"tproxy","tag":"in-tproxy","listen":"0.0.0.0","listen_port":52345}],"route":{"rules":[{"inbound":["in-tproxy"],"action":"sniff","sniffer":["http","tls","quic"]}]}}`); err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	NewGatewayController(engine.Group("/panel/api"))
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panel/api/gateway/status", nil))
	var envelope struct {
		Success bool `json:"success"`
		Obj     struct {
			CoreType   string `json:"coreType"`
			Owner      string `json:"gatewayCoreType"`
			Configured bool   `json:"configured"`
			Mismatch   bool   `json:"coreMismatch"`
		} `json:"obj"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Success || envelope.Obj.CoreType != service.CoreTypeHiddify || envelope.Obj.Owner != service.CoreTypeHiddify || !envelope.Obj.Configured || envelope.Obj.Mismatch {
		t.Fatalf("web Gateway misidentifies Hiddify: %s", response.Body.String())
	}
}
