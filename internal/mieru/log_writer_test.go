package mieru

import (
	"strings"
	"testing"
)

func TestMitaLogWriterBoundsPartialOutput(t *testing.T) {
	w := &logWriter{}
	chunk := strings.Repeat("x", 16*1024)
	for i := 0; i < 20; i++ {
		if n, err := w.Write([]byte(chunk)); err != nil || n != len(chunk) {
			t.Fatalf("log write: %d, %v", n, err)
		}
	}
	if len(w.buf) > 64*1024 {
		t.Fatalf("unbounded partial log: %d bytes", len(w.buf))
	}
}
