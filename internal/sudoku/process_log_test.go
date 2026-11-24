package sudoku

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLogWriterBoundsIncompleteLines(t *testing.T) {
	const limit = 64 << 10
	writer := &logWriter{label: "test"}
	prefix := bytes.Repeat([]byte("a"), limit)
	suffix := bytes.Repeat([]byte("b"), limit)
	for _, chunk := range [][]byte{prefix, suffix} {
		n, err := writer.Write(chunk)
		if err != nil || n != len(chunk) {
			t.Fatalf("log write returned %d, %v; want %d bytes", n, err, len(chunk))
		}
	}
	if writer.buf != string(suffix) {
		t.Fatalf("unterminated log line retained %d bytes; want its last %d", len(writer.buf), limit)
	}
}

func TestValidationBoundsDiagnosticOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	binary := filepath.Join(t.TempDir(), "validator")
	script := "#!/bin/sh\nhead -c 131072 /dev/zero | tr '\\000' x\nprintf '\\nfinal-validation-error\\n'\nexit 1\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	err := validateConfig(binary, "unused.json")
	if err == nil || !strings.Contains(err.Error(), "final-validation-error") || len(err.Error()) > 2300 {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
