//go:build linux

package service

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGatewayRuntimeLeaseExcludesBootRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.runtime.lock")
	release, err := gatewayRuntimeLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { release() }()
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("flock is required by Gateway boot restore")
	}
	if err := exec.Command("flock", "-n", path, "true").Run(); err == nil {
		t.Fatal("boot restore acquired live core transition lease")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if unlock, err := gatewayRuntimeLock(ctx, path); err == nil {
		unlock()
		t.Fatal("concurrent transition acquired lease")
	}
	release()
	release = func() {}
	if err := exec.Command("flock", "-n", path, "true").Run(); err != nil {
		t.Fatalf("lease survived completed transition: %v", err)
	}
}
