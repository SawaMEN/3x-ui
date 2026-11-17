package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

func TestStage9NodeSOCKSProxyDoesNotBypassPrivateDestinationGuard(t *testing.T) {
	n := &model.Node{Id: 9901, Scheme: "https", Address: "127.0.0.1", Port: 443, TlsVerifyMode: "verify", AllowPrivateAddress: false}
	for _, proxyURL := range []string{"socks5://127.0.0.1:1", "SOCKS5://127.0.0.1:1", "socks5h://127.0.0.1:1"} {
		t.Run(proxyURL, func(t *testing.T) {
			client, err := HTTPClientForNode(n, proxyURL)
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequestWithContext(netsafe.ContextWithAllowPrivate(context.Background(), false), http.MethodGet, "https://127.0.0.1:443/", nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Do(req)
			if !errors.Is(err, netsafe.ErrPrivateAddressBlocked) {
				t.Fatalf("SOCKS proxy bypassed the private-destination guard: %v", err)
			}
		})
	}
}

func TestStage9NodeEgressHTTPProxyRequiresExplicitPrivateOptIn(t *testing.T) {
	n := &model.Node{Id: 9902, Scheme: "https", Address: "example.com", Port: 443, TlsVerifyMode: "verify", AllowPrivateAddress: false}
	for _, proxy := range []string{"http://127.0.0.1:8888", "https://127.0.0.1:8888"} {
		if c, err := HTTPClientForNode(n, proxy); err == nil || c != nil {
			t.Fatalf("unrestricted CONNECT proxy unexpectedly accepted: client=%v err=%v", c, err)
		}
	}
	n.AllowPrivateAddress = true
	if _, err := HTTPClientForNode(n, "http://127.0.0.1:8888"); err != nil {
		t.Fatalf("explicitly authorized private destination must allow configured CONNECT proxy: %v", err)
	}
}

func TestStage9ProxyClientIdentityTracksPrivateOptIn(t *testing.T) {
	n := &model.Node{Id: 9903, Scheme: "https", Address: "127.0.0.1", Port: 443, TlsVerifyMode: "verify", AllowPrivateAddress: false}
	a, err := HTTPClientForNode(n, "socks5://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	n.AllowPrivateAddress = true
	b, err := HTTPClientForNode(n, "socks5://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("node client reused the previous private-address trust policy")
	}
	if b.Transport == nil || b.CheckRedirect == nil {
		t.Fatal("recreated client lost its secure node transport")
	}
	// All clients must refuse URL redirects regardless of proxy settings.
	next := &http.Request{URL: &url.URL{Scheme: "https", Host: "malicious.invalid"}}
	if err := b.CheckRedirect(next, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("proxy client unexpectedly follows redirect: %v", err)
	}
}
