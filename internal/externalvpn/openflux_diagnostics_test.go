package externalvpn

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestOpenFluxDiagnosticRefusesUnsupportedAndUnsaved(t *testing.T) {
	if OpenFluxDiagnosticTypeSupported("http://127.0.0.1") || OpenFluxDiagnosticTypeSupported("foo") {
		t.Fatal("diagnostic must accept only fixed transport types")
	}
	for _, typ := range []string{"direct", "yandex", "vyandex", "boards", "mailru", "cupsonline", "avito", "telemost", "oneme"} {
		if !OpenFluxDiagnosticTypeSupported(typ) {
			t.Fatalf("missing diagnostic for %s", typ)
		}
	}
	result := DiagnoseOpenFluxChannel(context.Background(), Instance{}, "yandex", "")
	if result.Passed || len(result.Steps) == 0 || result.Steps[0].Status != "failed" {
		t.Fatalf("unsaved tunnel was accepted: %+v", result)
	}
}

func TestOpenFluxDiagnosticRealDirectEncryptedRoundTrip(t *testing.T) {
	executable := os.Getenv("STAGE8_OPENFLUX_BINARY")
	if executable == "" {
		t.Skip("requires pinned official OpenFlux executable")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	if err := os.Symlink(executable, filepath.Join(dir, "openflux")); err != nil {
		t.Fatal(err)
	}
	inst := Instance{
		ID: 482, Tag: "openflux-diagnostic-direct", Protocol: model.OpenFlux,
		Listen: "127.0.0.1", Port: stage8Port(t),
		Settings: Settings{
			Context:    "diagnostic-contract",
			Clients:    []Client{{Email: "one", Password: strings.Repeat("h", 32), Enable: true}},
			Transports: []OpenFluxTransport{{Type: "direct", Priority: 100}},
		},
	}
	manager := &Manager{procs: map[int]*running{}, lastErrors: map[int]string{}}
	t.Cleanup(manager.StopAll)
	if err := manager.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	result := DiagnoseOpenFluxChannel(context.Background(), inst, "direct", "")
	if !result.Passed {
		t.Fatalf("real OpenFlux client could not pass encrypted echo: %+v", result)
	}
	if len(result.Steps) != 4 ||
		result.Steps[0].Status != "passed" ||
		result.Steps[1].Status != "skipped" ||
		result.Steps[2].Status != "passed" ||
		result.Steps[3].Status != "passed" {
		t.Fatalf("diagnostic stages misleading: %+v", result.Steps)
	}
}
