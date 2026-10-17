package oxidns

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercises the same official GitHub API -> release -> SHA256 -> archive
// pipeline used by the Install button, but only writes into a temp folder.
func TestVerifiedReleaseDownload(t *testing.T) {
	if os.Getenv("OXIDNS_REAL_DOWNLOAD_TEST") != "1" {
		t.Skip("requires internet access to official GitHub releases")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	version, asset, err := Latest(ctx)
	if err != nil {
		t.Fatalf("cannot select latest official release: %v", err)
	}
	executable, err := installRelease(ctx, *asset)
	if err != nil {
		t.Fatalf("verified official download failed: %v", err)
	}
	path := filepath.Join(t.TempDir(), "oxidns")
	if err := os.WriteFile(path, executable, 0755); err != nil {
		t.Fatal(err)
	}
	output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("downloaded binary cannot run: %v (%s)", err, output)
	}
	if !strings.Contains(string(output), strings.TrimPrefix(version, "v")) {
		t.Fatalf("binary version %s does not match release %s", output, version)
	}
}
