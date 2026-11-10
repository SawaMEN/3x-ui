package logger

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/op/go-logging"
)

func TestErrorWriterPreservesStackLines(t *testing.T) {
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	InitLogger(logging.ERROR)
	t.Cleanup(CloseLogger)
	p := []byte("panic marker\nstack frame marker\n")
	if n, err := (ErrorWriter{}).Write(p); n != len(p) || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	lines := GetFileLogs(10, "error")
	if len(lines) != 2 || !strings.Contains(lines[0], "stack frame marker") || !strings.Contains(lines[1], "panic marker") {
		t.Fatalf("diagnostics lost: %v", lines)
	}
}

func TestRuntimeCrashSurvivesRestart(t *testing.T) {
	if os.Getenv("XUI_TEST_RUNTIME_CRASH") == "1" {
		InitLogger(logging.ERROR)
		if err := InitRuntimeDiagnostics(); err != nil {
			panic(err)
		}
		log.Print("standard logger marker")
		done := make(chan struct{})
		go func() { panic("background crash marker") }()
		<-done
		return
	}
	dir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", dir)
	cmd := exec.Command(os.Args[0], "-test.run=^TestRuntimeCrashSurvivesRestart$")
	cmd.Env = append(os.Environ(), "XUI_TEST_RUNTIME_CRASH=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("child should crash")
	}
	crash, err := os.ReadFile(filepath.Join(dir, "panic.log"))
	if err != nil || !strings.Contains(string(crash), "background crash marker") {
		t.Fatalf("missing crash report: %v %s", err, crash)
	}
	InitLogger(logging.ERROR)
	t.Cleanup(CloseLogger)
	// Replay in a second process: the test process must retain its own crash output.
	replay := exec.Command(os.Args[0], "-test.run=^TestRuntimeReplayHelper$")
	replay.Env = append(os.Environ(), "XUI_TEST_RUNTIME_REPLAY=1")
	if out, err := replay.CombinedOutput(); err != nil {
		t.Fatalf("replay: %v %s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, logFileName))
	if err != nil || !strings.Contains(string(data), "standard logger marker") || !strings.Contains(string(data), "runtime: panic: background crash marker") {
		t.Fatalf("report not replayed: %v %s", err, data)
	}
	crash, err = os.ReadFile(filepath.Join(dir, "panic.log"))
	if err != nil || len(crash) != 0 {
		t.Fatalf("old crash should be consumed: %v %s", err, crash)
	}
}

func TestRuntimeReplayHelper(t *testing.T) {
	if os.Getenv("XUI_TEST_RUNTIME_REPLAY") != "1" {
		return
	}
	InitLogger(logging.ERROR)
	defer CloseLogger()
	if err := InitRuntimeDiagnostics(); err != nil {
		t.Fatal(err)
	}
}
