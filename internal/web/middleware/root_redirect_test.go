package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRootRedirectPreservesApplicationRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, method, path, target, wantLocation string
		failed                                   bool
		want                                     int
	}{
		{"root", "GET", "/?secret=private", "https://public.example/", "https://public.example/", false, 302},
		{"head", "HEAD", "/", "https://public.example/", "https://public.example/", false, 302},
		{"unknown", "GET", "/123?q=1", "https://public.example/site", "https://public.example/site/123?q=1", false, 302},
		{"login escape", "GET", "/?panel=1", "https://public.example/", "", false, 204},
		{"subscription", "GET", "/sub/token", "https://public.example/", "", false, 204},
		{"panel", "GET", "/panel/", "https://public.example/", "", false, 204},
		{"panel SPA NoRoute", "GET", "/panel/custom/deep", "https://public.example/", "", false, 204},
		{"legacy NoRoute", "GET", "/Legacy/SubID", "https://public.example/", "", false, 204},
		{"login", "POST", "/", "https://public.example/", "", false, 204},
		{"disabled", "GET", "/", "", "", false, 204},
		{"error", "GET", "/", "https://public.example/", "", true, 204},
		{"loop", "GET", "/", "http://example.com/", "", false, 204},
		{"invalid scheme", "GET", "/", "javascript:alert(1)", "", false, 204},
		{"ACME", "GET", "/.well-known/acme-challenge/token", "https://public.example/", "", false, 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(RootRedirect(func() (string, error) {
				if tc.failed {
					return "", errors.New("unavailable")
				}
				return tc.target, nil
			}, func(c *gin.Context) bool {
				p := c.Request.URL.Path
				return strings.HasPrefix(p, "/panel/") || strings.HasPrefix(p, "/Legacy/")
			}))
			router.Any("/", func(c *gin.Context) { c.Status(204) })
			router.GET("/panel/", func(c *gin.Context) { c.Status(204) })
			router.GET("/sub/:id", func(c *gin.Context) { c.Status(204) })
			router.NoRoute(func(c *gin.Context) { c.Status(204) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(tc.method, "http://example.com"+tc.path, nil))
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
			if got := response.Header().Get("Location"); got != tc.wantLocation {
				t.Fatalf("Location = %q, want %q", got, tc.wantLocation)
			}
			if tc.want == http.StatusFound {
				if got := response.Header().Get("Cache-Control"); got != "no-store" {
					t.Fatalf("Cache-Control = %q", got)
				}
			}
		})
	}
}
