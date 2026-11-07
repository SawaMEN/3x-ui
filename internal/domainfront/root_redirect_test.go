package domainfront

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRootRedirectPreservesPathQueryAndPort(t *testing.T) {
	tests := []struct {
		name, method, request, destination, want string
		secure                                   bool
	}{
		{"root", "GET", "http://panel.example/", "https://public.example/", "https://public.example/", false},
		{"root private query", "GET", "http://panel.example/?secret=private", "https://public.example/welcome?fixed=1", "https://public.example/welcome?fixed=1", false},
		{"ordinary path", "GET", "https://panel.example/123", "https://public.example/", "https://public.example/123", true},
		{"nested path and query", "GET", "http://panel.example/a/b?x=1", "https://public.example/welcome?fixed=1", "https://public.example/welcome/a/b?fixed=1&x=1", false},
		{"escaped path", "GET", "https://panel.example/a%2Fb/%20c?q=%2F", "https://public.example/", "https://public.example/a%2Fb/%20c?q=%2F", true},
		{"alternate HTTPS port", "GET", "https://panel.example:2345/123", "https://public.example/", "https://public.example:2345/123", true},
		{"alternate HTTP port", "GET", "http://panel.example:2345/123", "https://public.example/", "https://public.example:2345/123", false},
		{"destination explicit port", "GET", "http://panel.example:2345/123", "https://public.example:8443/", "https://public.example:8443/123", false},
		{"destination path at alternate port", "GET", "http://panel.example:2345/abc", "https://public.example/site", "https://public.example:2345/site/abc", false},
		{"default HTTP port", "GET", "http://panel.example:80/123", "https://public.example/", "https://public.example/123", false},
		{"root HEAD", "HEAD", "https://panel.example/", "https://public.example/", "https://public.example/", true},
		{"same origin alternate path", "GET", "https://panel.example/", "https://panel.example/site", "https://panel.example/site", true},
		{"same origin nested path", "GET", "https://panel.example/123", "https://panel.example/site", "https://panel.example/site/123", true},
		{"same origin path loop", "GET", "https://panel.example/site/123", "https://panel.example/site", "", true},
		{"same origin root loop", "GET", "https://panel.example/123", "https://panel.example/", "", true},
		{"same origin root", "GET", "https://panel.example/", "https://panel.example/", "", true},
		{"panel bypass", "GET", "https://panel.example/?panel=1", "https://public.example/", "", true},
		{"panel bypass on path", "GET", "https://panel.example/123?panel=1", "https://public.example/", "", true},
		{"ACME", "GET", "http://panel.example/.well-known/acme-challenge/token", "https://public.example/", "", false},
		{"POST", "POST", "https://panel.example/123", "https://public.example/", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.request, nil)
			if tc.secure {
				req.TLS = &tls.ConnectionState{}
			}
			if got := RootRedirectTarget(req, tc.destination); got != tc.want {
				t.Fatalf("redirect = %q, want %q", got, tc.want)
			}
		})
	}
}
