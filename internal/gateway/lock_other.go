//go:build !linux

package gateway

import (
	"context"
	"fmt"
)

func AcquireOperation(context.Context) (func(), error) {
	return nil, fmt.Errorf("Gateway Mode requires Linux")
}
