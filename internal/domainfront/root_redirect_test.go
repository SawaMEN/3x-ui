package domainfront

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRootRedirectAcrossDomainPortsAndPaths(t *testing.T) {
	for _, tc := range []struct {
		request, target string
		secure, want    bool
	}{
		{"http://panel.example:2053/", "https://public.example/", false, true},
		{"http://panel.example/", "https://public.example/", false, true},
		{"https://panel.example/", "https://public.example/", true, true},
		{"https://panel.example/", "https://panel.example/site", true, true},
		{"https://panel.example/", "https://panel.example/", true, false},
		{"https://panel.example/sub/token", "https://public.example/", true, false},
		{"https://panel.example/?panel=1", "https://public.example/", true, false},
	} {
		t.Run(tc.request+tc.target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.request, nil)
			if tc.secure {
				req.TLS = &tls.ConnectionState{}
			}
			got := RootRedirectTarget(req, tc.target)
			if (got != "") != tc.want {
				t.Fatalf("target=%q want redirect=%v", got, tc.want)
			}
		})
	}
}
