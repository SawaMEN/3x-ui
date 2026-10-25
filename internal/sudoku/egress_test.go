package sudoku

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

func TestCandidateAlwaysUsesDerivedCoreEgress(t *testing.T) {
	path, err := writeCandidateConfig(t.TempDir(), 8, Config{PanelEgress: "untrusted:123"})
	if err != nil {
		t.Fatal(err)
	}
	address, err := panelEgressAddress(path)
	if err != nil || address != sidecaregress.Address(8) {
		t.Fatalf("wrong payload route: %s %v", address, err)
	}
	path = filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := panelEgressAddress(path); err == nil {
		t.Fatal("legacy server config silently enabled direct egress")
	}
}
