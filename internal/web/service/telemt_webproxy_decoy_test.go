package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTelemtWebDecoyPreservesSite(t *testing.T) {
	dir := t.TempDir()
	if err := writeTelemtWebDecoy("example.com", dir); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(dir, "index.html")
	page, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "example.com") || strings.Contains(string(page), "{{DOMAIN}}") {
		t.Fatal("domain was not rendered")
	}
	custom := []byte("<!doctype html><title>My site</title><p>Keep this content</p>")
	if err := os.WriteFile(index, custom, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeTelemtWebDecoy("other.example.com", dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(index)
	if err != nil || string(got) != string(custom) {
		t.Fatalf("reconfiguration replaced the site: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary page leaked: %v, %v", entries, err)
	}
}

func TestTelemtWebDecoyRejectsInvalidIndex(t *testing.T) {
	for _, kind := range []string{"empty", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			index := filepath.Join(dir, "index.html")
			var err error
			switch kind {
			case "empty":
				err = os.WriteFile(index, nil, 0o644)
			case "directory":
				err = os.Mkdir(index, 0o755)
			case "symlink":
				err = os.Symlink(filepath.Join(dir, "missing.html"), index)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := writeTelemtWebDecoy("example.com", dir); err == nil {
				t.Fatal("accepted an invalid site index")
			}
		})
	}
}
