//go:build linux

package service

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"time"
)

// Separate from the web/CLI operation lock, which callers may already hold.
// The boot restore script takes the same runtime lease before touching nftables.
func gatewayRuntimeLock(ctx context.Context, path string) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = f.Close()
			return nil, err
		}
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN); _ = f.Close() }, nil
		}
		if err != unix.EWOULDBLOCK && err != unix.EAGAIN {
			_ = f.Close()
			return nil, err
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = f.Close()
			return nil, fmt.Errorf("Gateway runtime transition in progress: %w", ctx.Err())
		case <-timer.C:
		}
	}
}
