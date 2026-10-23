package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// RootRedirect leaves subscription paths and panel routes untouched.
func RootRedirect(target func() (string, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path != "/" || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) || c.Query("panel") == "1" {
			c.Next()
			return
		}
		raw, err := target()
		if err == nil && raw != "" {
			parsed, parseErr := url.Parse(raw)
			if parseErr == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && !strings.EqualFold(parsed.Host, c.Request.Host) {
				c.Header("Cache-Control", "no-store")
				c.Header("Referrer-Policy", "no-referrer")
				c.Redirect(http.StatusFound, parsed.String())
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
