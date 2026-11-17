package panel

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	ws "github.com/gorilla/websocket"

	"github.com/SawaMEN/3x-ui/v3/internal/web/websocket"
)

type panickingWebSocketConn struct {
	net.Conn
	readPanic, writePanic atomic.Bool
}

func (c *panickingWebSocketConn) Read(p []byte) (int, error) {
	if c.readPanic.Load() {
		panic("network read panic")
	}
	return c.Conn.Read(p)
}

func (c *panickingWebSocketConn) Write(p []byte) (int, error) {
	if c.writePanic.Load() {
		panic("network write panic")
	}
	return c.Conn.Write(p)
}

func TestWebSocketPumpsRecoverNetworkPanic(t *testing.T) {
	for _, kind := range []string{"read", "write"} {
		t.Run(kind, func(t *testing.T) {
			upgraded := make(chan *ws.Conn, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				peer, err := (&ws.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				upgraded <- peer
			}))
			defer server.Close()
			var transport *panickingWebSocketConn
			dialer := ws.Dialer{NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				c, err := (&net.Dialer{}).DialContext(ctx, network, address)
				if err != nil {
					return nil, err
				}
				transport = &panickingWebSocketConn{Conn: c}
				return transport, nil
			}}
			conn, response, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if response != nil {
				defer response.Body.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			peer := <-upgraded
			defer peer.Close()
			hub := websocket.NewHub()
			defer hub.Stop()
			go hub.Run()
			client := websocket.NewClient("panic-test")
			hub.Register(client)
			svc := NewWebSocketService(hub)
			var escaped any
			func() {
				defer func() { escaped = recover() }()
				if kind == "read" {
					transport.readPanic.Store(true)
					svc.readPump(client, conn)
				} else {
					transport.writePanic.Store(true)
					client.Send <- []byte("test")
					svc.writePump(client, conn)
				}
			}()
			if escaped != nil {
				t.Fatalf("%s pump panic escaped and would terminate panel: %v", kind, escaped)
			}
		})
	}
}
