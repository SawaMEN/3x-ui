package middleware

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

// SniScanDeadline allows the bounded bulk TLS scan to finish without extending
// the write timeout of other panel requests. Register before response wrappers.
func SniScanDeadline(path string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodPost && c.Request.URL.Path == path {
			if err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(6 * time.Minute)); err != nil && !errors.Is(err, http.ErrNotSupported) {
				logger.Warning("SNI scan: extending response deadline failed:", err)
			}
		}
		c.Next()
	}
}
