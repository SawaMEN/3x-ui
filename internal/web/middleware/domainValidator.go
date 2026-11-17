// Package middleware provides HTTP middleware functions for the 3x-ui web panel,
// including domain validation utilities.
package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/web/global"
)

type legacySubscriptionMatcher interface {
	IsLegacySubscriptionRequest(*http.Request) bool
}

func isLegacySubscriptionRequest(r *http.Request) bool {
	subServer := global.GetSubServer()
	if subServer == nil {
		return false
	}
	matcher, ok := subServer.(legacySubscriptionMatcher)
	return ok && matcher.IsLegacySubscriptionRequest(r)
}

// DomainValidatorMiddleware returns a Gin middleware that validates the request domain.
// It extracts the host from the request, strips any port number, and compares it
// against the configured domain. Requests from unauthorized domains are rejected
// with HTTP 403 Forbidden status. Confirmed legacy Hiddify subscription URLs are
// allowed so migrated :443 links keep working when the subscription port changes.
func DomainValidatorMiddleware(domain string) gin.HandlerFunc {
	normalize := func(host string) string {
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		host = strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
		return host
	}
	expected := normalize(domain)
	return func(c *gin.Context) {
		host := normalize(c.Request.Host)
		if host != expected {
			if isLegacySubscriptionRequest(c.Request) {
				c.Next()
				return
			}
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()
	}
}
