package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandaloneRemovalCleansOwnedPayloadsAndPreservesOtherCore(t *testing.T) {
	for _, engine := range []string{CoreTypeXray, CoreTypeSingBoxUpstream} {
		t.Run(engine, func(t *testing.T) {
			setupBulkDB(t)
			dir := t.TempDir()
			t.Setenv("XUI_BIN_FOLDER", dir)
			svc := NativeCore(engine)
			other := CoreTypeXray
			if engine == other {
				other = CoreTypeSingBoxUpstream
			}
			keep := []string{filepath.Base(NativeCore(other).BinaryPath()), filepath.Base(NativeCore(CoreTypeSingBox).BinaryPath()), "geoip.dat", "geosite.dat", filepath.Base(svc.ProcessConfigPath()), ".core-" + other + "-backup-recovery", ".core-backup-legacy"}
			remove := []string{filepath.Base(svc.BinaryPath()), ".core-" + engine + "-install-partial", ".core-" + engine + "-update-partial", ".core-" + engine + "-backup-old", ".core-install-legacy", ".core-update-legacy", ".core-geodata-legacy"}
			for _, name := range append(append([]string{}, keep...), remove...) {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("retained payload"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			settings := &SettingService{}
			if err := settings.SetCoreType(engine); err != nil {
				t.Fatal(err)
			}
			if err := UninstallStandaloneCore(context.Background(), engine); err == nil || !strings.Contains(err.Error(), "switch") {
				t.Fatalf("selected core removed: %v", err)
			}
			if err := settings.SetCoreType(CoreTypeSingBox); err != nil {
				t.Fatal(err)
			}
			if err := UninstallStandaloneCore(context.Background(), engine); err != nil {
				t.Fatal(err)
			}
			for _, name := range remove {
				if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
					t.Errorf("removed core payload remains: %s (%v)", name, err)
				}
			}
			for _, name := range keep {
				if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != "retained payload" {
					t.Errorf("unrelated data changed: %s (%q %v)", name, data, err)
				}
			}
		})
	}
}
