package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	golog "github.com/op/go-logging"
)

func TestGetLogsRetainsNewestRecordsAfterSeveralWraps(t *testing.T) {
	logBufferMu.Lock()
	logBuffer = nil
	logBufferMu.Unlock()
	const total = maxLogBufferSize*3 + 17
	for i := range total {
		level := "INFO"
		if i%3 == 0 {
			level = "ERROR"
		}
		addToBuffer(level, fmt.Sprintf("ring-record-%d", i))
	}
	for _, tc := range []struct {
		level string
		count int
	}{
		{"debug", 1},
		{"debug", 100},
		{"debug", maxLogBufferSize + 10},
		{"error", 1},
		{"error", 100},
		{"error", maxLogBufferSize + 10},
	} {
		got := GetLogs(tc.count, tc.level)
		want := []string{}
		for i := total - 1; i >= total-maxLogBufferSize && len(want) < tc.count; i-- {
			if tc.level != "error" || i%3 == 0 {
				want = append(want, fmt.Sprintf("ring-record-%d", i))
			}
		}
		if len(got) != len(want) {
			t.Fatalf("level=%s, count=%d: got %d records, want %d", tc.level, tc.count, len(got), len(want))
		}
		for i := range want {
			if !strings.HasSuffix(got[i], " - "+want[i]) {
				t.Fatalf("record %d = %q, want suffix %q", i, got[i], want[i])
			}
		}
	}
}

func TestGetLogsConcurrentWithBufferWrites(t *testing.T) {
	var workers sync.WaitGroup
	for writer := range 4 {
		workers.Go(func() {
			for i := range 1000 {
				addToBuffer("ERROR", fmt.Sprintf("concurrent-buffer-%d-%d", writer, i))
				if got := GetLogs(3, "debug"); len(got) > 3 {
					t.Errorf("snapshot exceeded limit: %d", len(got))
				}
			}
		})
	}
	workers.Wait()
}

func TestGetLogsEmptyAndNoMatchingRecords(t *testing.T) {
	logBufferMu.Lock()
	logBuffer = nil
	logBufferMu.Unlock()
	if got := GetLogs(10, "debug"); len(got) != 0 {
		t.Fatalf("empty buffer returned %d records", len(got))
	}
	for range maxLogBufferSize + 1 {
		addToBuffer("INFO", "filtered record")
	}
	if got := GetLogs(10, "error"); len(got) != 0 {
		t.Fatalf("error-only query returned %d INFO records", len(got))
	}
}

func BenchmarkLogBuffer(b *testing.B) {
	for _, mode := range []string{"write-full", "read-one", "read-100"} {
		b.Run(mode, func(b *testing.B) {
			logBufferMu.Lock()
			logBuffer = nil
			logBufferMu.Unlock()
			for range maxLogBufferSize {
				addToBuffer("INFO", "benchmark-log-record")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				switch mode {
				case "write-full":
					addToBuffer("INFO", "benchmark-log-record")
				case "read-one":
					GetLogs(1, "debug")
				default:
					GetLogs(100, "debug")
				}
			}
		})
	}
}

func TestFileLoggerRespectsConfiguredLevel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", dir)
	InitLogger(golog.WARNING)
	t.Cleanup(CloseLogger)
	Debug("filtered-debug-marker")
	Warning("retained-warning-marker")
	data, err := os.ReadFile(filepath.Join(dir, logFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "filtered-debug-marker") || !strings.Contains(string(data), "retained-warning-marker") {
		t.Fatalf("configured file log level ignored: %s", data)
	}
}

// TestGetLogs_ReturnsAtMostC guards the documented "up to c entries" contract.
// The loop condition must cap output at c (ERROR entries are queried at "debug"
// level so the level filter passes all of them, isolating the count).
func TestGetLogs_ReturnsAtMostC(t *testing.T) {
	logBufferMu.Lock()
	logBuffer = nil
	logBufferMu.Unlock()
	for i := range 5 {
		addToBuffer("ERROR", fmt.Sprintf("m%d", i))
	}

	cases := []struct{ c, want int }{
		{0, 0},
		{2, 2},
		{5, 5},
		{10, 5},                 // capped at what's available
		{int(^uint(0) >> 1), 5}, // untrusted count cannot grow the allocation beyond buffered records
	}
	for _, tc := range cases {
		if got := GetLogs(tc.c, "debug"); len(got) != tc.want {
			t.Errorf("GetLogs(%d) returned %d entries, want %d", tc.c, len(got), tc.want)
		}
	}
}

// InitLogger replaces the package logger while other goroutines are already
// logging — CI caught that as a data race between InitLogger and Warningf.
func TestInitLoggerConcurrentWithLogging(t *testing.T) {
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())

	stop := make(chan struct{})
	var logging sync.WaitGroup
	logging.Add(1)
	go func() {
		defer logging.Done()
		for {
			select {
			case <-stop:
				return
			default:
				Warningf("concurrent %s", "log")
			}
		}
	}()

	for range 10 {
		InitLogger(golog.CRITICAL)
	}
	close(stop)
	logging.Wait()
}

func TestGetFileLogsRetainsShortUnstructuredDiagnostics(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", dir)
	lines := []string{
		"2026/11/01 12:00:00 DEBUG - discarded at error level",
		"panic: SIGSEGV",
		"goroutine",
		"panic: goroutine DEBUG - fatal diagnostic line",
		"2026/11/01 12:00:01 ERROR - retained structured entry",
	}
	if err := os.WriteFile(filepath.Join(dir, logFileName), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := GetFileLogs(10, "error")
	want := []string{lines[4], lines[3], lines[2], lines[1]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("short raw crash lines were lost or severity filter regressed: got %#v, want %#v", got, want)
	}
}

func TestInitLoggerReusesFileWriterAndClosesReplacedWriter(t *testing.T) {
	firstDir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", firstDir)
	InitLogger(golog.DEBUG)
	t.Cleanup(CloseLogger)
	Info("first-log-writer-marker")

	fileRotateMu.Lock()
	first := fileRotate
	fileRotateMu.Unlock()
	if first == nil {
		t.Fatal("file logger was not initialized")
	}

	// Updating only the logging level must not create a second writer
	// for the same log file (leaked descriptors and competing rotations).
	InitLogger(golog.ERROR)
	fileRotateMu.Lock()
	same := fileRotate
	fileRotateMu.Unlock()
	if first != same {
		t.Fatal("InitLogger replaced a writer for the same log path")
	}

	secondDir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", secondDir)
	InitLogger(golog.ERROR)
	fileRotateMu.Lock()
	second := fileRotate
	fileRotateMu.Unlock()
	if second == nil || second == first || second.Filename != filepath.Join(secondDir, logFileName) {
		t.Fatalf("log folder change reused a stale writer: first %p, second %p", first, second)
	}
	// lumberjack closes its *os.File when Close() is called. After a folder
	// switch the old writer should no longer hold an open descriptor.
	if err := os.Remove(filepath.Join(firstDir, logFileName)); err != nil {
		t.Fatalf("old log file remained inaccessible after writer replacement: %v", err)
	}
}
