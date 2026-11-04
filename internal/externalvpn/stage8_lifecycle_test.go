package externalvpn

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func stage8OpenFlux(t *testing.T) (*Manager, Instance) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	// A real child exits on the rejected setting, otherwise remains available.
	script := "#!/bin/sh\nif grep -q 'SessionContext = broken' \"$2\"; then exit 3; fi\nexec sleep 60\n"
	if err := os.WriteFile(filepath.Join(dir, "openflux"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	m := &Manager{procs: map[int]*running{}, lastErrors: map[int]string{}}
	t.Cleanup(m.StopAll)
	inst := Instance{ID: 81, Tag: "external", Protocol: model.OpenFlux, Listen: "127.0.0.1", Port: 34567, Settings: Settings{Context: "stable", Transports: []OpenFluxTransport{{Type: "direct", Priority: 100}}, Clients: []Client{{Email: "alice", Password: strings.Repeat("a", 64), Enable: true}}}}
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	return m, inst
}

func TestStage8ReconcileRejectsInvalidBeforeStopping(t *testing.T) {
	m, inst := stage8OpenFlux(t)
	old := m.procs[inst.ID]
	inst.Settings.Context = "bad\nContext=other"
	m.Reconcile([]Instance{inst})
	if m.procs[inst.ID] != old {
		t.Fatal("invalid reconcile replaced working child")
	}
	select {
	case <-old.done:
		t.Fatal("invalid reconcile stopped child")
	default:
	}
}

func TestStage8InboundStartupFailureRestoresPrevious(t *testing.T) {
	m, inst := stage8OpenFlux(t)
	previous := fingerprint(inst)
	inst.Settings.Context = "broken"
	if err := m.Ensure(inst); err == nil {
		t.Fatal("early exit accepted")
	}
	restored := m.procs[inst.ID]
	if restored == nil || restored.fingerprint != previous {
		t.Fatal("previous configuration not restored")
	}
	select {
	case <-restored.done:
		t.Fatal("restored process stopped")
	default:
	}
	data, err := os.ReadFile(filepath.Join(directory(), "81", "openflux.conf"))
	if err != nil || !strings.Contains(string(data), "SessionContext = stable") {
		t.Fatal("rollback files", err)
	}
}

func TestStage8InvalidOutboundSetHasNoSideEffects(t *testing.T) {
	m, _ := stage8OpenFlux(t)
	_ = m
	previousPlans, previousProcesses := outboundPlans, outboundProcesses
	outboundPlans = map[string]*managedOutbound{}
	outboundProcesses = map[string]*managedOutbound{}
	t.Cleanup(func() { KeepOutbounds(nil); outboundPlans = previousPlans; outboundProcesses = previousProcesses })
	raw := openFluxPlan("audit", "a")
	if _, err := EnsureOutbound(raw); err != nil {
		t.Fatal(err)
	}
	original := outboundProcesses["audit"]
	next := openFluxPlan("audit", "b")
	invalid := openFluxPlan("bad", "c")
	invalid["settings"].(map[string]any)["secret"] = "short"
	if _, err := ApplyOutbounds([]map[string]any{next, invalid}); err == nil {
		t.Fatal("invalid set accepted")
	}
	if outboundProcesses["audit"] != original {
		t.Fatal("rejected set restarted live sidecar")
	}
}

func TestStage8CancelKillsDescendant(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Unix child process")
	}
	path := filepath.Join(t.TempDir(), "child")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", `sleep 60 & echo $! > "$1"; wait`, "audit", path)
	prepareVetroffCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var pid string
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); {
		data, _ := os.ReadFile(path)
		pid = strings.TrimSpace(string(data))
		if pid != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == "" {
		t.Fatal("child did not start")
	}
	t.Cleanup(func() { _ = exec.Command("kill", "-9", pid).Run() })
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("parent did not stop")
	}
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); {
		data, err := os.ReadFile("/proc/" + pid + "/stat")
		if os.IsNotExist(err) || strings.Contains(string(data), ") Z ") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("descendant survived cancel")
}

func TestStage8ProbeFailureCleansPrivateFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	previousPlans, previousProcesses := outboundPlans, outboundProcesses
	outboundPlans = map[string]*managedOutbound{}
	outboundProcesses = map[string]*managedOutbound{}
	t.Cleanup(func() { KeepOutbounds(nil); outboundPlans = previousPlans; outboundProcesses = previousProcesses })
	if _, cleanup, err := EnsureProbeOutbound(openFluxPlan("probe", "a")); err == nil {
		cleanup()
		t.Fatal("missing binary accepted")
	}
	files, err := filepath.Glob(filepath.Join(directory(), "outbound-*"))
	if err != nil || len(files) != 0 {
		t.Fatal("failed probe retained private files", files, err)
	}
}
