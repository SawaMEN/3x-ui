package service

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestStage4GatewayPauseFailureRestoresPublishedConfig(t *testing.T) {
	svc, starts := stage8GatewayFixture(t)
	network, host, cfg := newGatewayTestHost(t)
	svc.gateway = network
	if err := network.Enable(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(svc.ProcessConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	before := stage5Starts(t, starts)
	if err := database.GetDB().Create(&model.Inbound{Enable: true, Protocol: model.HTTP, Tag: "stage4-candidate", Port: 25198, Settings: `{"clients":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	host.failures["nft apply"] = 1
	if err := svc.Restart(context.Background()); err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("pause failure: %v", err)
	}
	data, err := os.ReadFile(svc.ProcessConfigPath())
	if err != nil || !bytes.Equal(data, old) {
		t.Fatal("failed apply left candidate on disk")
	}
	if !svc.IsRunning() || stage5Starts(t, starts) != before || !bytes.Equal(svc.process().AppliedConfig(), old) {
		t.Fatal("failed pause changed running core")
	}
}

func TestStage4CandidatePublicationRollback(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "first-start", true: "existing"}[exists], func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "core.json")
			candidate := filepath.Join(dir, "candidate.json")
			if exists {
				if err := os.WriteFile(target, []byte("previous"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(candidate, []byte("candidate"), 0600); err != nil {
				t.Fatal(err)
			}
			c := &nativeConfigCandidate{path: candidate, target: target}
			defer c.close()
			finish, err := c.stage(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := finish(false); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(target)
			if exists {
				if err != nil || string(data) != "previous" {
					t.Fatalf("prior file lost: %q %v", data, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("first-start candidate survived rollback: %v", err)
			}
		})
	}
}

func stage4XrayFixture(t *testing.T) *SingBoxService {
	t.Helper()
	setupBulkDB(t)
	oldRuntime, oldOwner, oldUpstream, oldXray := hiddifyRuntime, hiddifyProcess, upstreamProcess, standaloneXrayProcess
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	InitializeNativeCoreRuntime()
	svc := NativeCore(CoreTypeXray)
	t.Cleanup(func() {
		_ = svc.process().Stop()
		hiddifyRuntime, hiddifyProcess, upstreamProcess, standaloneXrayProcess = oldRuntime, oldOwner, oldUpstream, oldXray
	})
	script := `#!/bin/sh
case "$1" in
 version) echo 'Xray 26.10.21';;
 run) if [ "$2" = '-test' ]; then grep -q '"protocol"' "$4"; exit $?; fi
 if grep -q stage4-fail "$3"; then exit 7; fi
 exec sleep 60;;
 *) exit 19;;
esac
`
	if err := os.WriteFile(svc.BinaryPath(), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := (&SettingService{}).SetCoreType(CoreTypeXray); err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestStage4XrayStartUsesNativeXrayConfiguration(t *testing.T) {
	svc := stage4XrayFixture(t)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !svc.IsRunning() {
		t.Fatal("Xray Start did not launch runtime")
	}
	data, err := os.ReadFile(svc.ProcessConfigPath())
	var config struct {
		API       map[string]any   `json:"api"`
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err != nil || json.Unmarshal(data, &config) != nil || config.API == nil || len(config.Outbounds) == 0 {
		t.Fatal("Xray received sing-box configuration")
	}
	for _, outbound := range config.Outbounds {
		if outbound["protocol"] == nil || outbound["type"] != nil {
			t.Fatal("Xray received sing-box outbound schema")
		}
	}

}

func TestStage4StoppedXrayFailedRestartDoesNotLaunchOldCore(t *testing.T) {
	svc := stage4XrayFixture(t)
	if err := svc.WriteConfig(); err != nil {
		t.Fatal(err)
	}
	if err := svc.process().Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.process().Stop(); err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(svc.ProcessConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.Inbound{Enable: true, Protocol: model.HTTP, Tag: "stage4-fail", Port: 25197, Settings: `{"clients":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(context.Background()); err == nil {
		t.Fatal("failing Xray reported success")
	}
	if svc.IsRunning() {
		t.Fatal("failed restart launched old manually stopped Xray")
	}
	data, err := os.ReadFile(svc.ProcessConfigPath())
	if err != nil || !bytes.Equal(data, old) {
		t.Fatal("failed restart lost previous disk config")
	}
}
