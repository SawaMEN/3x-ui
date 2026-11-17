package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

const requestFailureLogged = "panel.requestFailureLogged"

// RequestRoute excludes concrete parameters, query strings and the private
// panel base path. Non-API endpoints are identified by their code handler.
func RequestRoute(c *gin.Context) string {
	path := c.FullPath()
	if path == "" {
		return "<unmatched>"
	}
	if index := strings.LastIndex(path, "/panel/api/"); index >= 0 {
		return path[index:]
	}
	if name := c.HandlerName(); name != "" {
		if index := strings.LastIndex(name, "/"); index >= 0 {
			name = name[index+1:]
		}
		return name
	}
	return "<registered>"
}

// MarkRequestFailureLogged avoids duplicating an application error or panic
// when the outer HTTP observer also sees its failed response.
func MarkRequestFailureLogged(c *gin.Context) { c.Set(requestFailureLogged, true) }

// RequestFailures records status-only aborts (including authentication and
// authorization denials), Gin errors and client cancellations in production.
// Successful requests do not generate access-log noise.
func RequestFailures() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		if c.GetBool(requestFailureLogged) {
			return
		}
		status := c.Writer.Status()
		reason := ""
		if last := c.Errors.Last(); last != nil {
			reason = logger.ErrorSummary(last.Err)
		}
		if reason == "" && c.Request.Context().Err() != nil {
			reason = logger.ErrorSummary(c.Request.Context().Err())
		}
		if status < http.StatusBadRequest && reason == "" {
			return
		}
		if reason == "" {
			reason = http.StatusText(status)
		}
		logger.Errorf("HTTP request failed method=%s route=%s status=%d duration_ms=%d error=%s", c.Request.Method, RequestRoute(c), status, time.Since(started).Milliseconds(), reason)
	}
}
