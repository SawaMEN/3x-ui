package singbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProcessProfileUsesRawCommandAndPinnedCapabilities(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "core")
	cfg := filepath.Join(dir, "candidate.json")
	script := `#!/bin/sh
case "$1" in
 version) printf 'hiddify-core version v5.0.0 hiddify-sing-box version 1.13.0-rc.2\nTags: with_xui_panel,with_v2ray_api\n' ;;
 check) exit 0 ;;
 srun) printf 'raw command reached\n' >&2; exit 9 ;;
 *) printf 'unexpected command: %s\n' "$1" >&2; exit 10 ;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(`{"inbounds":[{"type":"snell"}],"endpoints":[{"type":"masque-server"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	profile := ProcessOptions{Binary: binary, RunCommand: "srun", VersionPrefix: "hiddify-core version ", RequiredVersion: "v5.0.0", RequiredTags: []string{"with_xui_panel", "with_v2ray_api"}, MASQUE: true}
	p := NewProcessWithOptions(cfg, profile, true)
	if err := p.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.GetVersion() != "v5.0.0" || !p.SupportsMASQUE() || p.SupportsNativeAPI() {
		t.Fatal("profile capabilities use stock version gates")
	}
	if err := p.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "raw command reached") {
		t.Fatalf("wrong command or startup error hidden: %v", err)
	}
	if p.IsRunning() {
		t.Fatal("failed child reported running")
	}
	profile.RequiredTags = append(profile.RequiredTags, "missing_capability")
	if err := NewProcessWithOptions(cfg, profile, true).Validate(context.Background()); err == nil {
		t.Fatal("incomplete build accepted")
	}
	profile.RequiredTags = nil
	profile.RequiredVersion = "v4.1.0"
	if _, err := NewProcessWithOptions(cfg, profile, true).Version(context.Background()); err == nil {
		t.Fatal("unpinned version accepted")
	}
}
