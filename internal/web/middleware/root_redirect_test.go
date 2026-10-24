package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRootRedirectPreservesApplicationRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, target string
		failed                     bool
		want                       int
	}{
		{"root", "GET", "/?secret=private", "https://www.wikipedia.org/", false, 302},
		{"head", "HEAD", "/", "https://www.wikipedia.org/", false, 302},
		{"login escape", "GET", "/?panel=1", "https://www.wikipedia.org/", false, 204},
		{"subscription", "GET", "/sub/token", "https://www.wikipedia.org/", false, 204},
		{"panel", "GET", "/panel/", "https://www.wikipedia.org/", false, 204},
		{"login", "POST", "/", "https://www.wikipedia.org/", false, 204},
		{"disabled", "GET", "/", "", false, 204},
		{"error", "GET", "/", "https://www.wikipedia.org/", true, 204},
		{"loop", "GET", "/", "http://example.com/", false, 204},
		{"invalid scheme", "GET", "/", "javascript:alert(1)", false, 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(RootRedirect(func() (string, error) {
				if tc.failed {
					return "", errors.New("unavailable")
				}
				return tc.target, nil
			}))
			router.Any("/", func(c *gin.Context) { c.Status(204) })
			router.GET("/panel/", func(c *gin.Context) { c.Status(204) })
			router.GET("/sub/:id", func(c *gin.Context) { c.Status(204) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(tc.method, "http://example.com"+tc.path, nil))
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d", response.Code, tc.want)
			}
			if tc.want == http.StatusFound {
				if got := response.Header().Get("Location"); got != tc.target {
					t.Fatalf("Location = %q, want %q", got, tc.target)
				}
				if got := response.Header().Get("Cache-Control"); got != "no-store" {
					t.Fatalf("Cache-Control = %q", got)
				}
			} else if got := response.Header().Get("Location"); got != "" {
				t.Fatalf("unexpected redirect: %q", got)
			}
		})
	}
}
