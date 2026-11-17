package logger

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type failedDiagnosticWriter struct{ err error }

func (w failedDiagnosticWriter) Write(p []byte) (int, error) { return 0, w.err }

func TestFileBackendFailureRetainsOriginalRecordInFallback(t *testing.T) {
	for _, failure := range []error{errors.New("disk full"), nil} {
		var fallback bytes.Buffer
		writer := diagnosticFileWriter{target: failedDiagnosticWriter{err: failure}, fallback: &fallback}
		n, err := writer.Write([]byte("ORIGINAL ERROR RECORD\n"))
		if n != 0 || err == nil {
			t.Fatalf("write=%d, %v", n, err)
		}
		if failure == nil && !errors.Is(err, io.ErrShortWrite) {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(GetLogs(10, "error"), "\n"), "panel log file write failed") {
			t.Fatal("file failure missing from UI buffer")
		}
		if !strings.Contains(fallback.String(), "panel log file write failed") || !strings.Contains(fallback.String(), "ORIGINAL ERROR RECORD") {
			t.Fatal(fallback.String())
		}
	}
}
