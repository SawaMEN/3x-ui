package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestHiddifyGatewayRuntimeRestartStopAndRepair(t *testing.T) {
	svc, starts := stage8GatewayFixture(t)
	network, host, cfg := newGatewayTestHost(t)
	svc.gateway = network
	ctx := context.Background()
	if err := network.Enable(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(network.paths.rpFilter)
	removed := 0
	network.input = func(ctx context.Context, input, name string, args ...string) error {
		if !strings.Contains(strings.Join(args, " "), "-c") && strings.HasPrefix(input, "delete table") && !strings.Contains(input, "tproxy") {
			if !svc.IsRunning() {
				t.Error("interception removed after listener stop")
			}
			removed++
		}
		return host.input(ctx, input, name, args...)
	}
	if err := svc.Restart(ctx); err != nil {
		t.Fatal(err)
	}
	if removed != 1 || stage5Starts(t, starts) != 3 {
		t.Fatalf("restart transition: removals=%d", removed)
	}
	assertGatewayHealthy(t, network)
	after, _ := os.ReadFile(network.paths.rpFilter)
	if string(before) != string(after) {
		t.Fatal("restart replaced recovery baseline")
	}
	if err := svc.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := network.loadGatewayNetworkState()
	if err != nil {
		t.Fatal(err)
	}
	if host.firewall != "" || !state.DesiredActive {
		t.Fatal("stop leaked interception or erased operator intent")
	}
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	assertGatewayHealthy(t, network)
	host.firewall = ""
	if err := svc.ReconcileGateway(ctx); err != nil {
		t.Fatal(err)
	}
	assertGatewayHealthy(t, network)
	if err := network.Suspend(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReconcileGateway(ctx); err != nil {
		t.Fatal(err)
	}
	if host.firewall != "" {
		t.Fatal("watchdog undid explicit Gateway suspension")
	}
}
func TestHiddifyGatewayFailedRestartRestoresNetwork(t *testing.T) {
	svc, _ := stage8GatewayFixture(t)
	network, host, cfg := newGatewayTestHost(t)
	svc.gateway = network
	if err := network.Enable(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	ib := &model.Inbound{Enable: true, Protocol: model.HTTP, Tag: "stage5-fail", Port: 25088, Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(context.Background()); err == nil {
		t.Fatal("failed candidate started")
	}
	if !svc.IsRunning() || host.firewall == "" {
		t.Fatal("rollback left core or interception stopped")
	}
	assertGatewayHealthy(t, network)
}
func TestHiddifyGatewayResumeFailureLeavesNoInterception(t *testing.T) {
	network, host, cfg := newGatewayTestHost(t)
	if err := network.Enable(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	host.failures["systemctl enable"] = 1
	if err := network.resumeRuntime(context.Background()); err == nil {
		t.Fatal("firewall restore failure ignored")
	}
	if host.firewall != "" {
		t.Fatal("partial restore left interception")
	}
	state, err := network.loadGatewayNetworkState()
	if err != nil || !state.DesiredActive {
		t.Fatal("retry intent lost")
	}
	if err := network.resumeRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertGatewayHealthy(t, network)
}
func TestHiddifyTunGeneratedConfigAndLoopPrevention(t *testing.T) {
	setupBulkDB(t)
	ib := &model.Inbound{Enable: true, Protocol: model.Tun, Tag: "stage8-tun", Settings: `{"name":"hiddify0","gateway":["172.19.0.1/30"],"mtu":1500,"dns":["1.1.1.1"],"autoSystemRoutingTable":["0.0.0.0/0"],"autoOutboundsInterface":"auto"}`, StreamSettings: `{}`}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	svc := NativeCore(CoreTypeHiddify)
	cfg, err := svc.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Route["auto_detect_interface"] != true {
		t.Fatal("TUN core sockets can loop through virtual interface")
	}
	checkGeneratedHiddifyConfig(t, cfg)

	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"outbounds":[{"type":"direct","tag":"direct","bind_interface":"hiddify0"}]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetConfig(); err == nil {
		t.Fatal("outbound can recapture traffic through managed TUN")
	}
	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"route":{"auto_detect_interface":false}}`); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetConfig(); err == nil {
		t.Fatal("template disabled TUN loop prevention")
	}
}

func TestHiddifyGatewayDNSAndTunConflict(t *testing.T) {
	setupBulkDB(t)
	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"inbounds":` + gatewayNativeInbound + `,"route":{"rules":[{"inbound":["in-tproxy"],"action":"sniff","sniffer":["http","tls","quic"]}]}}`); err != nil {
		t.Fatal(err)
	}
	cfg, err := NativeCore(CoreTypeHiddify).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	rule := cfg.Route["rules"].([]any)[0].(map[string]any)
	if rule["action"] != "hijack-dns" {
		t.Fatalf("DNS escapes configured resolver: %#v", cfg.Route)
	}
	checkGeneratedHiddifyConfig(t, cfg)
	tun := &model.Inbound{Enable: true, Protocol: model.Tun, Tag: "auto-tun", Settings: `{"name":"tun0","gateway":["172.19.0.1/30"],"autoSystemRoutingTable":["0.0.0.0/0"]}`, StreamSettings: `{}`}
	if err := database.GetDB().Create(tun).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NativeCore(CoreTypeHiddify).GetConfig(); err == nil || !strings.Contains(err.Error(), "cannot be enabled together") {
		t.Fatalf("conflicting policy routing accepted: %v", err)
	}
}
func TestTunDoesNotReserveSocketPortButKeepsGatewayTag(t *testing.T) {
	setupConflictDB(t)
	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"inbounds":` + gatewayNativeInbound + `}`); err != nil {
		t.Fatal(err)
	}
	tun := &model.Inbound{Enable: true, Protocol: model.Tun, Tag: "tun-local", Port: 52345}
	if conflict, err := checkPortConflictTx(database.GetDB(), tun, 0); err != nil || conflict != nil {
		t.Fatalf("TUN reserved a TCP/UDP socket: %#v %v", conflict, err)
	}
	tun.Tag = "in-tproxy"
	if _, err := checkPortConflictTx(database.GetDB(), tun, 0); err == nil {
		t.Fatal("TUN can steal Gateway tag")
	}
}

func stage8GatewayFixture(t *testing.T) (*SingBoxService, string) {
	t.Helper()
	svc, starts, _ := stage5ProcessFixture(t)
	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"inbounds":` + gatewayNativeInbound + `}`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	return svc, starts
}
func TestHiddifyGatewayRejectsUnownedReadySockets(t *testing.T) {
	svc, _, _ := stage5ProcessFixture(t)
	network, host, cfg := newGatewayTestHost(t)
	svc.gateway = network
	if err := network.Enable(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReconcileGateway(context.Background()); err == nil {
		t.Fatal("foreign sockets accepted as Gateway")
	}
	if host.firewall != "" {
		t.Fatal("unowned listener retained interception")
	}
}

func TestHiddifyGatewayDeadCoreInvalidCandidateRemovesInterception(t *testing.T) {
	svc, _ := stage8GatewayFixture(t)
	network, host, cfg := newGatewayTestHost(t)
	svc.gateway = network
	if err := network.Enable(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := svc.stopProcess(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"route":{"final":"missing"}}`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(context.Background()); err == nil {
		t.Fatal("invalid desired config accepted")
	}
	if host.firewall != "" {
		t.Fatal("invalid candidate kept interception into dead core")
	}
}

func TestHiddifyGatewayRunningCoreMissingSocketsPausesInterception(t *testing.T) {
	svc, _ := stage8GatewayFixture(t)
	network, host, cfg := newGatewayTestHost(t)
	svc.gateway = network
	if err := network.Enable(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	host.listener = false
	if err := svc.ReconcileGateway(context.Background()); err == nil {
		t.Fatal("missing TCP/UDP listener ignored")
	}
	if host.firewall != "" {
		t.Fatal("interception retained without both sockets")
	}
	host.listener = true
	if err := svc.ReconcileGateway(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertGatewayHealthy(t, network)
}
