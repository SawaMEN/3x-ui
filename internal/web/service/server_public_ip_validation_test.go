package service

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type publicIPTestTransport struct{}

// Public address discovery in unit tests uses documentation addresses and
// never contacts a provider. Loopback requests still exercise the HTTP code.
func (publicIPTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if ip := net.ParseIP(request.URL.Hostname()); ip != nil && ip.IsLoopback() {
		return http.DefaultTransport.RoundTrip(request)
	}
	address := "203.0.113.7"
	if strings.Contains(request.URL.Hostname(), "6") {
		address = "2001:db8::7"
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(address)), Header: make(http.Header), Request: request}, nil
}

func offlinePublicIPTestClient() *http.Client {
	return &http.Client{Transport: publicIPTestTransport{}}
}

func TestPublicIPRejectsInvalidAndUnboundedResponses(t *testing.T) {
	for _, body := range []string{"", "<html>gateway failure</html>", "127.0.0.1 extra", "127.0.0.1", "10.0.0.1", "169.254.1.2", "fe80::1", "fc00::1", "224.0.0.1", strings.Repeat("x", 1024)} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }))
		if got := getPublicIP(srv.URL); got != "N/A" {
			t.Errorf("invalid public IP accepted: %q", got)
		}
		srv.Close()
	}
}

func TestPublicIPDiscoveryRequiresCorrectAddressFamily(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		address := "203.0.113.7"
		if r.URL.Path == "/v6" {
			address = "2001:db8::7"
		}
		_, _ = io.WriteString(w, address)
	}))
	defer srv.Close()
	if got := firstPublicIP([]string{srv.URL + "/v6", srv.URL}, false); got != "203.0.113.7" {
		t.Fatal(got)
	}
	if got := firstPublicIP([]string{srv.URL, srv.URL + "/v6"}, true); got != "2001:db8::7" {
		t.Fatal(got)
	}
	if got := firstPublicIP([]string{srv.URL}, true); got != "N/A" {
		t.Fatal(got)
	}
}
