//go:build !linux

package coremigration

import "errors"

func AcquirePanelLease() (func(), error) { return func() {}, nil }
func assertStopped() error               { return errors.New("installed-panel core migration requires Linux") }
