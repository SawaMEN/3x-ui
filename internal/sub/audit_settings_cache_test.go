package sub

import (
	"testing"
	"time"
)

func TestAuditSettingsRequestAfterPurgeDoesNotJoinStaleLoad(t *testing.T) {
	cache := newInboundSettingsCache(time.Minute)
	started := make(chan struct{})
	release := make(chan struct{})
	oldDone := make(chan struct{}, 1)
	defer close(release)
	go func() {
		_, _ = cache.settingsFor(1, func() (string, error) {
			close(started)
			<-release
			return `{"password":"old-value"}`, nil
		})
		oldDone <- struct{}{}
	}()
	<-started
	cache.purgeAll()
	type result struct {
		value string
		err   error
	}
	fresh := make(chan result, 1)
	go func() {
		value, err := cache.settingsFor(1, func() (string, error) { return `{"password":"new-value"}`, nil })
		fresh <- result{value, err}
	}()
	select {
	case got := <-fresh:
		if got.err != nil || got.value != `{"password":"new-value"}` {
			t.Error("request after edit received stale settings", got.err)
		}
	case <-time.After(time.Second):
		t.Error("request after purge joined the pre-edit load")
	}
	// Release the old request before checking that it did not replace the fresh cache.
	release <- struct{}{}
	<-oldDone
	if got, ok := cache.lookup(1); !ok || got != `{"password":"new-value"}` {
		t.Error("old load overwrote or displaced the current generation")
	}
}
