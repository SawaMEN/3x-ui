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
