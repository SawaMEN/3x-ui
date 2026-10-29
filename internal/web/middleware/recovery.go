package middleware

import (
	"fmt"
	"io"
	"net/http"
	"runtime/debug"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/gin-gonic/gin"
)

// Recovery keeps a failed request from terminating the connection silently.
// Use the route pattern: raw paths, queries, headers and bodies contain panel
// credentials and subscription secrets. Gin's default request dump is disabled.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		_, _ = fmt.Fprintf(logger.ErrorWriter{}, "HTTP handler panic (%s %s): %v\n%s", c.Request.Method, c.FullPath(), recovered, debug.Stack())
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}
