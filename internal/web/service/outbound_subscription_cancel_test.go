package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestOutboundRefreshStopsOnPanelCancellationWithoutDamagingProviderState(t *testing.T) {
	setupSettingTestDB(t)
	started, release := make(chan struct{}), make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	row := &model.OutboundSubscription{Remark: "slow provider", Url: provider.URL, TagPrefix: "slow-", Enabled: true, AllowPrivate: true, UpdateInterval: 600, LastError: "previous provider warning"}
	if err := database.GetDB().Create(row).Error; err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		_, err := (&OutboundSubscriptionService{}).RefreshAllEnabled(ctx)
		result <- err
		close(finished)
	}()
	defer func() { close(release); cancel(); provider.Close(); <-finished }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not reach provider")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled refresh: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("subscription refresh delayed panel restart after cancellation")
	}
	var saved model.OutboundSubscription
	if err := database.GetDB().First(&saved, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if saved.LastError != row.LastError || saved.LastUpdated != 0 {
		t.Fatalf("shutdown changed provider state: %+v", saved)
	}
}
