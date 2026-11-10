package service

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOfficialStandaloneCoreInstallRemoveCycle(t *testing.T) {
	if os.Getenv("XUI_TEST_LIVE_CORE_INSTALL") != "1" {
		t.Skip("set XUI_TEST_LIVE_CORE_INSTALL=1 to download official release binaries")
	}
	setupBulkDB(t)
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	oldRuntime, oldOwner, oldUpstream, oldXray := hiddifyRuntime, hiddifyProcess, upstreamProcess, standaloneXrayProcess
	t.Cleanup(func() {
		hiddifyRuntime, hiddifyProcess, upstreamProcess, standaloneXrayProcess = oldRuntime, oldOwner, oldUpstream, oldXray
	})
	InitializeNativeCoreRuntime()
	if err := (&SettingService{}).SetCoreType(CoreTypeSingBox); err != nil {
		t.Fatal(err)
	}
	owner := NativeCore(CoreTypeSingBox).BinaryPath()
	if err := os.WriteFile(owner, []byte("bundled owner remains"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, engine := range []string{CoreTypeXray, CoreTypeSingBoxUpstream} {
		t.Run(engine, func(t *testing.T) {
			svc := NativeCore(engine)
			if svc.Installed() {
				t.Fatal("standalone core present before installation")
			}
			saved := []byte("preserved core settings")
			if err := os.WriteFile(svc.ProcessConfigPath(), saved, 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
			defer cancel()
			version, err := InstallStandaloneCore(ctx, engine, "latest")
			if err != nil {
				t.Fatal(err)
			}
			if !svc.Installed() {
				t.Fatal("official binary was not installed")
			}
			info, err := os.Stat(svc.BinaryPath())
			if err != nil || info.Size() == 0 {
				t.Fatalf("invalid binary: %v", err)
			}
			if _, err := UpdateStandaloneCore(ctx, engine, version); err != nil {
				t.Fatal(err)
			}
			legacy := filepath.Join(dir, ".core-backup-live-"+engine)
			binaryData, err := os.ReadFile(svc.BinaryPath())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(legacy, binaryData, 0o700); err != nil {
				t.Fatal(err)
			}
			installed := coreInstallBytes(t, dir)
			if err := UninstallStandaloneCore(ctx, engine); err != nil {
				t.Fatal(err)
			}
			removed := coreInstallBytes(t, dir)
			if svc.Installed() || installed-removed != 2*info.Size() {
				t.Fatalf("disk bytes retained: installed=%d removed=%d binary=%d", installed, removed, info.Size())
			}
			if raw, err := os.ReadFile(svc.ProcessConfigPath()); err != nil || string(raw) != string(saved) {
				t.Fatalf("settings changed: %q %v", raw, err)
			}
			if raw, err := os.ReadFile(owner); err != nil || string(raw) != "bundled owner remains" {
				t.Fatalf("owner removed: %q %v", raw, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".core-") {
					t.Fatalf("temporary payload retained: %s", entry.Name())
				}
			}
			if err := UninstallStandaloneCore(ctx, engine); err != nil {
				t.Fatal(err)
			}
			if _, err := InstallStandaloneCore(ctx, engine, version); err != nil {
				t.Fatal(err)
			}
			if err := UninstallStandaloneCore(ctx, engine); err != nil {
				t.Fatal(err)
			}
			t.Logf("official %s %s: binary %d bytes, freed %d bytes including verified legacy backup, retained shared files %d bytes; install/update/remove/reinstall passed", engine, version, info.Size(), installed-removed, removed)
		})
	}
}

func coreInstallBytes(t *testing.T, dir string) int64 {
	t.Helper()
	var size int64
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return size
}
