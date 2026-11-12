package outbound

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

func TestAutomaticProbeCancellationStopsTemporaryCore(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	var proc *stubProcess
	withStubProcess(t, func(cfg *xray.Config, path string) batchProcess {
		proc = &stubProcess{cfg: cfg, serveSocks: true}
		return proc
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	items := `[{"tag":"a","protocol":"socks","settings":{"servers":[{"address":"127.0.0.1","port":1080}]}}]`
	go func() {
		_, err := (&OutboundService{}).TestAutomaticOutboundsContext(ctx, items, server.URL, items, "real", "xray")
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		<-done
		t.Fatal("probe did not reach local HTTP fixture")
	}
	cancel()
	select {
	case err := <-done:
		close(release)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("probe error=%v, want cancellation", err)
		}
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("automatic HTTP probe ignored cancellation")
	}
	if proc == nil || proc.IsRunning() {
		t.Fatal("temporary core survived canceled probe")
	}
}

func TestAutomaticProbeCancelsReadinessWait(t *testing.T) {
	ports, release, err := reserveLoopbackPorts(1)
	if err != nil {
		t.Fatal(err)
	}
	release()
	ctx, cancel := context.WithCancel(context.Background())
	proc := &stubProcess{running: true}
	done := make(chan *portsReadyError, 1)
	go func() { done <- waitForPortsReady(ctx, proc, ports, 10*time.Second) }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled readiness reported ready")
		}
	case <-time.After(time.Second):
		t.Fatal("port readiness did not cancel")
	}
}

func TestAutomaticSpeedProbeCancellationSkipsQueuedWorkers(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if requests.Add(1) == 1 {
			close(entered)
		}
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	defer close(release)
	var proc *stubProcess
	withStubProcess(t, func(cfg *xray.Config, path string) batchProcess {
		proc = &stubProcess{cfg: cfg, serveSocks: true}
		return proc
	})
	base := `{"tag":"a","protocol":"socks","settings":{"servers":[{"address":"127.0.0.1","port":1080}]}}`
	items := "[" + base + "," + strings.Replace(base, `"a"`, `"b"`, 1) + "," + strings.Replace(base, `"a"`, `"c"`, 1) + "]"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := (&OutboundService{}).TestAutomaticOutboundsContext(ctx, items, server.URL, items, "speed", "xray")
		done <- err
	}()
	<-entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("speed workers did not finish after cancellation")
	}
	if requests.Load() != 1 {
		t.Fatalf("queued speed probes continued after cancel: requests=%d", requests.Load())
	}
	if proc == nil || proc.IsRunning() {
		t.Fatal("temporary speed-test core remained running")
	}
}
