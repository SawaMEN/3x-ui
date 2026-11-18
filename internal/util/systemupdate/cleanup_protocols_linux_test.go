//go:build linux

package systemupdate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProtocolCleanupRecognizesAllStandaloneStages(t *testing.T) {
	for _, name := range []string{
		".hysteria-1234567890",
		".pingtunnel-1234567890",
		".trusttunnel_endpoint-1234567890",
		".openflux-1234567890",
		".sudoku-tunnel-1234567890",
		".caddy-naive.new",
		"telemt.new",
	} {
		if !isProtocolStagingFile(name) {
			t.Errorf("did not recognize standalone staging file %q", name)
		}
	}
	for _, name := range []string{
		".openflux-install.json", ".openflux-previous-1234567890",
		".core-xray-backup-1234567890", ".sudoku-previous-1234567890",
		"openflux", "vetroff", "hysteria", "pingtunnel",
		"trusttunnel_endpoint", "caddy-naive", "sing-box",
		"sing-box-upstream", "xray", "telemt", "server.key",
	} {
		if isProtocolStagingFile(name) {
			t.Errorf("must preserve installed binary or metadata %q", name)
		}
	}
}

func TestProtocolCleanupTempDirectories(t *testing.T) {
	for _, name := range []string{
		"3x-ui-sudoku-1234567890", "3x-ui-externalvpn-1234567890",
		"3x-ui-fptn-client-1234567890", "3x-ui-caddy-naive-1234567890",
		"3x-ui-naive-check-1234567890", "olcrtc-patches-1234567890",
		"3x-ui-olcrtc.ABC123", "telemt-update.ABC123",
	} {
		if !isAbandonedInstallerDir(name) {
			t.Errorf("did not recognize installer scratch directory %q", name)
		}
	}
	for _, name := range []string{
		"3x-ui-olcrtc-backup", "olcrtc-manager",
		"telemt-servers", "x-ui", "server", ".core-backup.ABC12345",
	} {
		if isAbandonedInstallerDir(name) {
			t.Errorf("must not remove directory %q", name)
		}
	}
}

func TestProtocolCleanupConfigAndTempMatch(t *testing.T) {
	for _, name := range []string{".oxidns-1234567890", ".dnstt-1234567890", ".telemt-1234567890.toml", ".telemt-meko-1234567890.env"} {
		if !isProtocolConfigStagingFile(name) {
			t.Errorf("unrecognized config-stage %q", name)
		}
	}
	for _, name := range []string{"settings.json", "server.key", "telemt.toml", ".openflux-install.json", "telemt.version", ".oxidns-backup", ".dnstt-recovery", ".telemt-previous.toml"} {
		if isProtocolConfigStagingFile(name) {
			t.Errorf("must preserve configuration/metadata %q", name)
		}
	}
	for _, name := range []string{"olcrtc-installer-1234567890.sh", "olcrtc-built-revisions-1234567890", "olcrtc-install-log-1234567890.log"} {
		if !isProtocolTempFile(name) {
			t.Errorf("unrecognized OlcRTC scratch file %q", name)
		}
	}
}

func TestPanelUpdateArtifactMatchersPreserveBackups(t *testing.T) {
	for _, name := range []string{
		".x-ui-download.123456.tar.gz", ".x-ui-download.123456.tar.gz.part",
		".x-ui-download.123456.tar.gz.sha256", "x-ui-linux-amd64.tar.gz.part",
	} {
		if !isOldPanelDownload(name) {
			t.Errorf("missed panel download %q", name)
		}
	}
	for _, name := range []string{".x-ui-download.lock", ".x-ui-upgrade.lock", "x-ui-linux-amd64", ".core-backup.ABC12345"} {
		if isOldPanelDownload(name) || isAbandonedPanelStageDir(name) {
			t.Errorf("must preserve lock, installed binary or backup %q", name)
		}
	}
}

func TestKnownInstallerDirCleanupKeepsRecentBackupsAndSymlinks(t *testing.T) {
	root := t.TempDir()
	age := time.Now().Add(-72 * time.Hour)
	for _, name := range []string{
		"3x-ui-externalvpn-1234567890", "3x-ui-sudoku-1234567890",
		"3x-ui-caddy-naive-1234567890", "telemt-update.ABC123",
		".core-backup.ABC12345",
	} {
		p := filepath.Join(root, name)
		if err := os.Mkdir(p, 0o700); err != nil {
			t.Fatal(err)
		}
		if name == "telemt-update.ABC123" {
			if err := os.WriteFile(filepath.Join(p, "previous-telemt"), []byte("sole backup"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(p, age, age); err != nil {
			t.Fatal(err)
		}
	}
	recent := filepath.Join(root, "3x-ui-naive-check-1234567890")
	if err := os.Mkdir(recent, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "3x-ui-fptn-client-1234567890")); err != nil {
		t.Fatal(err)
	}
	result := &CleanupResult{}
	if err := removeOldKnownDirectories(context.Background(), root, 24*time.Hour, isAbandonedInstallerDir, result); err != nil {
		t.Fatal(err)
	}
	if result.RemovedFiles != 3 {
		t.Fatalf("removed %d expected 3 abandoned download folders", result.RemovedFiles)
	}
	for _, name := range []string{"telemt-update.ABC123", ".core-backup.ABC12345", "3x-ui-naive-check-1234567890", "3x-ui-fptn-client-1234567890"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
			t.Fatalf("removed protected path %q: %v", name, err)
		}
	}
	if err := removeOldKnownDirectories(context.Background(), filepath.Join(root, "3x-ui-fptn-client-1234567890"), 0, isAbandonedInstallerDir, result); err == nil {
		t.Fatal("accepted symlink root")
	}
}

func TestVetroffRecoveryDirectoryIsPreserved(t *testing.T) {
	root := t.TempDir()
	age := time.Now().Add(-72 * time.Hour)
	for _, name := range []string{".vetroff-stage-1234567890", ".vetroff-stage-9999999999"} {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if name == ".vetroff-stage-1234567890" {
			if err := os.Mkdir(filepath.Join(dir, "previous"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(dir, age, age); err != nil {
			t.Fatal(err)
		}
	}
	result := &CleanupResult{}
	if err := removeOldKnownDirectories(context.Background(), root, 24*time.Hour, func(name string) bool {
		return strings.HasPrefix(name, ".vetroff-stage-")
	}, result); err != nil {
		t.Fatal(err)
	}
	if result.RemovedFiles != 1 {
		t.Fatalf("removed %d expected 1 staged-only directory", result.RemovedFiles)
	}
	if _, err := os.Stat(filepath.Join(root, ".vetroff-stage-1234567890", "previous")); err != nil {
		t.Fatal("destroyed Vetroff rollback backup: ", err)
	}
}

func TestPanelUpdateLockPreventsDeletingIncompleteDownload(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "x-ui")
	bin := filepath.Join(install, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install, "x-ui"), []byte("panel"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, ".x-ui-download.123456.tar.gz.part")
	if err := os.WriteFile(file, []byte("incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	age := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(file, age, age); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(root, ".x-ui-download.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	result := &CleanupResult{}
	if err := cleanupPanelUpdateArtifacts(context.Background(), bin, result); err == nil {
		t.Fatal("cleanup ignored active update lock")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("removed active update download: ", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cleanupPanelUpdateArtifacts(context.Background(), bin, result); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("stale partial download still exists: %v", err)
	}
}

func TestActiveNamedProcessIgnoresNonPIDDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sys"), 0o700); err != nil {
		t.Fatal(err)
	}
	proc := filepath.Join(root, "12345678")
	if err := os.Mkdir(proc, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proc, "comm"), []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	active, err := activeNamedProcess(root, "go")
	if err != nil || !active {
		t.Fatalf("expected running go process: %v %v", active, err)
	}
	active, err = activeNamedProcess(root, "tar")
	if err != nil || active {
		t.Fatalf("false positive process: %v %v", active, err)
	}
}

func TestAbandonedInstallerWithFreshContentsIsPreserved(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "3x-ui-externalvpn-1234567890")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "release.tar.gz")
	if err := os.WriteFile(file, []byte("active download"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatal(err)
	}
	result := &CleanupResult{}
	if err := removeOldKnownDirectories(context.Background(), root, 24*time.Hour, isAbandonedInstallerDir, result); err != nil {
		t.Fatal(err)
	}
	if result.RemovedFiles != 0 {
		t.Fatalf("removed a recently written download: %d", result.RemovedFiles)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("download file should still exist: %v", err)
	}
}
