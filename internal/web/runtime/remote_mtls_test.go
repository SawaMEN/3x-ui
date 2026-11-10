package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// nodeForPlainServer builds an http (non-TLS) node so do()'s token handling can
// be exercised without TLS scaffolding.
func nodeForPlainServer(t *testing.T, srv *httptest.Server, mode, token string) *model.Node {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return &model.Node{
		Id: 1, Name: "n1", Scheme: "http", Address: u.Hostname(), Port: port,
		BasePath: "/", ApiToken: token, Enable: true, AllowPrivateAddress: true,
		TlsVerifyMode: mode,
	}
}

// A persisted or hand-built node row with mtls over cleartext must fail closed.
// The node create/update service already rejects this combination, but runtime
// has to enforce the same rule when receiving a damaged configuration.
func TestStage9RemoteDoMTLSRequiresHTTPS(t *testing.T) {
	var reached bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	for _, token := range []string{"", "legacy-token"} {
		reached = false
		r := NewRemote(nodeForPlainServer(t, srv, "mtls", token), nil)
		if _, err := r.do(context.Background(), http.MethodGet, "ping", nil); err == nil {
			t.Fatal("mtls over http must be rejected before dispatch")
		}
		if reached {
			t.Fatal("request reached an unencrypted endpoint despite mtls setting")
		}
	}

	reached = false
	r := NewRemote(nodeForPlainServer(t, srv, "verify", ""), nil)
	if _, err := r.do(context.Background(), http.MethodGet, "ping", nil); err == nil {
		t.Fatal("non-mtls node without bearer must also fail closed")
	}
	if reached {
		t.Fatal("tokenless node request reached server")
	}
}
