package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"

	"github.com/SawaMEN/3x-ui/v3/internal/web/websocket"
)

func TestAuditStoppedHubClosesUpgradedSocket(t *testing.T) {
	hub := websocket.NewHub()
	hub.Stop()
	svc := NewWebSocketService(hub)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&ws.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		svc.HandleConnection(conn, "127.0.0.1")
	}))
	defer server.Close()
	conn, _, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.ReadMessage()
	if !ws.IsCloseError(err, ws.CloseNoStatusReceived) {
		t.Fatalf("stopped hub did not promptly close upgraded socket: %v", err)
	}
}
