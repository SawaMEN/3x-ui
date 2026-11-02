//go:build linux

package singbox

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestStage4RejectsBrokenCoreExecutables(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if string(foreign[:4]) != "\x7fELF" {
		t.Fatal("test binary is not ELF")
	}
	// Retain a complete executable while switching its declared architecture.
	machine := uint16(183)
	if runtime.GOARCH == "arm64" {
		machine = 62
	}
	binary.LittleEndian.PutUint16(foreign[18:20], machine)
	for _, kind := range []string{"missing", "not-executable", "bad-format", "missing-loader", "wrong-architecture", "wrong-identity", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "core")
			cfg := filepath.Join(dir, "config.json")
			if err := os.WriteFile(cfg, []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			data := []byte("#!/bin/sh\necho unrelated-program\n")
			mode := os.FileMode(0700)
			switch kind {
			case "not-executable":
				mode = 0600
			case "bad-format":
				data = []byte("not an executable")
			case "missing-loader":
				data = []byte("#!/stage4/missing/interpreter\n")
			case "wrong-architecture":
				data = foreign
			}
			if kind != "missing" {
				if err := os.WriteFile(path, data, mode); err != nil {
					t.Fatal(err)
				}
			}
			p := NewProcessWithOptions(cfg, ProcessOptions{Binary: path, Name: "stage4", RunCommand: "run", VersionPrefix: "sing-box version "}, true)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			if kind == "cancelled" {
				cancel()
			}
			if err := p.Start(ctx); err == nil {
				t.Fatal("broken or cancelled core reported successful start")
			}
			if p.IsRunning() {
				_ = p.Stop()
				t.Fatal("rejected executable left a running core")
			}
		})
	}
}
