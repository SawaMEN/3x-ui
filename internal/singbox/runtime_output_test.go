package singbox

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeConsoleDrainsWhenStorageFails(t *testing.T) {
	w := newRuntimeOutput(t.TempDir())
	defer w.Close()
	const report = "panic: storage unavailable"
	if n, err := w.Write([]byte(report)); n != len(report) || err != nil {
		t.Fatalf("pipe blocked: %d %v", n, err)
	}
	if w.String() != report {
		t.Fatal("memory diagnostic lost")
	}
}

func TestProcessCrashReportSurvivesProcessRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	binary, cfg := filepath.Join(dir, "core"), filepath.Join(dir, "core.json")
	script := `#!/bin/sh
case "$1" in
version) echo 'test-core 1';;
check) exit 0;;
run) printf 'panic: protocol crash\ngoroutine 7 [running]:\nmain.worker()\n\tcore.go:42\n' >&2; exit 9;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := NewProcessWithOptions(cfg, ProcessOptions{Binary: binary, RunCommand: "run", VersionPrefix: "test-core "}, true)
	var previous *runtimeOutput
	for range 2 {
		if err := p.Start(t.Context()); err == nil {
			t.Fatal("crash hidden")
		}
		if previous != nil && previous != p.consoleOutput {
			t.Fatal("restart created another rotation worker")
		}
		previous = p.consoleOutput
	}
	data, err := os.ReadFile(cfg + ".console.log")
	if err != nil || strings.Count(string(data), "panic: protocol crash") != 2 || !strings.Contains(string(data), "\tcore.go:42") {
		t.Fatalf("lost crash: %v %s", err, data)
	}
}

func TestNaiveDualTransportPreservesQUICCongestionControl(t *testing.T) {
	for _, network := range []string{"", "udp", "tcp"} {
		raw := map[string]any{"protocol": "naive", "tag": "naive", "settings": map[string]any{"network": network, "quicCongestionControl": "cubic", "tls": map[string]any{"certificatePath": "cert.pem", "keyPath": "key.pem"}, "clients": []any{map[string]any{"email": "alice", "password": "secret"}}}}
		out, err := TranslateXrayInbound(raw)
		if err != nil {
			t.Fatal(err)
		}
		if (out["quic_congestion_control"] == "cubic") != (network != "tcp") {
			t.Fatalf("network %q lost settings: %#v", network, out)
		}
	}
}
