package brokerclient

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// Each synthetic broker is loopback-only and is closed before the test returns.
func stalledBroker(t *testing.T, handshake bool) (*Client, <-chan struct{}) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if !handshake {
			var greeting [3]byte
			if _, err = io.ReadFull(conn, greeting[:]); err != nil {
				return
			}
			close(ready)
			<-release
			return
		}
		var greeting [3]byte
		if _, err = io.ReadFull(conn, greeting[:]); err != nil {
			return
		}
		if _, err = conn.Write([]byte{5, 2}); err != nil {
			return
		}
		var auth [2]byte
		if _, err = io.ReadFull(conn, auth[:]); err != nil {
			return
		}
		if _, err = io.CopyN(io.Discard, conn, int64(auth[1])); err != nil {
			return
		}
		var length [1]byte
		if _, err = io.ReadFull(conn, length[:]); err != nil {
			return
		}
		if _, err = io.CopyN(io.Discard, conn, int64(length[0])); err != nil {
			return
		}
		if _, err = conn.Write([]byte{1, 0}); err != nil {
			return
		}
		var request [10]byte
		if _, err = io.ReadFull(conn, request[:]); err != nil {
			return
		}
		if _, err = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
			return
		}
		var frame [5]byte
		if _, err = io.ReadFull(conn, frame[:]); err != nil {
			return
		}
		close(ready)
		<-release
	}()
	client, err := NewWithConfig(Config{Address: listener.Addr().String(), Password: "test-password", Users: map[string]string{"alice": "route"}}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close(); _ = listener.Close(); close(release); <-done })
	return client, ready
}

func waitBrokerReady(t *testing.T, ready <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("synthetic broker did not reach the stalled phase")
	}
}

func TestWaitCancellationInterruptsAuthentication(t *testing.T) {
	client, ready := stalledBroker(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- client.Wait(ctx, 0, 1) }()
	waitBrokerReady(t, ready)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait=%v, want cancellation", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("canceled request remained blocked in authentication")
	}
}

func TestWaitCancellationDoesNotWaitForBusyLane(t *testing.T) {
	client, ready := stalledBroker(t, true)
	first := make(chan error, 1)
	go func() { first <- client.Wait(context.Background(), 0, 1) }()
	waitBrokerReady(t, ready)
	ctx, cancel := context.WithCancel(context.Background())
	second := make(chan error, 1)
	entered := make(chan struct{})
	go func() { close(entered); second <- client.Wait(ctx, 0, 1) }()
	<-entered
	select {
	case err := <-second:
		t.Fatalf("request did not wait for the busy lane: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	defer func() { client.Close(); <-first }()
	select {
	case err := <-second:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait=%v, want cancellation", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("canceled request remained queued behind another budget request")
	}
}
