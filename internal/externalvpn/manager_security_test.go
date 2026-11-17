package externalvpn

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestBinaryRejectsProtocolPathTraversal(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "injected"), []byte("not a managed binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XUI_BIN_FOLDER", bin)
	for _, protocol := range []string{"../injected", "unknown", filepath.Join(root, "injected")} {
		if got := binary(model.Protocol(protocol)); got != "" {
			t.Fatalf("unmanaged protocol %q resolved to %q", protocol, got)
		}
	}
}
