package service

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/protocolserver"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestProtocolServerSelectionFollowsInstallation(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	for _, protocol := range []model.Protocol{model.NaiveProxy, model.Hysteria} {
		if !protocolserver.Supported(protocol, runtime.GOOS, runtime.GOARCH) {
			continue
		}
		native := map[string]any{"type": "naive", "tag": "optional", "listen": "0.0.0.0", "listen_port": 443, "users": []map[string]any{{"username": "alice", "name": "alice", "password": "secret"}}, "tls": map[string]any{"certificate_path": "/cert", "key_path": "/key"}}
		if protocol == model.Hysteria {
			native["type"] = "hysteria2"
		}
		build := func() *singbox.Config {
			return &singbox.Config{Inbounds: []map[string]any{native}, Route: map[string]any{}}
		}
		cfg := build()
		if err := prepareProtocolServers(cfg); err != nil {
			t.Fatal(err)
		}
		if len(cfg.ProtocolServers) != 0 || cfg.Inbounds[0]["type"] != native["type"] {
			t.Fatal("missing binary changed native ownership")
		}
		path := protocolserver.BinaryPath(protocol)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		cfg = build()
		if err := prepareProtocolServers(cfg); err != nil {
			t.Fatal(err)
		}
		if len(cfg.ProtocolServers) != 1 || cfg.Inbounds[0]["type"] != "socks" || cfg.Inbounds[0]["tag"] != "optional" || cfg.Inbounds[0]["listen"] != "127.0.0.1" {
			t.Fatalf("installed source not applied: %+v", cfg)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		cfg = build()
		if err := prepareProtocolServers(cfg); err != nil {
			t.Fatal(err)
		}
		if len(cfg.ProtocolServers) != 0 || cfg.Inbounds[0]["type"] != native["type"] {
			t.Fatal("removing binary did not restore native owner")
		}
	}
}

func TestProtocolServerCheckpointDoesNotOverwriteNativeCounters(t *testing.T) {
	setupBulkDB(t)
	phase6TrafficRows(t)
	svc := NativeCore(CoreTypeHiddify)
	native := phase6Snapshot(100)
	standalone := []singbox.V2RayStat{{Name: "user>>>alice>>>traffic>>>uplink", Value: 20}}
	for range 2 {
		if err := svc.commitHiddifySnapshot("native", native); err != nil {
			t.Fatal(err)
		}
		if err := svc.commitHiddifySnapshot("server", standalone, "hysteria:example:"); err != nil {
			t.Fatal(err)
		}
	}
	phase6AssertUsage(t, 131, 107)
}

func TestProtocolServerRejectedInstallRestoresBinaryAndServingCore(t *testing.T) {
	svc, starts, _ := stage5ProcessFixture(t)
	seedHiddifyConfigInbound(t, model.NaiveProxy, `{"tls":{"certificatePath":"/cert","keyPath":"/key"}}`, `{"network":"tcp","security":"none"}`)
	if err := svc.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := stage5Starts(t, starts)
	err := InstallProtocolServer(context.Background(), model.NaiveProxy, func(context.Context) error {
		path := protocolserver.BinaryPath(model.NaiveProxy)
		return os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o700)
	})
	if err == nil || !strings.Contains(err.Error(), "validate standalone Naive") {
		t.Fatalf("invalid installation result: %v", err)
	}
	if _, err := os.Stat(protocolserver.BinaryPath(model.NaiveProxy)); !os.IsNotExist(err) {
		t.Fatalf("failed binary retained: %v", err)
	}
	if !svc.IsRunning() || stage5Starts(t, starts) != before {
		t.Fatal("installation stopped the core")
	}
}

func TestProtocolServerRejectsUnsupportedUserRouting(t *testing.T) {
	cfg := &singbox.Config{ProtocolServers: []protocolserver.Plan{{Protocol: model.Hysteria, Tag: "hy"}}, Route: map[string]any{"rules": []any{map[string]any{"auth_user": []string{"alice"}, "outbound": "proxy"}}}}
	if err := validateHysteriaUserRoutes(cfg); err == nil || !strings.Contains(err.Error(), "authenticated user") {
		t.Fatalf("lost user routing: %v", err)
	}
}

func TestProtocolServerInstallSwitchesListenerAndRemovalRestoresNative(t *testing.T) {
	binaryPath := os.Getenv("CADDY_NAIVE_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("set CADDY_NAIVE_TEST_BINARY for official listener switching")
	}
	if !protocolserver.Supported(model.NaiveProxy, runtime.GOOS, runtime.GOARCH) {
		t.Skip("standalone Naive platform")
	}
	svc, _, _ := stage5ProcessFixture(t)
	t.Cleanup(func() { _ = protocolserver.StopAll() })
	cert, key := writeTestCertificate(t)
	settings, _ := json.Marshal(map[string]any{"tls": map[string]any{"certificatePath": cert, "keyPath": key}})
	inbound := seedHiddifyConfigInbound(t, model.NaiveProxy, string(settings), `{"network":"tcp","security":"none"}`)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	if err := database.GetDB().Model(inbound).Update("port", port).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := InstallProtocolServer(context.Background(), model.NaiveProxy, func(context.Context) error {
		return os.Symlink(binaryPath, protocolserver.BinaryPath(model.NaiveProxy))
	}); err != nil {
		t.Fatal(err)
	}
	if !protocolserver.Running(model.NaiveProxy) {
		t.Fatal("installed server was not started")
	}
	assertType := func(want string) {
		t.Helper()
		var cfg singbox.Config
		if err := json.Unmarshal(svc.process().AppliedConfig(), &cfg); err != nil {
			t.Fatal(err)
		}
		for _, row := range cfg.Inbounds {
			if row["tag"] == inbound.Tag {
				if row["type"] != want {
					t.Fatalf("listener type %v, want %s", row["type"], want)
				}
				return
			}
		}
		t.Fatal("managed listener missing")
	}
	assertType("socks")
	if err := os.Remove(protocolserver.BinaryPath(model.NaiveProxy)); err != nil {
		t.Fatal(err)
	}
	svc.ApplyPendingRestart(context.Background())
	assertType("naive")
	if protocolserver.Running(model.NaiveProxy) {
		t.Fatal("removed server still owns the listener")
	}
}

func TestProtocolServerLaunchFailureRestoresNativeOwner(t *testing.T) {
	if !protocolserver.Supported(model.NaiveProxy, runtime.GOOS, runtime.GOARCH) {
		t.Skip("standalone Naive platform")
	}
	svc, _, _ := stage5ProcessFixture(t)
	seedHiddifyConfigInbound(t, model.NaiveProxy, `{"tls":{"certificatePath":"/cert","keyPath":"/key"}}`, `{"network":"tcp","security":"none"}`)
	if err := svc.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := InstallProtocolServer(context.Background(), model.NaiveProxy, func(context.Context) error {
		return os.WriteFile(protocolserver.BinaryPath(model.NaiveProxy), []byte("#!/bin/sh\nif [ \"$1\" = validate ]; then exit 0; fi\nexit 7\n"), 0o700)
	})
	if err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("launch failure: %v", err)
	}
	if !svc.IsRunning() || protocolserver.Running(model.NaiveProxy) {
		t.Fatal("failed installation changed the serving owner")
	}
	if _, err := os.Stat(protocolserver.BinaryPath(model.NaiveProxy)); !os.IsNotExist(err) {
		t.Fatalf("failed binary retained: %v", err)
	}
	if !strings.Contains(string(svc.process().AppliedConfig()), `"type": "naive"`) {
		t.Fatal("native listener was not restored")
	}
}

func TestProtocolServerPreservesSourceSensitiveRouting(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rule     map[string]any
		rejected bool
	}{
		{"matching source", map[string]any{"inbound": []string{"naive"}, "source_ip_cidr": []string{"192.0.2.0/24"}}, true},
		{"another inbound", map[string]any{"inbound": []string{"other"}, "source_ip_cidr": []string{"192.0.2.0/24"}}, false},
		{"inactive option", map[string]any{"source_ip_is_private": false}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &singbox.Config{ProtocolServers: []protocolserver.Plan{{Protocol: model.NaiveProxy, Tag: "naive"}}, Route: map[string]any{"rules": []any{tc.rule}}}
			err := validateHysteriaUserRoutes(cfg)
			if (err != nil) != tc.rejected {
				t.Fatalf("routing compatibility: %v", err)
			}
		})
	}
}

func TestProtocolServerUpgradeFallsBackWithoutDroppingPolicy(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	if !protocolserver.Supported(model.Hysteria, runtime.GOOS, runtime.GOARCH) {
		t.Skip("unsupported platform")
	}
	if err := os.WriteFile(protocolserver.BinaryPath(model.Hysteria), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, strict := range []bool{false, true} {
		cfg := &singbox.Config{
			Inbounds: []map[string]any{{"type": "hysteria2", "tag": "hy", "listen_port": 443, "users": []map[string]any{{"name": "alice", "password": "secret"}}, "tls": map[string]any{"certificate_path": "/cert", "key_path": "/key"}}},
			Route:    map[string]any{"rules": []map[string]any{{"auth_user": []string{"alice"}, "outbound": "direct"}}},
		}
		strictProtocol := model.Protocol("")
		if strict {
			strictProtocol = model.Hysteria
		}
		err := prepareProtocolServersWithPolicy(cfg, strictProtocol)
		if strict {
			if err == nil {
				t.Fatal("new install accepted incompatible user routing")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.ProtocolServers) != 0 || cfg.Inbounds[0]["type"] != "hysteria2" || !strings.Contains(cfg.ProtocolServerFallbacks["hy"], "authenticated user") {
			t.Fatalf("upgrade did not preserve native routing: %+v", cfg)
		}
	}
}

func TestProtocolServerInstallPreservesOtherProtocolFallback(t *testing.T) {
	if !protocolserver.Supported(model.NaiveProxy, runtime.GOOS, runtime.GOARCH) {
		t.Skip("unsupported platform")
	}
	svc, _, _ := stage5ProcessFixture(t)
	inbound := seedHiddifyConfigInbound(t, model.Hysteria, `{}`, `{"network":"tcp","security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/cert","keyFile":"/key"}]}}`)
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Update("limit_ip", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(protocolserver.BinaryPath(model.Hysteria), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := InstallProtocolServer(context.Background(), model.NaiveProxy, func(context.Context) error {
		return os.WriteFile(protocolserver.BinaryPath(model.NaiveProxy), []byte("#!/bin/sh\nexit 0\n"), 0o700)
	}); err != nil {
		t.Fatalf("unrelated Hysteria fallback blocked Naive installation: %v", err)
	}
	cfg, err := svc.GetConfig()
	if err != nil || cfg.ProtocolServerFallbacks[inbound.Tag] == "" || !svc.IsRunning() {
		t.Fatalf("Hysteria fallback was lost: %v", err)
	}
}

func TestProtocolServerUpdateKeepsExistingCompatibilityFallback(t *testing.T) {
	if !protocolserver.Supported(model.Hysteria, runtime.GOOS, runtime.GOARCH) {
		t.Skip("unsupported platform")
	}
	svc, _, _ := stage5ProcessFixture(t)
	inbound := seedHiddifyConfigInbound(t, model.Hysteria, `{}`, `{"network":"tcp","security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/cert","keyFile":"/key"}]}}`)
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Update("limit_ip", 1).Error; err != nil {
		t.Fatal(err)
	}
	path := protocolserver.BinaryPath(model.Hysteria)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	replacement := []byte("#!/bin/sh\n# updated version\nexit 0\n")
	if err := InstallProtocolServer(context.Background(), model.Hysteria, func(context.Context) error {
		return os.WriteFile(path, replacement, 0o700)
	}); err != nil {
		t.Fatalf("existing fallback blocked binary update: %v", err)
	}
	cfg, err := svc.GetConfig()
	if err != nil || cfg.ProtocolServerFallbacks[inbound.Tag] == "" || !svc.IsRunning() {
		t.Fatalf("existing native fallback lost: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(replacement) {
		t.Fatalf("updated binary not retained: %v", err)
	}
}
