package web

import (
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/web/global"
)

type panelFailedListener struct{ err error }

func (l panelFailedListener) Accept() (net.Conn, error) { return nil, l.err }
func (panelFailedListener) Close() error                { return nil }
func (panelFailedListener) Addr() net.Addr              { return &net.TCPAddr{} }

func TestPanelListenerFailureRequestsRestart(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		stopped bool
		want    int
	}{
		{"unexpected", errors.New("accept failed"), false, 1},
		{"normal close", http.ErrServerClosed, false, 0},
		{"closed listener", net.ErrClosed, false, 0},
		{"stopping", errors.New("accept failed during shutdown"), true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			global.SetRestartHook(func() { calls++ })
			t.Cleanup(func() { global.SetRestartHook(nil) })
			server := NewServer()
			defer server.cancel()
			if tc.stopped {
				server.cancel()
			}
			server.serveHTTP(&http.Server{}, panelFailedListener{tc.err})
			if calls != tc.want {
				t.Fatalf("restart requests = %d, want %d", calls, tc.want)
			}
		})
	}
}
