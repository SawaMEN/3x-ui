package job

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
)

func TestAutoBalancerJobSkipsCanceledTick(t *testing.T) {
	setupIntegrationDB(t)
	queries := 0
	cb := database.GetDB().Callback().Query()
	if err := cb.Before("gorm:query").Register("audit:count-auto-job", func(tx *gorm.DB) { queries++ }); err != nil {
		t.Fatal(err)
	}
	defer cb.Remove("audit:count-auto-job")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	NewOutboundAutoBalancerJob(ctx).Run()
	if queries != 0 {
		t.Fatalf("canceled job performed %d database reads", queries)
	}
}

func TestAutoBalancerJobCancellationSkipsApply(t *testing.T) {
	setupIntegrationDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cb := database.GetDB().Callback().Query()
	entered, release := make(chan struct{}), make(chan struct{})
	queries := 0
	if err := cb.Before("gorm:query").Register("audit:cancel-auto-job", func(tx *gorm.DB) {
		queries++
		if queries != 1 {
			return
		}
		close(entered)
		select {
		case <-tx.Statement.Context.Done():
			_ = tx.AddError(tx.Statement.Context.Err())
		case <-release:
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer cb.Remove("audit:cancel-auto-job")
	done := make(chan struct{})
	go func() { NewOutboundAutoBalancerJob(ctx).Run(); close(done) }()
	<-entered
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("canceled job remained in database")
	}
	if queries != 1 {
		t.Fatalf("job entered apply after cancellation: queries=%d", queries)
	}
}
