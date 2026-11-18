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
