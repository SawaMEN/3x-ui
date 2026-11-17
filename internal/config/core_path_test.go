package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOwnerCorePathResolvesRelativeFolderBesidePanel(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", "relative-bin")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := OwnerCoreBinaryPath()
	if !filepath.IsAbs(path) || filepath.Dir(path) != filepath.Join(filepath.Dir(exe), "relative-bin") {
		t.Fatalf("path=%q", path)
	}
}
