package session

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// CurrentSessionID returns the opaque session identifier stored in the current
// authenticated cookie. Callers use it only to identify the active session;
// persistence continues to store a SHA-256 hash rather than the raw value.
func CurrentSessionID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	raw, _ := sessions.Default(c).Get(loginSessionKey).(string)
	return raw
}
