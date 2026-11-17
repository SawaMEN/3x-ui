package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

func TestSubscriptionDeleteReplacesReferencesAtomically(t *testing.T) {
	setupBulkDB(t)
	sub := &model.OutboundSubscription{Remark: "test", Url: "https://example.com/sub", TagPrefix: "test", LastFetchedOutbounds: `[{"tag":"node-a","protocol":"socks"}]`}
	if err := database.GetDB().Create(sub).Error; err != nil {
		t.Fatal(err)
	}
	cfg := `{"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"node-a","rules":[{"outbound":"node-a"}]},"dns":{"final":"node-a"}}`
	if err := (&SettingService{}).setString("singBoxConfigTemplate", cfg); err != nil {
		t.Fatal(err)
	}
	svc := &OutboundSubscriptionService{}
	if err := svc.Delete(sub.Id); err == nil {
		t.Fatal("referenced subscription deleted without replacement")
	}
	stored, _ := (&SettingService{}).getString("singBoxConfigTemplate")
	if stored != cfg {
		t.Fatal("failed deletion modified settings")
	}
	if err := svc.DeleteReplacing(sub.Id, "direct"); err != nil {
		t.Fatal(err)
	}
	stored, _ = (&SettingService{}).getString("singBoxConfigTemplate")
	var got map[string]any
	if err := json.Unmarshal([]byte(stored), &got); err != nil {
		t.Fatal(err)
	}
	if got["route"].(map[string]any)["final"] != "direct" {
		t.Fatal("default route not repaired")
	}
	if got["dns"].(map[string]any)["final"] != "node-a" {
		t.Fatal("unrelated DNS tag was changed")
	}
}

func TestEditorIgnoresStaleRuntimeConnections(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	svc := NativeCore(CoreTypeSingBox)
	if err := os.WriteFile(svc.ProcessConfigPath(), []byte(`{"outbounds":[{"type":"direct","tag":"ghost-from-old-runtime"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.GetEditorConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(snapshot.Config)
	if strings.Contains(string(data), "ghost-from-old-runtime") {
		t.Fatal("editor resurrected old process state")
	}
}

func TestStandaloneRemovalProtectsSelectedCoreAndKeepsConfig(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	settings := &SettingService{}
	svc := NativeCore(CoreTypeXray)
	if err := os.WriteFile(svc.BinaryPath(), []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(svc.ProcessConfigPath(), []byte("saved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetCoreType(CoreTypeXray); err != nil {
		t.Fatal(err)
	}
	if err := UninstallStandaloneCore(context.Background(), CoreTypeXray); err == nil {
		t.Fatal("selected core was deleted")
	}
	if err := settings.SetCoreType(CoreTypeSingBox); err != nil {
		t.Fatal(err)
	}
	if err := UninstallStandaloneCore(context.Background(), CoreTypeXray); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(svc.BinaryPath()); !os.IsNotExist(err) {
		t.Fatal("binary remains")
	}
	if b, err := os.ReadFile(filepath.Clean(svc.ProcessConfigPath())); err != nil || string(b) != "saved" {
		t.Fatal("configuration was removed")
	}
}

func TestDomainRedirectUsesPublicPorts(t *testing.T) {
	cfg := rootRedirectDomainConfig(domainfront.DefaultConfig(), &entity.AllSetting{RootRedirectEnable: true, RootRedirectURL: "https://elsewhere.example", WebDomain: "panel.example", WebPort: 2053})
	if !cfg.Enabled || cfg.HTTPPort != 80 || cfg.HTTPSPort != 443 || len(cfg.Domains) != 1 {
		t.Fatalf("domain redirect config: %+v", cfg)
	}
}

func TestNativeCoreTemplatesAreIndependent(t *testing.T) {
	setupBulkDB(t)
	settings := &SettingService{}
	if err := settings.SetCoreType(CoreTypeSingBox); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetSingBoxConfigTemplate(`{"log":{"level":"debug"}}`); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetCoreType(CoreTypeSingBoxUpstream); err != nil {
		t.Fatal(err)
	}
	raw, err := settings.GetSingBoxConfigTemplate()
	if err != nil || raw != "" {
		t.Fatal("owner overlay leaked into upstream", raw, err)
	}
	if err := settings.SetSingBoxConfigTemplate(`{"log":{"level":"warn"}}`); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetCoreType(CoreTypeSingBox); err != nil {
		t.Fatal(err)
	}
	raw, err = settings.GetSingBoxConfigTemplate()
	if err != nil || !strings.Contains(raw, "debug") {
		t.Fatal("owner settings lost", raw, err)
	}
}
