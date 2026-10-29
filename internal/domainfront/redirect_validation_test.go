package domainfront

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedirectTargetValidation(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "//example.com/", "https://name:password@example.com/", "https://example.com:0/", "https://example.com:65536/", "https://example.com:/", "https://example.com/\n", "https://example.com/" + strings.Repeat("a", 2048)} {
		if _, err := ValidateRootRedirectURL(raw); err == nil {
			t.Errorf("accepted invalid target %q", raw)
		}
	}
	for _, raw := range []string{"https://example.com/", "http://example.com:8080/page?fixed=1#section"} {
		if _, err := ValidateRootRedirectURL(raw); err != nil {
			t.Fatal(raw, err)
		}
	}
	if err := ValidateRootRedirectDomains("https://PANEL.example./", []string{"panel.example"}); err == nil {
		t.Fatal("managed root allowed")
	}
	if err := ValidateRootRedirectDomains("https://panel.example/login", []string{"panel.example"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRootRedirectDomains("https://panel.example/?panel=1", []string{"panel.example"}); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPRootRedirectAvoidsHTTPSHopAndDropsQuery(t *testing.T) {
	r := &frontRuntime{cfg: DomainFrontConfig{PublicPort: 443, Domains: []DomainFrontDomain{{Name: "panel.example", Enabled: true}}}, backend: Backend{RedirectURL: "https://target.example/welcome?fixed=1"}}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(method, "http://panel.example/?secret=do-not-forward", nil)
		r.redirectHTTPS(response, req)
		if response.Code != http.StatusFound || response.Header().Get("Location") != "https://target.example/welcome?fixed=1" {
			t.Fatal(response.Code, response.Header())
		}
		if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("redirect lacks privacy headers")
		}
	}
	for _, path := range []string{"/sub/token", "/.well-known/acme-challenge/token", "/?panel=1"} {
		response := httptest.NewRecorder()
		r.redirectHTTPS(response, httptest.NewRequest(http.MethodGet, "http://panel.example"+path, nil))
		if response.Code != http.StatusPermanentRedirect || !strings.HasPrefix(response.Header().Get("Location"), "https://panel.example/") {
			t.Fatal("panel/subscription route was captured", path, response.Header())
		}
	}
	response := httptest.NewRecorder()
	r.redirectHTTPS(response, httptest.NewRequest(http.MethodGet, "http://unknown.example/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatal("unknown host was redirected")
	}
}
