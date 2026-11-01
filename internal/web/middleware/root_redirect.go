package middleware

import (
	"net/http"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"

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
			if target := domainfront.RootRedirectTarget(c.Request, raw); target != "" {
				c.Header("Cache-Control", "no-store")
				c.Header("Referrer-Policy", "no-referrer")
				c.Redirect(http.StatusFound, target)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
