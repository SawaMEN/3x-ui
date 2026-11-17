package tail

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReadTailLinesAcrossChunkBoundaries(t *testing.T) {
	content := "first\r\n" + strings.Repeat("a", int(TailChunkSize)*2+7) + "\r\n" +
		strings.Repeat("b", int(TailChunkSize)+3) + "\n\nlast"
	path := filepath.Join(t.TempDir(), "boundaries.log")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, budget := range []int64{1, 5, TailChunkSize - 1, TailChunkSize, TailChunkSize + 1, int64(len(content)) - 1, int64(len(content)), DefaultTailBytes} {
		for _, limit := range []int{-1, 0, 1, 2, 10} {
			t.Run(fmt.Sprintf("bytes=%d/lines=%d", budget, limit), func(t *testing.T) {
				// Independent whole-string oracle: discard the cut oldest record,
				// then retain nonempty complete lines newest first.
				start := max(0, len(content)-int(budget))
				parts := strings.Split(content[start:], "\n")
				if start > 0 {
					parts = parts[1:]
				}
				want := []string{}
				for i := len(parts) - 1; i >= 0 && (limit <= 0 || len(want) < limit); i-- {
					if line := strings.TrimSuffix(parts[i], "\r"); line != "" {
						want = append(want, line)
					}
				}
				got, err := ReadTailLines(path, limit, budget)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("tail differs from reference: error=%v, got %d lines, want %d", err, len(got), len(want))
				}
			})
		}
	}
}

func BenchmarkReadTailLines(b *testing.B) {
	for _, tc := range []struct{ name, content string }{
		{"short-records", strings.Repeat("2026/11/01 10:00:00 INFO example message\n", 4096)},
		{"long-record", strings.Repeat("x", 2<<20) + "\nlast\n"},
		{"cut-long-record", "prefix\n" + strings.Repeat("x", 3<<20)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "bench.log")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := ReadTailLines(path, 2000, 2<<20); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestReadTailLinesNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	content := "one\ntwo\nthree\nfour\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadTailLines(path, 0, DefaultTailBytes)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"four", "three", "two", "one"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestReadTailLinesHonorsByteBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	content := strings.Repeat("0123456789\n", 100)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadTailLines(path, 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || len(got) >= 100 {
		t.Fatalf("got %d lines; expected a bounded tail", len(got))
	}
}

func TestReadTailLinesDiscardsTruncatedOldestLine(t *testing.T) {
	cases := []struct {
		name    string
		content string
		budget  int64
		want    []string
	}{
		{
			name:    "newline terminated",
			content: "old-prefix-12345\nlast\n",
			budget:  int64(len("345\nlast\n")),
			want:    []string{"last"},
		},
		{
			name:    "unterminated final line",
			content: "abcdefghij\nlast",
			budget:  int64(len("ij\nlast")),
			want:    []string{"last"},
		},
		{
			name:    "no complete line within budget",
			content: "abcdefghij",
			budget:  4,
			want:    []string{},
		},
		{
			name:    "unlimited budget retains unterminated first line",
			content: "first\nsecond",
			budget:  int64(len("first\nsecond")),
			want:    []string{"second", "first"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.log")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := ReadTailLines(path, 0, tc.budget)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ReadTailLines() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestReadTailLinesLineLimitPreservesNewestRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	content := "first\nsecond\nthird\nfourth\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTailLines(path, 2, DefaultTailBytes)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"fourth", "third"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadTailLines maxLines=2 = %#v, want %#v", got, want)
	}
}

func TestReadTailLinesNegativeLimitMeansUnlimited(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTailLines(path, -1, DefaultTailBytes)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"two", "one"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("negative maxLines panicked or truncated logs: got %#v, want %#v", got, want)
	}
}

func TestReadTailChunkRejectsShortReadsAfterRotation(t *testing.T) {
	// After Stat, a file can be truncated before its tail chunks are read.
	// ReadAt then returns partial data + io.EOF; accepting the bytes would
	// expose NUL-filled bogus log entries to the dashboard.
	reader := strings.NewReader("short")
	buf := make([]byte, 16)
	if err := readTailChunk(reader, buf, 0); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short read: got error %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestReadTailChunkAcceptsCompleteRead(t *testing.T) {
	reader := strings.NewReader("first line\nlast line\n")
	buf := make([]byte, len("last line\n"))
	if err := readTailChunk(reader, buf, int64(len("first line\n"))); err != nil {
		t.Fatalf("complete read unexpectedly failed: %v", err)
	}
	if string(buf) != "last line\n" {
		t.Fatalf("incomplete read: %q", buf)
	}
}

func TestRecentOutputBufferKeepsOnlyCompleteLatestRecords(t *testing.T) {
	buf := NewRecentOutputBuffer(12)
	for _, chunk := range []string{"older\n", "newer\n", "latest\n"} {
		n, err := buf.Write([]byte(chunk))
		if n != len(chunk) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	if !buf.Truncated() {
		t.Fatal("expected bounded stdout to discard oldest bytes")
	}
	if got := string(buf.CompleteBytes()); got != "latest\n" {
		t.Fatalf("truncated oldest row leaked into stdout: got %q", got)
	}
}

func TestRecentOutputBufferHandlesSingleOversizedWrite(t *testing.T) {
	buf := NewRecentOutputBuffer(8)
	input := strings.Repeat("x", 64) + "\nlast\n"
	if _, err := buf.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
	if got := string(buf.CompleteBytes()); got != "last\n" {
		t.Fatalf("oversized write corrupted the newest complete record: got %q", got)
	}
	if len(buf.data) > 8 || !buf.Truncated() {
		t.Fatalf("output exceeded the byte budget: len=%d, truncated=%v", len(buf.data), buf.Truncated())
	}
}

func TestRecentOutputBufferPreservesCompleteUntruncatedOutput(t *testing.T) {
	buf := NewRecentOutputBuffer(64)
	if _, err := buf.Write([]byte("old\nnew\n")); err != nil {
		t.Fatal(err)
	}
	if buf.Truncated() || string(buf.CompleteBytes()) != "old\nnew\n" {
		t.Fatalf("normal journal capture changed: %q, truncated=%v", buf.CompleteBytes(), buf.Truncated())
	}
}

func TestRecentOutputBufferRingWrapAndEmptyWrites(t *testing.T) {
	const capacity = 19
	buf := NewRecentOutputBuffer(capacity)
	var history string
	for _, chunk := range []string{"alpha\n", "beta\n", "gamma\n", "delta\n", "epsilon\n", "zeta\n"} {
		history += chunk
		if n, err := buf.Write([]byte(chunk)); err != nil || n != len(chunk) {
			t.Fatalf("write %q: %d, %v", chunk, n, err)
		}
		got := string(buf.CompleteBytes())
		expected := history
		if len(expected) > capacity {
			expected = expected[len(expected)-capacity:]
			if i := strings.IndexByte(expected, '\n'); i >= 0 {
				expected = expected[i+1:]
			} else {
				expected = ""
			}
		}
		if got != expected {
			t.Fatalf("ring output after %q: got %q, want %q", chunk, got, expected)
		}
		if len(buf.data) > capacity {
			t.Fatalf("buffer grew past limit: %d", len(buf.data))
		}
	}
	before := string(buf.CompleteBytes())
	wasTruncated := buf.Truncated()
	if n, err := buf.Write(nil); n != 0 || err != nil {
		t.Fatalf("empty write: %d, %v", n, err)
	}
	if string(buf.CompleteBytes()) != before || buf.Truncated() != wasTruncated {
		t.Fatalf("empty write changed captured output")
	}
}
