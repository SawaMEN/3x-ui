//go:build !windows

package web

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/sudoku"
	"github.com/SawaMEN/3x-ui/v3/internal/web/job"
)

func TestSudokuSurvivesPanelReloadAndStopsOnShutdown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	if err := os.WriteFile(sudoku.GetBinaryPath(dir), []byte("#!/bin/sh\nif [ \"$1\" = '-test' ]; then exit 0; fi\nexec sleep 60\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sudoku.version"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr := sudoku.GetManager()
	mgr.StopAll()
	defer mgr.StopAll()
	desired := []sudoku.Instance{{ID: 981, Tag: "audit-sudoku", Config: sudoku.Config{Mode: "server", LocalPort: 2443, AEAD: "none"}}}
	mgr.Reconcile(context.Background(), desired)
	if err := mgr.Check(desired); err != nil {
		t.Fatal(err)
	}
	s := NewServer(nil)
	_ = job.NewSudokuJob(s.ctx)
	if err := s.StopPanelOnly(); err != nil {
		t.Fatal(err)
	}
	// Allow the old constructor's detached cancellation watcher to run.
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		if err := mgr.Check(desired); err != nil {
			t.Fatalf("panel-only reload stopped live Sudoku: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	full := NewServer(nil)
	_ = job.NewSudokuJob(full.ctx)
	if err := full.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Check(desired); err == nil {
		t.Fatal("full shutdown returned while Sudoku was still running")
	}
}

func TestShutdownStopsSidecarReconciledByLastCronTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	pidFile := filepath.Join(dir, "audit-child.pid")
	t.Setenv("XUI_AUDIT_CHILD_PID", pidFile)
	script := "#!/bin/sh\necho $$ > \"$XUI_AUDIT_CHILD_PID\"\nexec sleep 60\n"
	if err := os.WriteFile(filepath.Join(dir, "openflux"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	mgr := externalvpn.GetManager()
	mgr.StopAll()
	defer mgr.StopAll()
	inst := externalvpn.Instance{ID: 982, Tag: "audit-external", Protocol: model.OpenFlux, Listen: "127.0.0.1", Port: 34567, Settings: externalvpn.Settings{Context: "stable", Transports: []externalvpn.OpenFluxTransport{{Type: "direct", Priority: 100}}, Clients: []externalvpn.Client{{Email: "synthetic", Password: strings.Repeat("a", 64), Enable: true}}}}
	if err := mgr.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	readPID := func() int {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			t.Fatal(err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			t.Fatal(err)
		}
		return pid
	}
	initialPID := readPID()
	s := NewServer(nil)
	s.cron = cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger)))
	entered := make(chan struct{})
	tickErr := make(chan error, 1)
	_, err := s.cron.AddFunc("@every 1s", func() {
		close(entered)
		<-s.ctx.Done()
		deadline := time.Now().Add(200 * time.Millisecond)
		for time.Now().Before(deadline) && syscall.Kill(initialPID, 0) == nil {
			time.Sleep(time.Millisecond)
		}
		tickErr <- mgr.Ensure(inst)
	})
	if err != nil {
		t.Fatal(err)
	}
	s.cron.Start()
	<-entered
	if err := s.Stop(); err == nil || !strings.Contains(err.Error(), "OpenFlux metrics unavailable") {
		t.Fatalf("expected missing metrics diagnostic for dummy child, got: %v", err)
	}
	if err := <-tickErr; err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(readPID(), 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("last cron tick left sidecar alive after full stop: %v", err)
	}
}
