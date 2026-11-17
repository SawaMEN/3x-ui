package sudoku

import (
	"bytes"
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
