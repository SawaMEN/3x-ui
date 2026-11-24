package runtime

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

// Custom DialContext must not accidentally disable HTTP/2 for nodes behind
// an h2-speaking HTTPS endpoint. HTTP/1.1-only endpoints still work as before.
func TestNodeTransportNegotiatesHTTP2(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.ProtoMajor != 2 {
			t.Errorf("node request used HTTP/%d instead of HTTP/2", req.ProtoMajor)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport := newNodeTransport(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}, true)
	defer transport.CloseIdleConnections()
	if !transport.ForceAttemptHTTP2 {
		t.Fatal("node transport must negotiate HTTP/2")
	}
	if transport.TLSHandshakeTimeout != 10*time.Second {
		t.Fatalf("TLS handshake timeout = %s, want 10s", transport.TLSHandshakeTimeout)
	}

	req, err := http.NewRequestWithContext(
		netsafe.ContextWithAllowPrivate(context.Background(), true),
		http.MethodGet, server.URL, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Fatalf("node negotiated HTTP/%d, want HTTP/2", resp.ProtoMajor)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
}
