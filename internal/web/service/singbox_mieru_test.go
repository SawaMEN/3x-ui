package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestMieruBridgeUsesInboundTagAndRejectsConflicts(t *testing.T) {
	// Normalized client records do not live in inbound.Settings.
	ib := &model.Inbound{Id: 42, Tag: "mieru-in", Protocol: model.Mieru, Enable: true, Port: 8443, Settings: `{}`}
	cfg := &singbox.Config{}
	if err := injectSingBoxMieruEgress(cfg, []*model.Inbound{ib}); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) != 1 || cfg.Inbounds[0]["tag"] != ib.Tag || cfg.Inbounds[0]["listen"] != "127.0.0.1" || cfg.Inbounds[0]["listen_port"] != mieru.SOCKSPortForInbound(42) {
		t.Fatalf("bad bridge: %v", cfg.Inbounds)
	}
	if err := injectSingBoxMieruEgress(cfg, []*model.Inbound{ib}); err == nil {
		t.Fatal("duplicate bridge accepted")
	}
	other := *ib
	other.Id += 1000
	other.Tag = "second-mieru"
	if err := injectSingBoxMieruEgress(&singbox.Config{}, []*model.Inbound{ib, &other}); err == nil {
		t.Fatal("wrapped bridge port collision accepted")
	}
	public := &model.Inbound{Enable: true, Tag: "public", Port: mieru.SOCKSPortForInbound(42)}
	if err := injectSingBoxMieruEgress(&singbox.Config{}, []*model.Inbound{ib, public}); err == nil {
		t.Fatal("public listener collision accepted")
	}
	ib.Enable = false
	cfg = &singbox.Config{}
	if err := injectSingBoxMieruEgress(cfg, []*model.Inbound{ib}); err != nil || len(cfg.Inbounds) != 0 {
		t.Fatal("disabled bridge installed")
	}
}

func TestMieruGeneratedConfigurationKeepsCascadeRoute(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	ib := seedHiddifyConfigInbound(t, model.Mieru, `{"tcpPorts":["19443"]}`, `{}`)
	template := `{"outbounds":[{"type":"socks","tag":"exit","server":"127.0.0.1","server_port":23456},{"type":"direct","tag":"direct"}],"route":{"rules":[{"inbound":["managed-mieru"],"action":"route","outbound":"exit"}],"final":"exit"}}`
	if err := database.GetDB().Create(&model.Setting{Key: "singBoxConfigTemplate", Value: template}).Error; err != nil {
		t.Fatal(err)
	}
	cfg, err := (&HiddifyConfigGenerator{}).Generate()
	if err != nil {
		t.Fatal(err)
	}
	stats, _ := json.Marshal(cfg.Experimental["v2ray_api"])
	if strings.Contains(string(stats), ib.Tag) || strings.Contains(string(stats), mieru.SocksUsername()) {
		t.Fatal("Mieru bytes would be charged twice by core and Mita collectors")
	}
	found := false
	for _, inbound := range cfg.Inbounds {
		if inbound["tag"] == ib.Tag && inbound["type"] == "socks" {
			found = true
		}
	}
	sniffFound := false
	for _, raw := range singBoxRouteRules(cfg.Route) {
		if rule, ok := raw.(map[string]any); ok && rule["action"] == "sniff" {
			for _, tag := range readInboundTags(rule["inbound"]) {
				if tag == ib.Tag {
					sniffFound = true
				}
			}
		}
	}
	if !sniffFound {
		t.Fatal("Mieru bypasses domain sniffing rules")
	}
	if !found {
		t.Fatal("Mieru is outside core routing")
	}
	raw, _ := json.Marshal(cfg.Route)
	var route struct {
		Final string `json:"final"`
		Rules []struct {
			Inbound  []string `json:"inbound"`
			Outbound string   `json:"outbound"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(raw, &route); err != nil {
		t.Fatal(err)
	}
	if route.Final != "exit" {
		t.Fatalf("cascade default lost: %s", raw)
	}
	for _, r := range route.Rules {
		if len(r.Inbound) == 1 && r.Inbound[0] == ib.Tag && r.Outbound == "exit" {
			return
		}
	}
	t.Fatalf("Mieru cascade rule lost: %s", raw)
}
