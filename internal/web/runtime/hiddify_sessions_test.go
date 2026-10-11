package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestLocalHiddifySessionsUseClashAndPreserveIdentity(t *testing.T) {
	var mu sync.Mutex
	closed := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer phase6" {
			w.WriteHeader(401)
			return
		}
		if r.Method == http.MethodDelete {
			mu.Lock()
			closed = append(closed, r.URL.Path)
			mu.Unlock()
			if strings.HasSuffix(r.URL.Path, "fails") {
				w.WriteHeader(500)
			} else {
				w.WriteHeader(204)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"connections":[{"id":"tcp","metadata":{"type":"vless/vpn","user":"alice","sourceIP":"::ffff:192.0.2.1","sourcePort":"12","network":"tcp"}},{"id":"udp","metadata":{"type":"tuic/other","user":"alice","sourceIP":"2001:0db8:0:0:0:0:0:1","sourcePort":"13","network":"udp"}},{"id":"fails","metadata":{"type":"vless/vpn","user":"bob","sourceIP":"192.0.2.2"}},{"id":"anonymous","metadata":{"type":"mixed/vpn","sourceIP":"192.0.2.3"}},{"metadata":{"user":"alice"}}]}`))
	}))
	defer server.Close()
	cfg, _ := json.Marshal(map[string]any{"experimental": map[string]any{"clash_api": map[string]any{"external_controller": strings.TrimPrefix(server.URL, "http://"), "secret": "phase6"}}})
	singbox.SetRuntimeConfigReader(func() []byte { return cfg })
	t.Cleanup(func() { singbox.SetRuntimeConfigReader(nil) })
	local := NewLocal(LocalDeps{CoreType: func() string { return "hiddify-core" }})
	sessions, err := local.ActiveSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 4 || sessions[0].Source != "192.0.2.1:12" || sessions[1].Source != "[2001:db8::1]:13" || sessions[0].CreatedAt != 0 || sessions[1].Network != "udp" {
		t.Fatalf("session identity lost: %+v", sessions)
	}
	if n, err := local.DisconnectUsersSessions(context.Background(), "vpn", []string{"", "alice"}); n != 1 || err != nil {
		t.Fatalf("filtered disconnect: %d %v", n, err)
	}
	if n, err := local.DisconnectUsersSessions(context.Background(), "", []string{""}); n != 0 || err == nil {
		t.Fatal("empty user accepted")
	}
	if n, err := local.DisconnectInboundSessions(context.Background(), ""); n != 0 || err == nil {
		t.Fatal("empty inbound accepted")
	}
	if n, err := local.DisconnectUsersSessions(context.Background(), "", []string{"alice", "bob"}); n != 2 || err == nil {
		t.Fatalf("partial failure lost: %d %v", n, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(closed) != 4 {
		t.Fatalf("wrong connections closed: %v", closed)
	}
	for _, path := range closed {
		if strings.HasSuffix(path, "anonymous") {
			t.Fatal("anonymous connection selected by empty user")
		}
	}
}
