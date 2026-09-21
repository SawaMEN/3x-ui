//go:build !linux

package systemupdate

import (
	"context"
	"fmt"
)

func GetStatus(ctx context.Context) (Status, error) {
	return Status{}, fmt.Errorf("system package management is only supported on Linux")
}

func Refresh(ctx context.Context) (Status, error) {
	return Status{}, fmt.Errorf("system package management is only supported on Linux")
}

func Apply(ctx context.Context) (UpdateResult, error) {
	return UpdateResult{}, fmt.Errorf("system package management is only supported on Linux")
}
