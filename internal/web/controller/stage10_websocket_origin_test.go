package controller

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStage10WebSocketOriginIsPortAndSchemeScoped(t *testing.T) {
	for _, tc := range []struct {
		name, host, origin string
		tls, allow         bool
	}{
		{"no Origin for native clients", "panel.example:8443", "", true, true},
		{"same HTTPS origin", "panel.example:8443", "https://panel.example:8443", true, true},
		{"case insensitive hostname", "Panel.Example:8443", "https://panel.example:8443", true, true},
		{"cross-port request", "panel.example:8443", "https://panel.example:9443", true, false},
		{"cross-scheme TLS request", "panel.example:8443", "http://panel.example:8443", true, false},
		{"cross-host", "panel.example:8443", "https://evil.example:8443", true, false},
		{"cross-host suffix", "panel.example:8443", "https://panel.example.evil:8443", true, false},
		{"custom Origin port with default Host", "panel.example", "https://panel.example:8443", true, false},
		{"implicit HTTPS port", "panel.example", "https://panel.example", true, true},
		{"explicit HTTPS default port", "panel.example", "https://panel.example:443", true, true},
		{"non-TLS proxy terminating HTTPS", "panel.example:8443", "https://panel.example:8443", false, true},
		{"non-TLS direct HTTP", "panel.example:8080", "http://panel.example:8080", false, true},
		{"IPv6 address and port", "[::1]:8443", "https://[::1]:8443", true, true},
		{"IPv6 cross-port", "[::1]:8443", "https://[::1]:9999", true, false},
		{"malformed origin", "panel.example", "not a URL", false, false},
		{"origin with credentials", "panel.example", "https://user:pass@panel.example", true, false},
		{"origin with path", "panel.example", "https://panel.example/another", true, false},
		{"origin with query", "panel.example", "https://panel.example/?x=1", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://panel.example/ws", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if got := checkSameOrigin(req); got != tc.allow {
				t.Fatalf("checkSameOrigin(host=%q, origin=%q, tls=%v) = %v, want %v", tc.host, tc.origin, tc.tls, got, tc.allow)
			}
		})
	}
}
