package middleware

import (
	"net/http"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"

	"github.com/gin-gonic/gin"
)

// RootRedirect applies only to browser GET/HEAD requests. Registered routes
// always win; the optional guards protect application routes served by NoRoute.
func RootRedirect(target func() (string, error), protected ...func(*gin.Context) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) || c.Query("panel") == "1" {
			c.Next()
			return
		}
		if c.Request.URL.Path != "/" {
			if c.FullPath() != "" {
				c.Next()
				return
			}
			for _, guard := range protected {
				if guard != nil && guard(c) {
					c.Next()
					return
				}
			}
		}

		raw, err := target()
		if err == nil && raw != "" {
			if destination := domainfront.RootRedirectTarget(c.Request, raw); destination != "" {
				c.Header("Cache-Control", "no-store")
				c.Header("Referrer-Policy", "no-referrer")
				c.Redirect(http.StatusFound, destination)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
