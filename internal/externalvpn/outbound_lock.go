package externalvpn

import (
	"context"

	"golang.org/x/sync/semaphore"
)

// Existing apply/supervision paths use a noncancelable lock. Probes can leave
// the same queue before owning any process or private files.
type outboundMutex struct{ sem *semaphore.Weighted }

func newOutboundMutex() *outboundMutex { return &outboundMutex{sem: semaphore.NewWeighted(1)} }

func (m *outboundMutex) Lock()                                 { _ = m.sem.Acquire(context.Background(), 1) }
func (m *outboundMutex) Unlock()                               { m.sem.Release(1) }
func (m *outboundMutex) LockContext(ctx context.Context) error { return m.sem.Acquire(ctx, 1) }
