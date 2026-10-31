package tail

import (
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
