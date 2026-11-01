package web

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestPanelURLRestartClosesKeepAliveBeforeWaitingForJobs(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "old panel") }))
	transport := &http.Transport{}
	client := &http.Client{Transport: transport, Timeout: time.Second}
	fetch := func(url string) (string, error) {
		response, err := client.Get(url)
		if err != nil {
			return "", err
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		return string(data), err
	}
	if _, err := fetch(old.URL + "/old/"); err != nil {
		t.Fatal(err)
	}
	gate, started := make(chan struct{}), make(chan struct{})
	scheduler := cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger)))
	if _, err := scheduler.AddFunc("@every 1s", func() { close(started); <-gate }); err != nil {
		t.Fatal(err)
	}
	scheduler.Start()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not start")
	}
	s := NewServer()
	s.listener, s.httpServer, s.cron = old.Listener, old.Config, scheduler
	stopped := make(chan error, 1)
	finished := make(chan struct{})
	go func() { stopped <- s.StopPanelOnly(); close(finished) }()
	defer func() {
		close(gate)
		<-scheduler.Stop().Done()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("retired server cleanup did not finish")
		}
		transport.CloseIdleConnections()
		old.Close()
	}()
	select {
	case <-s.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("restart did not cancel panel context")
	}
	// An already connected browser must not keep using the retired router.
	if body, err := fetch(old.URL + "/old/"); err == nil {
		t.Fatalf("old URL remains reachable during restart: %s", body)
	}
	select {
	case err := <-stopped:
		if err != nil && !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			t.Fatal(err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("panel restart still waits for a blocked background job")
	}
	listener, err := net.Listen("tcp", old.Listener.Addr().String())
	if err != nil {
		t.Fatalf("new panel cannot bind: %v", err)
	}
	replacement := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/new/" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, "new panel")
	})}
	go func() { _ = replacement.Serve(listener) }()
	defer replacement.Close()
	if body, err := fetch(old.URL + "/new/"); err != nil || body != "new panel" {
		t.Fatalf("new URL not available: %q, %v", body, err)
	}
}
