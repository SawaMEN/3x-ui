package singbox

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type clashRetryRoundTripper func(*http.Request) (*http.Response, error)

func (f clashRetryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestConnectionsRetryTemporaryRefusal(t *testing.T) {
	var calls atomic.Int32
	client := &ClashStatsClient{
		baseURL: "http://127.0.0.1:10090",
		client: &http.Client{Transport: clashRetryRoundTripper(func(*http.Request) (*http.Response, error) {
			if calls.Add(1) <= 2 {
				return nil, syscall.ECONNREFUSED
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body: io.NopCloser(strings.NewReader(`{"connections":[{"id":"session-1"}]}`)),
			}, nil
		})},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connections, err := client.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || len(connections) != 1 || connections[0].ID != "session-1" {
		t.Fatalf("calls=%d connections=%+v", calls.Load(), connections)
	}
}

func TestConnectionsDoesNotRetryAuthenticationFailure(t *testing.T) {
	var calls atomic.Int32
	client := &ClashStatsClient{
		baseURL: "http://127.0.0.1:10090",
		client: &http.Client{Transport: clashRetryRoundTripper(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{},
				Body: io.NopCloser(strings.NewReader(`{"message":"unauthorized"}`)),
			}, nil
		})},
	}
	_, err := client.Connections(context.Background())
	if err == nil || calls.Load() != 1 {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}

func TestConnectionsRetryHonorsContextCancellation(t *testing.T) {
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	client := &ClashStatsClient{
		baseURL: "http://127.0.0.1:10090",
		client: &http.Client{Transport: clashRetryRoundTripper(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			cancel()
			return nil, syscall.ECONNREFUSED
		})},
	}
	_, err := client.Connections(ctx)
	if !errors.Is(err, context.Canceled) || calls.Load() != 1 {
		t.Fatalf("calls=%d err=%v", calls.Load(), err)
	}
}
