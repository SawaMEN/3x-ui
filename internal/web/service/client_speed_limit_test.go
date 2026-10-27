package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/clientlimit"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestClientSpeedLimitPersistenceAndCompatibility(t *testing.T) {
	setupBulkDB(t)
	s := &ClientService{}
	ib := mkInbound(t, 30121, model.VLESS, `{"decryption":"none","clients":[]}`)
	c := model.Client{Email: "limited", ID: "11111111-1111-4111-8111-111111111111", Enable: true, SpeedLimit: &model.ClientSpeedLimit{Enabled: true, UploadMbps: 5, DownloadMbps: 10}}
	if err := s.SyncInbound(nil, ib.Id, []model.Client{c}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListForInbound(nil, ib.Id)
	if err != nil || len(rows) != 1 {
		t.Fatalf("reload: %v %v", rows, err)
	}
	if rows[0].SpeedLimit == nil || *rows[0].SpeedLimit != *c.SpeedLimit {
		t.Fatalf("persisted wrong speed cap: %+v", rows[0].SpeedLimit)
	}
	c.SpeedLimit = nil
	if err = s.SyncInbound(nil, ib.Id, []model.Client{c}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.ListForInbound(nil, ib.Id)
	if err != nil || rows[0].SpeedLimit == nil || rows[0].SpeedLimit.DownloadMbps != 10 {
		t.Fatalf("old API erased cap: %v %v", rows, err)
	}
	exported, err := s.ExportAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(exported) != 1 || exported[0].Client.SpeedLimit == nil || exported[0].Client.SpeedLimit.UploadMbps != 5 {
		t.Fatalf("export lost cap: %+v", exported)
	}
	c.SpeedLimit = &model.ClientSpeedLimit{Enabled: false, UploadMbps: 5, DownloadMbps: 10}
	if err = s.SyncInbound(nil, ib.Id, []model.Client{c}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.ListForInbound(nil, ib.Id)
	if err != nil || rows[0].SpeedLimit.Enabled {
		t.Fatalf("cannot disable cap: %v %v", rows, err)
	}
	c.SpeedLimit = &model.ClientSpeedLimit{Enabled: true, UploadMbps: -1}
	if err = s.SyncInbound(nil, ib.Id, []model.Client{c}); err == nil || !strings.Contains(err.Error(), "between 0 and 100000") {
		t.Fatalf("invalid cap accepted: %v", err)
	}
	unsupported := mkInbound(t, 30122, model.MTProto, `{"clients":[]}`)
	c.SpeedLimit = &model.ClientSpeedLimit{Enabled: true, UploadMbps: 1}
	if err = s.SyncInbound(nil, unsupported.Id, []model.Client{c}); err == nil || !strings.Contains(err.Error(), "not supported for mtproto") {
		t.Fatalf("unsupported protocol silently accepted cap: %v", err)
	}
}

func TestClientSpeedLimitGeneratedConfig(t *testing.T) {
	setupBulkDB(t)
	seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none"}`, `{"network":"tcp","security":"none"}`)
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Update("speed_limit", encodeSpeedLimit(&model.ClientSpeedLimit{Enabled: true, UploadMbps: 1, DownloadMbps: 2})).Error; err != nil {
		t.Fatal(err)
	}
	cfg, err := NativeCore(CoreTypeHiddify).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SpeedLimitPlan.Routes) == 0 {
		t.Fatal("no enforcement routes generated")
	}
	for _, r := range cfg.SpeedLimitPlan.Routes {
		if r.Email != "alice" || r.UploadMbps != 1 || r.DownloadMbps != 2 {
			t.Fatalf("bad limiter plan: %+v", r)
		}
	}
	data := checkGeneratedHiddifyConfig(t, cfg)
	if strings.Contains(string(data), "uploadMbps") {
		t.Fatal("panel fields leaked into core config")
	}
	stats, _ := json.Marshal(cfg.Experimental)
	if strings.Contains(string(stats), clientlimit.InternalPrefix+"return") {
		t.Fatal("return bridge doubles traffic counters")
	}
}

func TestClientSpeedLimitPreservesRouteOrderAndRejects(t *testing.T) {
	cfg := singbox.NewConfig()
	cfg.Inbounds = []map[string]any{{"type": "vless", "tag": "in", "listen_port": 12345, "users": []map[string]any{{"name": "alice"}}}}
	cfg.Route["rules"] = []map[string]any{
		{"domain_suffix": []string{"blocked.test"}, "action": "reject"},
		{"domain_suffix": []string{"proxy.test"}, "action": "route", "outbound": "direct", "override_port": 8443},
		{"action": "hijack-dns", "port": 53},
	}
	clients := map[int][]model.Client{1: {{Email: "alice", Enable: true, SpeedLimit: &model.ClientSpeedLimit{Enabled: true, DownloadMbps: 5}}}}
	if err := injectClientSpeedLimits(cfg, clients, []*model.Inbound{{Id: 1, Enable: true, Protocol: model.VLESS, Tag: "in"}}); err != nil {
		t.Fatal(err)
	}
	rules := singBoxRouteRules(cfg.Route)
	first := len(cfg.SpeedLimitPlan.Routes)
	if rules[first].(map[string]any)["action"] != "reject" {
		t.Fatal("shaping bypasses blocked domains")
	}
	wrapped := rules[first+1].(map[string]any)
	if wrapped["type"] != "logical" || wrapped["outbound"] == "direct" || wrapped["override_port"] != 8443 {
		t.Fatalf("original outbound or route options lost: %v", wrapped)
	}
	predicate := wrapped["rules"].([]map[string]any)[0]
	if _, ok := predicate["override_port"]; ok {
		t.Fatal("action fields leaked into match predicate")
	}
	if rules[first+3].(map[string]any)["action"] != "hijack-dns" {
		t.Fatal("DNS interception order changed")
	}
	if err := injectClientSpeedLimits(singbox.NewConfig(), nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestClientSpeedLimitRecoversListenerWithRunningCore(t *testing.T) {
	svc, starts, _ := stage5ProcessFixture(t)
	defer clientlimit.Default.Close()
	seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none"}`, `{"network":"tcp"}`)
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Update("speed_limit", encodeSpeedLimit(&model.ClientSpeedLimit{Enabled: true, UploadMbps: 1})).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if clientlimit.Default.Address() == "" {
		t.Fatal("limiter not started")
	}
	before := stage5Starts(t, starts)
	clientlimit.Default.Close()
	if !svc.IsRunning() {
		t.Fatal("core unexpectedly stopped")
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if clientlimit.Default.Address() == "" || stage5Starts(t, starts) != before+1 {
		t.Fatal("running core was resumed without restoring its limiter")
	}
}
