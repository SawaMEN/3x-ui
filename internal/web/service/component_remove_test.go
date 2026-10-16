package service

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/protocolserver"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestRemoveProtocolComponentFallsBackAndPreservesSettings(t *testing.T) {
	for _, component := range []string{"naiveproxy", "hysteria2"} {
		t.Run(component, func(t *testing.T) {
			svc, _, _ := stage5ProcessFixture(t)
			protocol := model.NaiveProxy
			if component == "hysteria2" {
				protocol = model.Hysteria
			}
			inbound := seedHiddifyConfigInbound(t, protocol, `{"tls":{"certificatePath":"/cert","keyPath":"/key"}}`, `{"network":"tcp","security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/cert","keyFile":"/key"}]}}`)
			path := protocolserver.BinaryPath(protocol)
			if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := RemoveProtocolComponent(context.Background(), component); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("binary retained: %v", err)
			}
			if !svc.IsRunning() || protocolserver.Running(protocol) {
				t.Fatal("native fallback is not serving")
			}
			cfg, err := svc.GetConfig()
			if err != nil || len(cfg.ProtocolServers) != 0 {
				t.Fatalf("standalone still selected: %v", err)
			}
			var applied singbox.Config
			if err := json.Unmarshal(svc.process().AppliedConfig(), &applied); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range applied.Inbounds {
				if row["tag"] == inbound.Tag && row["type"] != "socks" {
					found = true
				}
			}
			if !found {
				t.Fatal("native listener not applied to the running core")
			}
			var count int64
			database.GetDB().Model(&model.Inbound{}).Where("id = ?", inbound.Id).Count(&count)
			if count != 1 {
				t.Fatal("removal deleted panel settings")
			}
		})
	}
}

func TestRemoveProtocolComponentRestoresBinaryOnRejectedFallback(t *testing.T) {
	svc, _, _ := stage5ProcessFixture(t)
	inbound := seedHiddifyConfigInbound(t, model.NaiveProxy, `{"tls":{"certificatePath":"/cert","keyPath":"/key"}}`, `{"network":"tcp","security":"none"}`)
	if err := database.GetDB().Model(inbound).Update("tag", "stage-audit-reject").Error; err != nil {
		t.Fatal(err)
	}
	path := protocolserver.BinaryPath(model.NaiveProxy)
	contents := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(path, contents, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := RemoveProtocolComponent(context.Background(), "naiveproxy"); err == nil {
		t.Fatal("accepted rejected fallback")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(contents) || !svc.IsRunning() {
		t.Fatalf("working installation not restored: %v", err)
	}
}

func TestRemoveProtocolComponentProtectsActiveTunnels(t *testing.T) {
	for _, tc := range []struct {
		name, binary string
		protocol     model.Protocol
	}{
		{"sudoku", "sudoku-tunnel", model.Sudoku}, {"pingtunnel", "pingtunnel", model.Pingtunnel}, {"trusttunnel", "trusttunnel_endpoint", model.TrustTunnel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupBulkDB(t)
			dir := t.TempDir()
			t.Setenv("XUI_BIN_FOLDER", dir)
			path := filepath.Join(dir, tc.binary)
			if err := os.WriteFile(path, []byte("binary"), 0o700); err != nil {
				t.Fatal(err)
			}
			inbound := &model.Inbound{Enable: true, Protocol: tc.protocol, Tag: "active", Settings: "{}", StreamSettings: "{}"}
			if err := database.GetDB().Create(inbound).Error; err != nil {
				t.Fatal(err)
			}
			if err := RemoveProtocolComponent(context.Background(), tc.name); err == nil || !strings.Contains(err.Error(), "disable") {
				t.Fatalf("active tunnel removal: %v", err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("active binary deleted")
			}
			if err := database.GetDB().Model(inbound).Update("enable", false).Error; err != nil {
				t.Fatal(err)
			}
			if err := RemoveProtocolComponent(context.Background(), tc.name); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("unused binary retained")
			}
		})
	}
	if err := RemoveProtocolComponent(context.Background(), "../hiddify-core"); err == nil {
		t.Fatal("accepted arbitrary path")
	}
}

func TestRemoveProtocolComponentStopsStandaloneAndAppliesNativeListener(t *testing.T) {
	if !protocolserver.Supported(model.NaiveProxy, runtime.GOOS, runtime.GOARCH) {
		t.Skip("unsupported platform")
	}
	svc, _, _ := stage5ProcessFixture(t)
	t.Cleanup(func() { _ = protocolserver.StopAll() })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	inbound := seedHiddifyConfigInbound(t, model.NaiveProxy, `{"tls":{"certificatePath":"/cert","keyPath":"/key"}}`, `{"network":"tcp","security":"none"}`)
	if err := database.GetDB().Model(inbound).Update("port", listener.Addr().(*net.TCPAddr).Port).Error; err != nil {
		t.Fatal(err)
	}
	// The listener supplies the readiness endpoint for the managed process fixture.
	if err := InstallProtocolServer(context.Background(), model.NaiveProxy, func(context.Context) error {
		return os.WriteFile(protocolserver.BinaryPath(model.NaiveProxy), []byte("#!/bin/sh\nif [ \"$1\" = validate ]; then exit 0; fi\nexec sleep 60\n"), 0o700)
	}); err != nil {
		t.Fatal(err)
	}
	if !protocolserver.Running(model.NaiveProxy) || !strings.Contains(string(svc.process().AppliedConfig()), `"type": "socks"`) {
		t.Fatal("standalone fixture not serving")
	}
	if err := RemoveProtocolComponent(context.Background(), "naiveproxy"); err != nil {
		t.Fatal(err)
	}
	if protocolserver.Running(model.NaiveProxy) || !svc.IsRunning() || !strings.Contains(string(svc.process().AppliedConfig()), `"type": "naive"`) {
		t.Fatal("removal did not stop standalone and apply native listener")
	}
}
