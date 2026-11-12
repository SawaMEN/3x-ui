package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service/outbound"
)

func TestAutomaticProbeCancellationPreservesChoice(t *testing.T) {
	sub, _ := setupTimedBalancer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	autoBalanceProbe = func(_ context.Context, items, target, all, mode, core string) ([]*outbound.TestOutboundResult, error) {
		calls++
		cancel()
		return []*outbound.TestOutboundResult{{Tag: "b", Success: true, HTTPStatus: 200, Delay: 1, DownloadMbps: 999}}, nil
	}
	count, err := (&OutboundSubscriptionService{}).ProbeAutomaticSubscriptionsContext(ctx, sub.Id)
	if count != 0 || !errors.Is(err, context.Canceled) {
		t.Errorf("count=%d error=%v, want canceled without changes", count, err)
	}
	saved, err := (&OutboundSubscriptionService{}).Get(sub.Id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SelectedTag != sub.SelectedTag || saved.LastSwitch != sub.LastSwitch || saved.ProbeResults != sub.ProbeResults || calls != 1 {
		t.Fatalf("canceled probe committed/continued: selected=%q, calls=%d", saved.SelectedTag, calls)
	}
}

func TestAutomaticApplyCanceledTickPreservesAcknowledgement(t *testing.T) {
	sub, _ := setupTimedBalancer(t)
	if err := database.GetDB().Model(sub).Updates(map[string]any{"applied_tag": "a", "apply_error": "previous diagnostic"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	count, err := (&OutboundSubscriptionService{}).ApplyAutomaticSelectionsContext(ctx)
	if count != 0 || !errors.Is(err, context.Canceled) {
		t.Errorf("count=%d error=%v, want cancellation", count, err)
	}
	saved, err := (&OutboundSubscriptionService{}).Get(sub.Id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.AppliedTag != "a" || saved.ApplyError != "previous diagnostic" {
		t.Fatalf("canceled apply overwrote acknowledged state: %+v", saved)
	}
}

func TestAutomaticBalancerDatabaseReadsCancel(t *testing.T) {
	for _, operation := range []string{"probe", "apply"} {
		t.Run(operation, func(t *testing.T) {
			sub, _ := setupTimedBalancer(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			cb := database.GetDB().Callback().Query()
			if err := cb.Before("gorm:query").Register("audit:cancel-auto-query", func(tx *gorm.DB) {
				if tx.Statement.Table != "outbound_subscriptions" {
					return
				}
				once.Do(func() { close(entered) })
				select {
				case <-tx.Statement.Context.Done():
					_ = tx.AddError(tx.Statement.Context.Err())
				case <-release:
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer cb.Remove("audit:cancel-auto-query")
			autoBalanceProbe = func(ctx context.Context, items, target, all, mode, core string) ([]*outbound.TestOutboundResult, error) {
				return nil, ctx.Err()
			}
			done := make(chan error, 1)
			go func() {
				if operation == "probe" {
					_, err := (&OutboundSubscriptionService{}).ProbeAutomaticSubscriptionsContext(ctx, sub.Id)
					done <- err
				} else {
					_, err := (&OutboundSubscriptionService{}).ApplyAutomaticSelectionsContext(ctx)
					done <- err
				}
			}()
			<-entered
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				close(release)
				<-done
				t.Fatal("database read ignored cancellation")
			}
		})
	}
}
