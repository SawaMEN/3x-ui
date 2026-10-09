package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestHiddifyConfigFromDatabase(t *testing.T) {
	setupBulkDB(t)
	svc := NativeCore(CoreTypeHiddify)
	if err := singBoxSettingService.SetCoreType(CoreTypeHiddify); err != nil {
		t.Fatal(err)
	}
	if got, _ := singBoxSettingService.GetCoreType(); got != CoreTypeHiddify {
		t.Fatalf("core normalized incorrectly: %s", got)
	}
	ib := &model.Inbound{Enable: true, Protocol: model.VLESS, Tag: "vless-xhttp", Port: 18443,
		Settings: `{"decryption":"none"}`, StreamSettings: `{"network":"xhttp","xhttpSettings":{"path":"/vpn","mode":"stream-up"}}`}
	db := database.GetDB()
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "alice", UUID: "11111111-1111-4111-8111-111111111111", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"inbounds":` + gatewayNativeInbound + `}`); err != nil {
		t.Fatal(err)
	}
	cfg, err := svc.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	outbound, err := svc.translateOutbound(map[string]any{
		"protocol": "vless", "tag": "hiddify-out", "settings": map[string]any{
			"address": "example.com", "port": 443, "id": client.UUID, "encryption": "none"},
		"streamSettings": map[string]any{"network": "xhttp", "xhttpSettings": map[string]any{"path": "/out", "mode": "stream-up", "xPaddingBytes": "100-200"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Outbounds = append(cfg.Outbounds, outbound)
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"type": "xhttp"`) || !strings.Contains(string(data), `"alice"`) || !strings.Contains(string(data), `"in-tproxy"`) {
		t.Fatalf("runtime configuration lost managed sections: %s", data)
	}
	if len(cfg.Services) != 0 {
		t.Fatal("unsupported sing-box 1.14 API injected")
	}
	if _, err := NativeCore(CoreTypeSingBox).GetConfig(); err == nil {
		t.Fatal("stock sing-box incorrectly accepted XHTTP")
	}
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		return
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XUI_BIN_FOLDER", filepath.Dir(binary))
	file := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := hiddify.NewProcess(file, true).Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHiddifyRestartFlagsAreIndependent(t *testing.T) {
	h := NativeCore(CoreTypeHiddify)
	legacy := NativeCore(CoreTypeSingBox)
	h.restartFlag().Store(false)
	legacy.restartFlag().Store(false)
	t.Cleanup(func() { h.restartFlag().Store(false); legacy.restartFlag().Store(false) })
	h.SetToNeedRestart()
	if legacy.IsNeedRestartAndSetFalse() || !h.IsNeedRestartAndSetFalse() {
		t.Fatal("restart flags leak between cores")
	}
	if normalizeNodeCoreType("hiddify-core") != CoreTypeHiddify {
		t.Fatal("node heartbeat loses core identity")
	}
}

func TestHiddifyTrafficSnapshotDoesNotDoubleCount(t *testing.T) {
	inbounds, clients := splitHiddifyStats([]singbox.V2RayStat{
		{Name: "inbound>>>vpn>>>traffic>>>uplink", Value: 100},
		{Name: "inbound>>>vpn>>>traffic>>>downlink", Value: 200},
		{Name: "user>>>alice>>>traffic>>>uplink", Value: 100},
		{Name: "user>>>alice>>>traffic>>>downlink", Value: 200},
		{Name: "outbound>>>direct>>>traffic>>>uplink", Value: 100},
		{Name: "user>>>alice>>>traffic>>>unknown", Value: 999},
		{Name: "user>>>disabled>>>traffic>>>uplink", Value: 0},
	})
	if len(inbounds) != 1 || len(clients) != 1 || inbounds[0].Up != 100 || inbounds[0].Down != 200 || clients[0].Up != 100 || clients[0].Down != 200 {
		t.Fatalf("incorrect accounting: %v %v", inbounds, clients)
	}
}

func TestHiddifyClashPresenceAndDisconnect(t *testing.T) {
	deleted := []string{}
	var deletedMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("Clash authentication missing")
			w.WriteHeader(401)
			return
		}
		if r.Method == http.MethodDelete {
			deletedMu.Lock()
			deleted = append(deleted, r.URL.Path)
			deletedMu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"connections":[{"id":"alice-session","metadata":{"type":"vless/vpn","user":"alice","sourceIP":"192.0.2.1","sourcePort":"1234"}},{"id":"bob-session","metadata":{"type":"vless/vpn","user":"bob","sourceIP":"192.0.2.2","sourcePort":"1234"}}]}`))
	}))
	defer server.Close()
	data, _ := json.Marshal(map[string]any{"experimental": map[string]any{"clash_api": map[string]any{"external_controller": strings.TrimPrefix(server.URL, "http://"), "secret": "test-secret"}}})
	singbox.SetRuntimeConfigReader(func() []byte { return data })
	t.Cleanup(func() {
		singbox.SetRuntimeConfigReader(func() []byte { return SelectedNativeCore().process().AppliedConfig() })
	})
	svc := NativeCore(CoreTypeHiddify)
	online, tags, err := svc.OnlinePresence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := online["alice"]["192.0.2.1"]; !ok || len(tags) != 1 || tags[0] != "vpn" {
		t.Fatalf("presence mapping lost identity: %v %v", online, tags)
	}
	if err := svc.DisconnectClientIPs(context.Background(), "alice", []string{"192.0.2.1"}); err != nil {
		t.Fatal(err)
	}
	deletedMu.Lock()
	defer deletedMu.Unlock()
	if len(deleted) != 1 || deleted[0] != "/connections/alice-session" {
		t.Fatalf("wrong sessions closed: %v", deleted)
	}
}
