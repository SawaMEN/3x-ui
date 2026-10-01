package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"

	"github.com/gin-gonic/gin"
)

type auditMutation struct {
	Time   int64  `json:"time"`
	Actor  string `json:"actor"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	IP     string `json:"ip"`
}

var auditNoiseRoutes = map[string]struct{}{
	"/panel/api/inbounds/pushClientTraffics": {},
	"/panel/api/clients/onlines":             {},
	"/panel/api/clients/onlinesByGuid":       {},
	"/panel/api/clients/activeInbounds":      {},
	"/panel/api/clients/lastOnline":          {},
	"/panel/api/clients/clientIpsByGuid":     {},
}

// AuditMutationMiddleware writes authenticated API mutations to the panel log.
// It records metadata only and never reads request bodies or credentials.
func AuditMutationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isAuditSafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		if isAuditNoiseRoute(path) {
			c.Next()
			return
		}

		c.Next()

		actor := "session"
		if scope, ok := c.Get("api_token_scope"); ok {
			if value, ok := scope.(string); ok && value != "" {
				actor = "token:" + value
			}
		}
		event := auditMutation{
			Time:   time.Now().Unix(),
			Actor:  actor,
			Method: c.Request.Method,
			Path:   path,
			Status: c.Writer.Status(),
			IP:     c.ClientIP(),
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			logger.Warning("audit encode failed:", err)
			return
		}
		logger.Info("AUDIT ", string(encoded))
	}
}

func isAuditSafeMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func isAuditNoiseRoute(path string) bool {
	if _, ok := auditNoiseRoutes[path]; ok {
		return true
	}
	return strings.HasPrefix(path, "/panel/api/server/clientIps")
}
