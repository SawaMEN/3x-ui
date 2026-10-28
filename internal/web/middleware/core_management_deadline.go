package middleware

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/gin-gonic/gin"
)

// CoreManagementDeadline is registered before gzip so downloads and validated
// switches can finish within their own bounded context rather than the global 30s.
func CoreManagementDeadline(prefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodPost {
			rest, ok := strings.CutPrefix(c.Request.URL.Path, prefix)
			if ok {
				for _, core := range []string{"sing-box", "sing-box-upstream", "xray"} {
					if rest == core+"/install" || rest == core+"/select" {
						if err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(6 * time.Minute)); err != nil && !errors.Is(err, http.ErrNotSupported) {
							logger.Warning("core operation: extending response deadline failed:", err)
						}
						break
					}
				}
			}
		}
		c.Next()
	}
}
