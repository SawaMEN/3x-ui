package sudoku

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOptionalSudokuRequiresExplicitInstallation(t *testing.T) {
	dir := t.TempDir()
	optional := filepath.Join(dir, "optional", "sudoku")
	if err := os.MkdirAll(optional, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := []byte("#!/bin/sh\nexit 0\n")
	hash := sha256.Sum256(binary)
	if err := os.WriteFile(GetBinaryPath(optional), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeInstalledVersion(optional, "panel-test"); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(map[string]any{"sudoku": map[string]string{"binary_sha256": hex.EncodeToString(hash[:])}})
	if err := os.WriteFile(filepath.Join(optional, "speed-components.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if IsInstalled(dir) {
		t.Fatal("optional payload appears installed")
	}
	path, found, err := installOptional(context.Background(), dir)
	if err != nil || !found || path != GetBinaryPath(dir) {
		t.Fatal(path, found, err)
	}
	if !IsInstalled(dir) || readInstalledVersion(dir) != "panel-test" {
		t.Fatal("explicit installation lost binary version")
	}
	if err := os.WriteFile(GetBinaryPath(optional), []byte("corrupt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := installOptional(context.Background(), dir); err == nil {
		t.Fatal("corrupt optional payload accepted")
	}
	got, err := os.ReadFile(GetBinaryPath(dir))
	if err != nil || string(got) != string(binary) {
		t.Fatal("failed update changed installed Sudoku")
	}
}

func TestUnversionedSudokuIsNotInstalled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(GetBinaryPath(dir), []byte("incomplete"), 0o755); err != nil {
		t.Fatal(err)
	}
	if IsInstalled(dir) {
		t.Fatal("unversioned binary presented as installed")
	}
}
