package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestLocalHiddifyBatchCoalescesConcurrentMutations(t *testing.T) {
	var applies, api atomic.Int32
	l := NewLocal(LocalDeps{CoreType: func() string { return "hiddify-core" }, APIPort: func() int { api.Add(1); return -1 }, RequestConfigApply: func(context.Context) error { applies.Add(1); return nil }})
	finish := l.BeginConfigBatch()
	nested := l.BeginConfigBatch()
	ib := &model.Inbound{Id: 1, Protocol: model.VLESS, Enable: true, Tag: "vless"}
	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := l.AddUser(context.Background(), ib, map[string]any{"email": "alice"}); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	if err := l.UpdateInbound(context.Background(), ib, ib); err != nil {
		t.Fatal(err)
	}
	if err := l.UpdateUser(context.Background(), ib, "alice", model.Client{Email: "alice", Enable: false}); err != nil {
		t.Fatal(err)
	}
	if err := nested(); err != nil {
		t.Fatal(err)
	}
	if !l.ConfigBatchActive() || applies.Load() != 0 {
		t.Fatal("inner batch applied partial state")
	}
	if err := finish(); err != nil {
		t.Fatal(err)
	}
	if err := finish(); err != nil {
		t.Fatal(err)
	}
	if applies.Load() != 1 || api.Load() != 0 || l.ConfigBatchActive() {
		t.Fatalf("applies=%d API=%d active=%v", applies.Load(), api.Load(), l.ConfigBatchActive())
	}
}

func TestLocalHiddifyBatchErrorAndNoop(t *testing.T) {
	failure := errors.New("apply unavailable")
	calls := 0
	l := NewLocal(LocalDeps{CoreType: func() string { return "hiddify-core" }, RequestConfigApply: func(context.Context) error { calls++; return failure }})
	if err := l.BeginConfigBatch()(); err != nil || calls != 0 {
		t.Fatal("empty batch applied")
	}
	finish := l.BeginConfigBatch()
	if err := l.RemoveUser(context.Background(), &model.Inbound{Protocol: model.VLESS}, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := finish(); !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("lost apply error: %v calls=%d", err, calls)
	}
	if l.ConfigBatchActive() {
		t.Fatal("failed apply leaked active scope")
	}
}

func TestLocalHiddifyDisabledProtocolTransitionAppliesOnce(t *testing.T) {
	calls := 0
	l := NewLocal(LocalDeps{CoreType: func() string { return "hiddify-core" }, RequestConfigApply: func(context.Context) error { calls++; return nil }})
	old := &model.Inbound{Id: 501, Protocol: model.MTProto, Enable: true}
	replacement := &model.Inbound{Id: 501, Protocol: model.VLESS, Enable: false}
	if err := l.UpdateInbound(context.Background(), old, replacement); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("disabled transition apply calls=%d", calls)
	}
}
