package singbox

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNewClashHTTPClientDisablesEnvironmentProxy(t *testing.T) {
	client := newClashHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Clash client transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("Clash client must not use HTTP_PROXY/HTTPS_PROXY for local controller requests")
	}
}

func TestClashClientsReuseControllerConnection(t *testing.T) {
	var opened atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"connections":[]}`)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	server.Start()
	defer server.Close()

	runtimeConfigReader.RLock()
	previous := runtimeConfigReader.read
	runtimeConfigReader.RUnlock()
	SetRuntimeConfigReader(func() []byte {
		return fmt.Appendf(nil, `{"experimental":{"clash_api":{"external_controller":%q}}}`, server.Listener.Addr().String())
	})
	t.Cleanup(func() { SetRuntimeConfigReader(previous) })

	for range 5 {
		client := NewClashStatsClient()
		if _, err := client.Connections(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := opened.Load(); got != 1 {
		t.Fatalf("opened %d controller connections for five polls, want 1", got)
	}
}
