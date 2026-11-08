package service

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/amneziawgnet"
	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestSingBoxAmneziaWGCascadeBridge(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	ib := amneziawgInbound(7, "awg-cascade", []model.Client{
		{Email: "active", Enable: true, PublicKey: "key-a", AllowedIPs: []string{"10.8.1.2/32"}},
		{Email: "disabled", Enable: false, PublicKey: "key-b", AllowedIPs: []string{"10.8.1.3/32"}},
		{Email: "quota", Enable: true, PublicKey: "key-c", AllowedIPs: []string{"10.8.1.4/32"}},
	})
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&coremodel.ClientTraffic{InboundId: ib.Id, Email: "quota", Enable: false}).Error; err != nil {
		t.Fatal(err)
	}
	seedProtocolCascade(t, ib.Tag)
	cfg, err := (&HiddifyConfigGenerator{}).Generate()
	if err != nil {
		t.Fatal(err)
	}
	assertProtocolCascade(t, cfg, ib.Tag)
	var relay map[string]any
	for _, listener := range cfg.Inbounds {
		if listener["tag"] == ib.Tag {
			relay = listener
		}
	}
	if relay == nil || relay["type"] != "socks" || relay["listen"] != "127.0.0.1" || relay["listen_port"] != amneziawgnet.SOCKSPortForInbound(ib.Id) {
		t.Fatalf("missing/misdirected relay: %v", relay)
	}
	users := relay["users"].([]map[string]any)
	if len(users) != 1 || users[0]["username"] != "active" || users[0]["password"] != amneziawgnet.SocksPassword() {
		t.Fatalf("wrong peer authentication: %v", users)
	}
	raw := checkGeneratedHiddifyConfig(t, cfg)
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	// No network=tcp restriction: native SOCKS must accept UDP ASSOCIATE too.
	if relay["network"] != nil {
		t.Fatal("relay disabled UDP")
	}
	api := wire["experimental"].(map[string]any)["v2ray_api"].(map[string]any)
	stats := api["stats"].(map[string]any)
	if !slices.Contains(readInboundTags(stats["users"]), "active") || !slices.Contains(readInboundTags(stats["inbounds"]), ib.Tag) {
		t.Fatal("core does not own relay user and inbound accounting")
	}
	if slices.Contains(readInboundTags(stats["users"]), "quota") {
		t.Fatal("disabled peer restored in core statistics")
	}
}

func TestSingBoxAmneziaWGRelayCollisions(t *testing.T) {
	ib := amneziawgInbound(7, "awg", []model.Client{{Email: "active", Enable: true, PublicKey: "key", AllowedIPs: []string{"10.8.1.2/32"}}})
	for _, cfg := range []*singbox.Config{
		{Inbounds: []map[string]any{{"tag": "awg", "listen_port": 1234}}},
		{Endpoints: []map[string]any{{"tag": "busy", "listen_port": amneziawgnet.SOCKSPortForInbound(7)}}},
	} {
		if err := injectSingBoxAmneziaWGEgress(cfg, []*model.Inbound{ib}); err == nil {
			t.Fatal("conflicting relay accepted")
		}
	}
	other := *ib
	other.Id += 65535 - amneziawgnet.SOCKSBasePort
	other.Tag = "other"
	if err := injectSingBoxAmneziaWGEgress(&singbox.Config{}, []*model.Inbound{ib, &other}); err == nil {
		t.Fatal("wrapped relay-port collision accepted")
	}
}

func TestSingBoxAmneziaWGSkipsInactiveAndRemote(t *testing.T) {
	for _, kind := range []string{"disabled", "remote", "quota"} {
		ib := amneziawgInbound(7, "awg", []model.Client{{Email: "active", Enable: true, PublicKey: "key", AllowedIPs: []string{"10.8.1.2/32"}}})
		switch kind {
		case "disabled":
			ib.Enable = false
		case "remote":
			id := 1
			ib.NodeID = &id
		case "quota":
			ib.ClientStats = []coremodel.ClientTraffic{{Email: "active", Enable: false}}
		}
		cfg := &singbox.Config{}
		if err := injectSingBoxAmneziaWGEgress(cfg, []*model.Inbound{nil, ib}); err != nil || len(cfg.Inbounds) > 0 {
			t.Fatalf("%s created a relay: %v %v", kind, cfg.Inbounds, err)
		}
	}
}

func TestSingBoxAmneziaWGRelayReservesOtherSidecarSockets(t *testing.T) {
	ib := amneziawgInbound(7, "awg", []model.Client{{Email: "active", Enable: true, PublicKey: "key", AllowedIPs: []string{"10.8.1.2/32"}}})
	other := &model.Inbound{Enable: true, Protocol: model.Mieru, Tag: "sidecar", Port: amneziawgnet.SOCKSPortForInbound(7)}
	if err := injectSingBoxAmneziaWGEgress(&singbox.Config{}, []*model.Inbound{ib, other}); err == nil {
		t.Fatal("sidecar listener port collision accepted")
	}
}
