//go:build linux

package gateway

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestGatewayOperationLockCancellationAndReuse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operation.lock")
	release, err := acquireGatewayLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if other, err := acquireGatewayLock(ctx, path); err == nil {
		other()
		release()
		t.Fatal("overlapping operation acquired lock")
	}
	release()
	next, err := acquireGatewayLock(context.Background(), path)
	if err != nil {
		t.Fatal("released lock unavailable:", err)
	}
	next()
	canceled, done := context.WithCancel(context.Background())
	done()
	if next, err := acquireGatewayLock(canceled, path); err == nil {
		next()
		t.Fatal("canceled operation acquired lock")
	}
}
