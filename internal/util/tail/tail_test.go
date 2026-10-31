package tail

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { t.Fatal(err) }
	got, err := ReadTailLines(path, 2, DefaultTailBytes)
	if err != nil { t.Fatal(err) }
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
