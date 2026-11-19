//go:build linux

package systemupdate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanupCommands(t *testing.T) {
	for _, tc := range []struct {
		manager string
		command string
	}{
		{"apt-get", "apt-get clean"},
		{"pacman", "pacman -Scc --noconfirm"},
		{"dnf", "dnf clean all"},
		{"yum", "yum clean all"},
		{"zypper", "zypper clean --all"},
		{"apk", "apk cache clean"},
	} {
		cmds := cleanupCommands(tc.manager)
		if len(cmds) != 1 || strings.Join(cmds[0], " ") != tc.command {
			t.Fatalf("%s: unexpected cleanup commands: %v", tc.manager, cmds)
		}
	}
	if len(cleanupCommands("unknown")) != 0 {
		t.Fatal("unexpected commands for unknown distribution")
	}
}

func TestCleanupOnlyRemovesRecognizedStaleFiles(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-2 * time.Hour)
	for _, file := range []string{
		".core-xray-install-12345",
		".core-sing-box-upstream-install-abc",
		".core-geodata-xyz",
		"xray",
		"sing-box-upstream",
		".core-backup-123",
		".core-xray-install-recent",
	} {
		path := filepath.Join(dir, file)
		if err := os.WriteFile(path, []byte("binary"), 0o600); err != nil {
			t.Fatal(err)
		}
		if file != ".core-xray-install-recent" {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	result := &CleanupResult{}
	if err := removeStaleFiles(context.Background(), dir, time.Hour, isStaleCoreArtifact, result); err != nil {
		t.Fatal(err)
	}
	if result.RemovedFiles != 3 {
		t.Fatalf("removed %d files, expected 3", result.RemovedFiles)
	}
	for _, file := range []string{"xray", "sing-box-upstream", ".core-backup-123", ".core-xray-install-recent"} {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Fatalf("must preserve %q: %v", file, err)
		}
	}
}

func TestCleanupDoesNotFollowSymlinksOrDeleteDirectories(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "keep")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ".core-xray-install-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".core-xray-install-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatal(err)
	}
	result := &CleanupResult{}
	if err := removeStaleFiles(context.Background(), dir, 0, isStaleCoreArtifact, result); err != nil {
		t.Fatal(err)
	}
	if result.RemovedFiles != 0 {
		t.Fatalf("removed unexpected entries: %d", result.RemovedFiles)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("removed symlink target: ", err)
	}
	if err := removeStaleFiles(context.Background(), filepath.Join(dir, ".core-xray-install-link"), 0, isStaleCoreArtifact, result); err == nil {
		t.Fatal("cleanup accepted a symlinked directory")
	}
}

func TestPartialDownloadMatcher(t *testing.T) {
	for _, file := range []string{"foo.deb.part", "foo.pkg.tar.zst.part", "foo.partial"} {
		if !isPartialDownload(file) {
			t.Fatalf("did not recognize %q", file)
		}
	}
	for _, file := range []string{"linux.pkg.tar.zst", "linux-image.deb", "xray", "pacman.lck"} {
		if isPartialDownload(file) {
			t.Fatalf("would remove complete file %q", file)
		}
	}
}

func TestDetectRunningPackageManager(t *testing.T) {
	proc := t.TempDir()
	pid := filepath.Join(proc, "12345678")
	if err := os.Mkdir(pid, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pid, "comm"), []byte("pacman\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := activePackageManager(proc)
	if err != nil || got != "pacman" {
		t.Fatalf("got %q, %v, want pacman", got, err)
	}
	if err := os.WriteFile(filepath.Join(pid, "comm"), []byte("x-ui\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = activePackageManager(proc)
	if err != nil || got != "" {
		t.Fatalf("expected no package manager, got %q, %v", got, err)
	}
}

func TestActivePackageManagerPrefersRealManagerOverPackageKit(t *testing.T) {
	proc := t.TempDir()
	for pid, name := range map[string]string{
		"12345678": "packagekitd",
		"12345679": "pacman",
	} {
		path := filepath.Join(proc, pid)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "comm"), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	active, err := activePackageManager(proc)
	if err != nil || active != "pacman" {
		t.Fatalf("active=%q, err=%v; must not ignore real pacman updates", active, err)
	}
}

func TestCleanupBlockingPackageManagerPackageKit(t *testing.T) {
	proc := t.TempDir()
	path := filepath.Join(proc, "12345678")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "comm"), []byte("packagekitd\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name       string
		busy       bool
		checkError error
		want       string
		wantError  bool
	}{
		{name: "idle daemon does not block cleanup"},
		{name: "active transaction blocks cleanup", busy: true, want: "packagekitd"},
		{name: "D-Bus error fails closed", checkError: os.ErrPermission, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checked := false
			active, err := cleanupBlockingPackageManager(context.Background(), proc, func(context.Context) (bool, error) {
				checked = true
				return tc.busy, tc.checkError
			})
			if !checked {
				t.Fatal("PackageKit transaction list was not queried")
			}
			if (err != nil) != tc.wantError || active != tc.want {
				t.Fatalf("active=%q, err=%v; want active=%q, error=%v", active, err, tc.want, tc.wantError)
			}
		})
	}
}

func TestCleanupBlockingPackageManagerNeverChecksPackageKitWhenPacmanRuns(t *testing.T) {
	proc := t.TempDir()
	path := filepath.Join(proc, "12345679")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "comm"), []byte("pacman\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	active, err := cleanupBlockingPackageManager(context.Background(), proc, func(context.Context) (bool, error) {
		t.Fatal("should not query PackageKit for a running pacman transaction")
		return false, nil
	})
	if err != nil || active != "pacman" {
		t.Fatalf("active=%q, err=%v", active, err)
	}
}

func TestParsePackageKitTransactionList(t *testing.T) {
	for _, tc := range []struct {
		name    string
		output  string
		busy    bool
		wantErr bool
	}{
		{name: "idle", output: "ao 0\n"},
		{name: "one transaction", output: "ao 1 /org/freedesktop/PackageKit/transactions/1\n", busy: true},
		{name: "queued transactions", output: "ao 2 /org/freedesktop/PackageKit/transactions/1 /org/freedesktop/PackageKit/transactions/2\n", busy: true},
		{name: "empty response", output: "", wantErr: true},
		{name: "malformed count", output: "ao invalid", wantErr: true},
		{name: "count mismatch", output: "ao 1", wantErr: true},
		{name: "unexpected type", output: "as 0", wantErr: true},
		{name: "unexpected path", output: "ao 1 incorrect", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			busy, err := parsePackageKitTransactionList(tc.output)
			if busy != tc.busy || (err != nil) != tc.wantErr {
				t.Fatalf("output=%q: busy=%v, err=%v", tc.output, busy, err)
			}
		})
	}
}

func TestPackageCacheCleanupAllowed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		active  string
		err     error
		allowed bool
	}{
		{name: "idle", allowed: true},
		{name: "unattended upgrade active", active: "unattended-upgr"},
		{name: "other package manager active", active: "pacman"},
		{name: "PackageKit status unknown", err: os.ErrPermission},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &CleanupResult{}
			got := packageCacheCleanupAllowed(tc.active, tc.err, result)
			if got != tc.allowed {
				t.Fatalf("allowed=%v, want %v", got, tc.allowed)
			}
			if (len(result.Warnings) > 0) == tc.allowed {
				t.Fatalf("wrong warning state: %v", result.Warnings)
			}
			if tc.active != "" && !strings.Contains(result.Warnings[0], tc.active) {
				t.Fatalf("warning does not name the blocking process: %v", result.Warnings)
			}
		})
	}
}

func TestActivePackageManagerZombieIgnored(t *testing.T) {
	proc := t.TempDir()
	pid := filepath.Join(proc, "12345678")
	if err := os.Mkdir(pid, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pid, "comm"), []byte("unattended-upgr\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stat := filepath.Join(pid, "stat")
	if err := os.WriteFile(stat, []byte("12345678 (unattended-upgr) Z 1 1 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	active, err := activePackageManager(proc)
	if err != nil || active != "" {
		t.Fatalf("zombie package manager should not block cleanup: %q, %v", active, err)
	}
	if err := os.WriteFile(stat, []byte("12345678 (unattended-upgr) S 1 1 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	active, err = activePackageManager(proc)
	if err != nil || active != "unattended-upgr" {
		t.Fatalf("live package manager must be protected: %q, %v", active, err)
	}
}

func TestProcProcessState(t *testing.T) {
	for _, tc := range []struct {
		value   string
		state   byte
		wantErr bool
	}{
		{value: "42 (unattended-upgr) Z 1 2 3", state: 'Z'},
		{value: "42 (name with ) bracket) S 1 2 3", state: 'S'},
		{value: "malformed", wantErr: true},
		{value: "42 (abc)", wantErr: true},
	} {
		state, err := procProcessState(tc.value)
		if (err != nil) != tc.wantErr || state != tc.state {
			t.Fatalf("stat %q: state=%q err=%v", tc.value, state, err)
		}
	}
}
