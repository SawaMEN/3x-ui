package sub

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRemoteRoutingRefreshCancellationDoesNotDelayRetry(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	client := remoteRoutingTestClient(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-release:
			}
		}
		return remoteRoutingResponse(http.StatusOK, `{"Name":"synthetic"}`), nil
	})
	resolver := newRemoteRoutingResolver(client, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- resolver.refreshSourceContext(ctx, remoteRoutingHapp, "https://routing.example.test/rules")
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("refresh returned %v", err)
		}
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("scheduled refresh did not cancel its HTTP request")
	}
	resolver.mu.Lock()
	cached, attempted := len(resolver.entries), len(resolver.lastAttempt)
	resolver.mu.Unlock()
	if cached != 0 || attempted != 0 {
		t.Fatalf("canceled load changed cache/retry schedule: entries=%d attempts=%d", cached, attempted)
	}
	if err := resolver.refreshSourceContext(context.Background(), remoteRoutingHapp, "https://routing.example.test/rules"); err != nil {
		t.Fatalf("immediate retry blocked: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("requests=%d, want 2", calls.Load())
	}
}

func TestRemoteRoutingCanceledWaiterDoesNotCancelSharedFetch(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	resolver := newRemoteRoutingResolver(remoteRoutingTestClient(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		<-release
		return remoteRoutingResponse(http.StatusOK, `{"Name":"shared"}`), nil
	}), false)
	leader := make(chan error, 1)
	go func() { leader <- resolver.refreshSource(remoteRoutingHapp, "https://routing.example.test/shared") }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	follower := make(chan error, 1)
	go func() {
		follower <- resolver.refreshSourceContext(ctx, remoteRoutingHapp, "https://routing.example.test/shared")
	}()
	cancel()
	select {
	case err := <-follower:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("waiter error=%v", err)
		}
	case <-time.After(time.Second):
		close(release)
		<-leader
		<-follower
		t.Fatal("canceled waiter remained blocked on another fetch")
	}
	close(release)
	if err := <-leader; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("requests=%d, want coalesced single request", calls.Load())
	}
	if len(resolver.entries) != 1 {
		t.Fatal("waiter cancellation removed leader's cache")
	}
}

func TestRemoteRoutingCanceledPersistedLoadWait(t *testing.T) {
	resolver := newRemoteRoutingResolver(nil, true)
	if err := resolver.loadMu.Acquire(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	defer resolver.loadMu.Release(1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- resolver.ensurePersistedLoadedContext(ctx) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("persisted-cache load waited for another panel after cancellation")
	}
	if resolver.loaded {
		t.Fatal("canceled load was marked successful")
	}
}

func TestRemoteRoutingConfiguredSourcesStopAfterCancellation(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	resolver := newRemoteRoutingResolver(remoteRoutingTestClient(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	}), false)
	original := routingSourceResolver
	routingSourceResolver = resolver
	defer func() { routingSourceResolver = original }()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RefreshRemoteRoutingSourcesContext(ctx, "https://routing.example.test/happ?secret=private", "https://routing.example.test/clash", "https://routing.example.test/json")
	}()
	<-started
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "private") {
		t.Fatalf("cancellation error=%v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("started %d loads after cancellation, want 1", calls.Load())
	}
}

func TestRemoteRoutingCancellationClosesHTTPSRequest(t *testing.T) {
	started, serverCanceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		close(started)
		<-req.Context().Done()
		close(serverCanceled)
	}))
	defer server.Close()
	resolver := newRemoteRoutingResolver(server.Client(), false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- resolver.refreshSourceContext(ctx, remoteRoutingHapp, server.URL+"/rules") }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTPS request did not reach local test server")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled HTTPS refresh is still running")
	}
	select {
	case <-serverCanceled:
	case <-time.After(time.Second):
		t.Fatal("upstream request was not closed")
	}
}
