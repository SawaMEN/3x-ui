package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

func TestStage9NodeRequestsNeverFollowRedirects(t *testing.T) {
	var forwarded atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	// Even a redirect that keeps the host should not move a node credential
	// from the configured API route to a different endpoint.
	for _, dest := range []string{receiver.URL + "/capture", "/capture"} {
		var localForwarded atomic.Int32
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/capture" {
				localForwarded.Add(1)
				return
			}
			w.Header().Set("Location", dest)
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		u, err := url.Parse(source.URL)
		if err != nil {
			t.Fatal(err)
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			t.Fatal(err)
		}
		n := &model.Node{Id: port, Scheme: "http", Address: u.Hostname(), Port: port, AllowPrivateAddress: true, TlsVerifyMode: "verify"}
		client, err := HTTPClientForNode(n, "")
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(netsafe.ContextWithAllowPrivate(context.Background(), true), http.MethodPost, source.URL+"/node", strings.NewReader("sensitive-config"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer must-not-escape")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusTemporaryRedirect || localForwarded.Load() != 0 || forwarded.Load() != 0 {
			t.Fatalf("node request followed redirect: status=%d local=%d external=%d", resp.StatusCode, localForwarded.Load(), forwarded.Load())
		}
		source.Close()
	}
}

func TestStage9NodeTrustModeRejectsInvalidOrCleartextMTLS(t *testing.T) {
	for _, tc := range []struct {
		name, mode, scheme string
	}{
		{"invalid TLS mode", "unexpected", "https"},
		{"mTLS over HTTP", "mtls", "http"},
		{"pin over HTTP", "pin", "http"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := &model.Node{Id: 9009, Address: "example.com", Scheme: tc.scheme, Port: 443, TlsVerifyMode: tc.mode}
			c, err := HTTPClientForNode(n, "")
			if err == nil || c != nil {
				t.Fatalf("insecure trust configuration unexpectedly accepted: client=%v err=%v", c, err)
			}
		})
	}
	if c, err := HTTPClientForNode(nil, ""); err == nil || c != nil {
		t.Fatal("nil node must be rejected")
	}
}
