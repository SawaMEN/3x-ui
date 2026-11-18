//go:build !linux

package systemupdate

import (
	"context"
	"fmt"
)

func Cleanup(ctx context.Context) (CleanupResult, error) {
	return CleanupResult{}, fmt.Errorf("cache cleanup is only supported on Linux")
}
