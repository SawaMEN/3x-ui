package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestDomainPreflightAPIIsReadOnlyAndExplainsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.StopDomainFront(); _ = database.CloseDB() })
	router := gin.New()
	NewDomainController(router.Group("/panel/api"))
	cfg := domainfront.DefaultConfig()
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: "sub.example.org", Mode: "sub_link_only", Enabled: true, TLS: "auto"}}
	cfg.Domains[0].Panel = true
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/panel/api/domains/preflight", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	var payload struct {
		Success bool `json:"success"`
		Obj     struct {
			Ready  bool `json:"ready"`
			Checks []struct {
				Code     string `json:"code"`
				Severity string `json:"severity"`
			} `json:"checks"`
		} `json:"obj"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Success || payload.Obj.Ready || len(payload.Obj.Checks) == 0 {
		t.Fatalf("preflight did not report the invalid draft: %s", res.Body.String())
	}
	var persisted model.Setting
	if err := database.GetDB().Where("key = ?", "domainFrontConfig").First(&persisted).Error; err == nil {
		t.Fatalf("preflight unexpectedly persisted the draft: %+v", persisted)
	}
}
