//go:build linux

package naiveproxy

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestExtractOfficialArchiveKeepsLocalOwnership(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root archive ownership regression")
	}
	for _, command := range []string{"tar", "xz"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skip(command + " not installed")
		}
	}
	var raw bytes.Buffer
	writer := tar.NewWriter(&raw)
	contents := []byte("official binary fixture")
	if err := writer.WriteHeader(&tar.Header{Name: "caddy-forwardproxy-naive/caddy", Mode: 0o755, Size: int64(len(contents)), Uid: 1001, Gid: 1001}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	compressor := exec.Command("xz", "-c")
	compressor.Stdin = &raw
	compressed, err := compressor.Output()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	archive := filepath.Join(dir, "release.tar.xz")
	if err := os.WriteFile(archive, compressed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractTarXZ(context.Background(), archive, dir); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "caddy-forwardproxy-naive", "caddy")
	data, err := os.ReadFile(binary)
	if err != nil || !bytes.Equal(data, contents) {
		t.Fatalf("extracted binary: %q, %v", data, err)
	}
	info, err := os.Stat(binary)
	if err != nil {
		t.Fatal(err)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != uint32(os.Geteuid()) {
		t.Fatalf("archive ownership was restored: uid %d", stat.Uid)
	}
}
