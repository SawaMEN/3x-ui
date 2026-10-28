package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStandaloneCoreAdapters(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	setupBulkDB(t)
	oldRuntime, oldOwner, oldUpstream, oldXray := hiddifyRuntime, hiddifyProcess, upstreamProcess, standaloneXrayProcess
	t.Cleanup(func() {
		hiddifyRuntime, hiddifyProcess, upstreamProcess, standaloneXrayProcess = oldRuntime, oldOwner, oldUpstream, oldXray
	})
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	InitializeNativeCoreRuntime()
	for _, engine := range []string{CoreTypeSingBoxUpstream, CoreTypeXray} {
		t.Run(engine, func(t *testing.T) {
			svc := NativeCore(engine)
			if svc.BinaryPath() == NativeCore(CoreTypeSingBox).BinaryPath() {
				t.Fatal("standalone binary aliases bundled binary")
			}
			if svc.process().ConfigPath() != svc.ProcessConfigPath() {
				t.Fatal("config paths diverge")
			}
			prefix := "sing-box version 1.14.4"
			if engine == CoreTypeXray {
				prefix = "Xray 26.10.21"
			}
			script := "#!/bin/sh\ncase \"$1\" in\nversion) echo '" + prefix + "';;\ncheck) exit 0;;\nrun) if [ \"$2\" = '-test' ]; then exit 0; fi; exec sleep 60;;\n*) exit 13;;\nesac\n"
			if err := os.WriteFile(svc.BinaryPath(), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			if err := (&SettingService{}).SetCoreType(engine); err != nil {
				t.Fatal(err)
			}
			if err := svc.WriteConfig(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(svc.ProcessConfigPath())
			if err != nil {
				t.Fatal(err)
			}
			if engine == CoreTypeXray && !strings.Contains(string(data), `"protocol"`) {
				t.Fatal("Xray received a sing-box config")
			}
			if engine == CoreTypeSingBoxUpstream && strings.Contains(string(data), `"v2ray_api"`) {
				t.Fatal("unsupported upstream V2Ray API generated")
			}
			if err := svc.process().Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.process().Stop() })
			if !svc.IsRunning() {
				t.Fatal("standalone runtime did not start")
			}
			if NativeCore(CoreTypeSingBox).IsRunning() {
				t.Fatal("standalone start launched bundled core")
			}
			if err := svc.process().Stop(); err != nil {
				t.Fatal(err)
			}
			if svc.IsRunning() {
				t.Fatal("standalone runtime did not stop")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(svc.BinaryPath()), "hiddify-core.json")); !os.IsNotExist(err) {
				t.Fatal("standalone operation wrote bundled config")
			}
		})
	}
}

// Opt-in checks use downloaded official release binaries, never public-IP lookups.
func TestStandaloneReleaseConfig(t *testing.T) {
	for _, tc := range []struct{ engine, env string }{{CoreTypeXray, "XUI_TEST_XRAY_BINARY"}, {CoreTypeSingBoxUpstream, "XUI_TEST_SINGBOX_BINARY"}} {
		t.Run(tc.engine, func(t *testing.T) {
			source := os.Getenv(tc.env)
			if source == "" {
				t.Skip("release binary not supplied")
			}
			setupBulkDB(t)
			t.Setenv("XUI_BIN_FOLDER", t.TempDir())
			svc := NativeCore(tc.engine)
			data, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(svc.BinaryPath(), data, 0755); err != nil {
				t.Fatal(err)
			}
			if tc.engine == CoreTypeXray {
				for _, name := range []string{"geoip.dat", "geosite.dat"} {
					b, err := os.ReadFile(filepath.Join(filepath.Dir(source), name))
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(filepath.Dir(svc.BinaryPath()), name), b, 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := (&SettingService{}).SetCoreType(tc.engine); err != nil {
				t.Fatal(err)
			}
			if err := svc.WriteConfig(); err != nil {
				t.Fatal(err)
			}
			process := standaloneProcess(tc.engine, svc.ProcessConfigPath(), true)
			if err := process.Start(context.Background()); err != nil {
				if strings.Contains(err.Error(), "create netlink socket: operation not permitted") {
					t.Skip("generated config passed; this sandbox prohibits sing-box network initialization")
				}
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = process.Stop() })
			if !process.IsRunning() {
				t.Fatal("official release did not stay running")
			}
			if err := process.Stop(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
