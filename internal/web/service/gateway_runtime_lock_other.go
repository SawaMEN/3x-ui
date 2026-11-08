//go:build !linux

package service

import "context"

func gatewayRuntimeLock(ctx context.Context, path string) (func(), error) {
	return func() {}, ctx.Err()
}
