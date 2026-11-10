package service

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	wgutil "github.com/SawaMEN/3x-ui/v3/internal/util/wireguard"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

func TestSidecarSubscriptionRoutesSurviveNativeTemplate(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Mieru, model.TrustTunnel, model.Pingtunnel} {
		t.Run(string(protocol), func(t *testing.T) {
			setupBulkDB(t)
			t.Setenv("XUI_BIN_FOLDER", t.TempDir())
			ib := &model.Inbound{Enable: true, Protocol: protocol, Tag: "sidecar", Port: 2443, Settings: `{}`, StreamSettings: `{}`}
			if protocol == model.Pingtunnel {
				ib.Port = 0
			}
			if err := database.GetDB().Create(ib).Error; err != nil {
				t.Fatal(err)
			}
			seedProtocolCascade(t, ib.Tag)
			cfg, err := (&HiddifyConfigGenerator{}).Generate()
			if err != nil {
				t.Fatal(err)
			}
			assertProtocolCascade(t, cfg, ib.Tag)
			var bridge map[string]any
			for _, row := range cfg.Inbounds {
				if row["tag"] == ib.Tag {
					bridge = row
				}
			}
			if bridge == nil || bridge["type"] != "socks" || bridge["listen"] != "127.0.0.1" || bridge["listen_port"] != sidecaregress.Port(ib.Id) {
				t.Fatalf("wrong bridge: %v", bridge)
			}
			if bridge["network"] != nil {
				t.Fatal("UDP disabled on bridge")
			}
			checkGeneratedHiddifyConfig(t, cfg)
			api := cfg.Experimental["v2ray_api"].(map[string]any)
			stats := api["stats"].(map[string]any)
			counted := slices.Contains(readInboundTags(stats["inbounds"]), ib.Tag)
			if counted == (protocol == model.Mieru || protocol == model.TrustTunnel) {
				t.Fatal("sidecar and core accounting overlap, or unaccounted protocol")
			}
		})
	}
}

func TestSidecarRelayPortCollisionAndRemote(t *testing.T) {
	ib := &model.Inbound{Id: 1, Enable: true, Protocol: model.Mieru, Tag: "mieru"}
	second := *ib
	second.Id += sidecaregress.Slots
	second.Tag = "second"
	if err := injectSingBoxSidecarEgress(&singbox.Config{}, []*model.Inbound{ib, &second}); err == nil {
		t.Fatal("wrapped relay port collision ignored")
	}
	node := 1
	ib.NodeID = &node
	cfg := &singbox.Config{}
	if err := injectSingBoxSidecarEgress(cfg, []*model.Inbound{ib}); err != nil || len(cfg.Inbounds) > 0 {
		t.Fatal("remote sidecar installed locally")
	}
}

func TestVKTurnLocalTargetsKeepIndependentRoutingIdentity(t *testing.T) {
	for _, kind := range []string{"wireguard", "hysteria2"} {
		t.Run(kind, func(t *testing.T) {
			target := &model.Inbound{Id: 1, Enable: true, Protocol: model.WireGuard, Tag: "target", Port: 51820}
			targetField := "wireguardInboundId"
			forwardType := "wireguardInbound"
			cfg := &singbox.Config{Route: map[string]any{"final": "exit", "rules": []any{map[string]any{"inbound": []string{"vk"}, "outbound": "exit"}}}, Outbounds: []map[string]any{{"type": "direct", "tag": "exit"}}}
			if kind == "wireguard" {
				cfg.Endpoints = []map[string]any{{"type": "wireguard", "tag": "target", "listen_port": 51820, "peers": []any{}}}
			} else {
				target.Protocol = model.Hysteria
				targetField = "hysteria2InboundId"
				forwardType = "hysteria2Inbound"
				cfg.Inbounds = []map[string]any{{"type": "hysteria2", "tag": "target", "listen_port": 51820, "users": []any{map[string]any{"name": "a", "password": "p"}}}}
			}
			ib := &model.Inbound{Id: 2, Enable: true, Protocol: model.VKTurnProxy, Tag: "vk", Port: 53000, Settings: fmt.Sprintf(`{"forward":{"type":%q,%q:1}}`, forwardType, targetField)}
			if err := injectSingBoxSidecarEgress(cfg, []*model.Inbound{target, ib}); err != nil {
				t.Fatal(err)
			}
			rows := cfg.Inbounds
			if kind == "wireguard" {
				rows = cfg.Endpoints
			}
			if len(rows) != 2 || rows[0]["tag"] != "target" || rows[0]["listen_port"] != 51820 || rows[1]["tag"] != "vk" || rows[1]["listen_port"] != sidecaregress.Port(2)+1 {
				t.Fatalf("VK target was not independently cloned: %v", rows)
			}
			for _, raw := range singBoxRouteRules(cfg.Route) {
				rule := raw.(map[string]any)
				if rule["outbound"] == "exit" && slices.Contains(readInboundTags(rule["inbound"]), "target") {
					t.Fatal("VK route captured original listener clients")
				}
			}
		})
	}
}

func TestVKTurnExternalBridgeAndInvalidTarget(t *testing.T) {
	ib := &model.Inbound{Id: 9, Enable: true, Protocol: model.VKTurnProxy, Tag: "vk", Port: 54000, Settings: `{"forward":{"type":"external","host":"203.0.113.7","port":51820}}`}
	cfg := &singbox.Config{}
	if err := injectSingBoxSidecarEgress(cfg, []*model.Inbound{ib}); err != nil {
		t.Fatal(err)
	}
	relay := cfg.Inbounds[0]
	if relay["type"] != "direct" || relay["network"] != "udp" || relay["override_address"] != "203.0.113.7" || relay["listen_port"] != sidecaregress.Port(9)+1 {
		t.Fatal(relay)
	}
	ib.Settings = `{"forward":{"type":"wireguardInbound","wireguardInboundId":77}}`
	blocked := &singbox.Config{}
	if err := injectSingBoxSidecarEgress(blocked, []*model.Inbound{ib}); err != nil {
		t.Fatal(err)
	}
	if len(blocked.Inbounds) != 1 || singBoxRouteRules(blocked.Route)[0].(map[string]any)["action"] != "reject" {
		t.Fatal("unavailable local target did not fail closed")
	}
}

func TestSidecarAccountingSurvivesFinalization(t *testing.T) {
	cfg := &singbox.Config{SidecarTrafficTags: map[string]bool{"mieru": true}, Inbounds: []map[string]any{{"type": "socks", "tag": "mieru", "listen": "127.0.0.1", "listen_port": 60000}, {"type": "socks", "tag": "ordinary", "listen": "127.0.0.1", "listen_port": 60002, "users": []map[string]any{{"username": "shared-user", "password": "p"}}}}, Outbounds: []map[string]any{{"type": "direct", "tag": "direct"}}, Route: map[string]any{"final": "direct"}}
	if err := hiddify.FinalizeServerConfig(cfg); err != nil {
		t.Fatal(err)
	}
	stats := cfg.Experimental["v2ray_api"].(map[string]any)["stats"].(map[string]any)
	if slices.Contains(readInboundTags(stats["inbounds"]), "mieru") || !slices.Contains(readInboundTags(stats["inbounds"]), "ordinary") || !slices.Contains(readInboundTags(stats["users"]), "shared-user") {
		t.Fatalf("wrong accounting: %v", stats)
	}
}

func TestVKTurnWireGuardDatabaseConfigUsesSubscription(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	// The peer private key must never be copied into runtime JSON.
	key, public, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	target := &model.Inbound{Enable: true, Tag: "wireguard-target", Protocol: model.WireGuard, Port: 51820, Settings: fmt.Sprintf(`{"secretKey":%q,"subnetIp":"10.10.0.0","subnetCidr":24,"clients":[{"email":"alice","enable":true,"publicKey":%q,"privateKey":"never-export","allowedIPs":["10.10.0.2/32","fd10::2/128"]}]}`, key, public)}
	if err := database.GetDB().Create(target).Error; err != nil {
		t.Fatal(err)
	}
	vk := &model.Inbound{Enable: true, Tag: "vk-route", Protocol: model.VKTurnProxy, Port: 53000, Settings: fmt.Sprintf(`{"forward":{"type":"wireguardInbound","wireguardInboundId":%d}}`, target.Id)}
	if err := database.GetDB().Create(vk).Error; err != nil {
		t.Fatal(err)
	}
	seedProtocolCascade(t, vk.Tag)
	cfg, err := (&HiddifyConfigGenerator{}).Generate()
	if err != nil {
		t.Fatal(err)
	}
	assertProtocolCascade(t, cfg, vk.Tag)
	if len(cfg.Endpoints) != 2 || cfg.Endpoints[0]["tag"] != "wireguard-target" || cfg.Endpoints[1]["tag"] != "vk-route" {
		t.Fatalf("missing local termination endpoints: %v", cfg.Endpoints)
	}
	data := checkGeneratedHiddifyConfig(t, cfg)
	if strings.Contains(string(data), "never-export") {
		t.Fatal("peer private key leaked")
	}
	specs, err := (&VKTurnProxyService{}).loadDesiredSpecs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if specs[vk.Id].Connect != sidecaregress.UDPAddress(vk.Id) {
		t.Fatalf("relay still bypasses VK's own endpoint: %v", specs)
	}
}

func TestMieruRangeCannotOverlapCoreRelay(t *testing.T) {
	ib := &model.Inbound{Id: 1, Enable: true, Protocol: model.Mieru, Tag: "mieru", Port: 2443, Settings: `{"tcpPorts":["59999-60001"]}`}
	if err := injectSingBoxSidecarEgress(&singbox.Config{}, []*model.Inbound{ib}); err == nil {
		t.Fatal("Mieru port range shadows its own relay")
	}
}

func TestSidecarDisableAndDeleteRefreshCoreBridge(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Mieru, model.TrustTunnel, model.Pingtunnel, model.VKTurnProxy} {
		t.Run(string(protocol), func(t *testing.T) {
			setupConflictDB(t)
			mgr := runtime.NewManager(runtime.LocalDeps{APIPort: func() int { return 0 }})
			mgr.SetLocalRuntimeOverride(&fakeNodeRuntime{})
			runtime.SetManager(mgr)
			t.Cleanup(func() { runtime.SetManager(nil) })
			ib := &model.Inbound{Enable: true, Protocol: protocol, Tag: "lifecycle", Port: 2443, Settings: `{}`, StreamSettings: `{}`}
			if protocol == model.Pingtunnel {
				ib.Port = 0
			}
			if err := database.GetDB().Create(ib).Error; err != nil {
				t.Fatal(err)
			}
			restart, err := (&InboundService{}).SetInboundEnable(ib.Id, false)
			if err != nil || !restart {
				t.Fatalf("disable did not refresh bridge: %v %v", restart, err)
			}
			restart, err = (&InboundService{}).DelInbound(ib.Id)
			if err != nil || !restart {
				t.Fatalf("delete did not refresh bridge: %v %v", restart, err)
			}
		})
	}
}

func TestWireGuardCanonicalDisableOverridesStoredPeer(t *testing.T) {
	key, public, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	ib := &model.Inbound{Tag: "wg", Port: 51820, Settings: fmt.Sprintf(`{"secretKey":%q,"clients":[{"email":"alice","enable":true,"publicKey":%q,"allowedIPs":["10.10.0.2/32"]}]}`, key, public)}
	endpoint, err := singBoxWireGuardInbound(ib, []model.Client{{Email: "alice", Enable: false}})
	if err != nil {
		t.Fatal(err)
	}
	if len(endpoint["peers"].([]map[string]any)) != 0 {
		t.Fatal("stored peer re-enabled a disabled canonical account")
	}
}

func TestOfficialSudokuDoesNotCreateCoreRelay(t *testing.T) {
	inbound := &model.Inbound{Id: 1, Enable: true, Protocol: model.Sudoku, Tag: "sudoku", Port: 2443}
	cfg := &singbox.Config{}
	if routedSidecar(inbound.Protocol) || usesCoreSidecarRelay(inbound) {
		t.Fatal("official Sudoku still classified as a routed sidecar")
	}
	if err := injectSingBoxSidecarEgress(cfg, []*model.Inbound{inbound}); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) != 0 || len(cfg.Endpoints) != 0 || len(cfg.Route) != 0 {
		t.Fatalf("unused core relay generated for official Sudoku: %+v", cfg)
	}
}
