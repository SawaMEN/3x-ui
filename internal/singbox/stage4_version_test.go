package singbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStage4VersionProbeOncePerValidationAndStart(t *testing.T) {
	for _, masque := range []bool{false, true} {
		t.Run(fmt.Sprint(masque), func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "core")
			cfg := filepath.Join(dir, "config.json")
			counter := filepath.Join(dir, "versions")
			script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\nversion) echo v >> %q; echo 'sing-box version 1.14.4';;\ncheck) exit 0;;\nrun) exec sleep 60;;\nesac\n", counter)
			if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			data := `{}`
			if masque {
				data = `{"inbounds":[{"type":"masque-server"}]}`
			}
			if err := os.WriteFile(cfg, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			p := NewProcessWithOptions(cfg, ProcessOptions{Name: "stage4", Binary: binary, RunCommand: "run", VersionPrefix: "sing-box version ", MASQUE: true}, true)
			defer p.Stop()
			if err := p.Validate(context.Background()); err != nil {
				t.Fatal(err)
			}
			out, err := os.ReadFile(counter)
			if err != nil || strings.Count(string(out), "v\n") != 1 {
				t.Fatalf("Validate repeats version subprocess: %q %v", out, err)
			}
			if err := os.WriteFile(counter, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := p.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			out, err = os.ReadFile(counter)
			if err != nil || strings.Count(string(out), "v\n") != 1 {
				t.Fatalf("Start repeats version subprocess: %q %v", out, err)
			}
		})
	}
}
