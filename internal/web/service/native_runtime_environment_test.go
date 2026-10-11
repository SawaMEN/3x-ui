//go:build linux

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// main loads EnvironmentFile after Go package initialization. The supervised
// process must use the same final paths as config writers and offline migration.
func TestHiddifyRuntimeUsesEnvironmentLoadedAfterPackageInitialization(t *testing.T) {
	previousRuntime, previousOwner, previousStock := hiddifyRuntime, hiddifyProcess, singBoxProcess
	t.Cleanup(func() { hiddifyRuntime, hiddifyProcess, singBoxProcess = previousRuntime, previousOwner, previousStock })
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	binary := hiddify.GetBinaryPath()
	script := fmt.Sprintf("#!/bin/sh\nprintf 'hiddify-core version %s\\nTags: with_xui_panel,with_v2ray_api,with_awg,with_quic,with_wireguard,with_clash_api,with_utls,with_masque,with_gvisor,with_naive_outbound,with_purego,tfogo_checklinkname0\\nRevision: %s\\n'\n", hiddify.TargetVersion, hiddify.TargetRevision)
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	InitializeNativeCoreRuntime()
	svc := NativeCore(CoreTypeSingBox)
	if svc.process().ConfigPath() != svc.ProcessConfigPath() {
		t.Fatal("writer and runtime config paths diverge")
	}
	if hiddifyProcess.ConfigPath() != filepath.Join(filepath.Dir(binary), "hiddify-core.json") {
		t.Fatal("runtime ignored service environment")
	}
	if singBoxProcess.ConfigPath() != singbox.GetConfigPath() {
		t.Fatal("legacy runtime ignored service environment")
	}
	version, err := svc.process().Version(context.Background())
	if err != nil || version != hiddify.TargetVersion {
		t.Fatal("runtime executed the wrong owner binary", version, err)
	}
}
