package sub

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func subscriptionFormat(c *gin.Context) string {
	format := strings.ToLower(c.Query("format"))
	if format == "" {
		format = c.GetString("subscriptionFormat")
	}
	if format == "singbox" {
		return "sing-box"
	}
	return format
}

func (a *SUBController) formatAllowed(c *gin.Context, format string) bool {
	mode := a.formatMode(c)
	return mode == "" || mode == "both" || mode == format
}

func (a *SUBController) defaultBodyFormat(c *gin.Context) string {
	mode := a.formatMode(c)
	if mode == "xray" || mode == "sing-box" {
		return mode
	}
	if value, ok := c.Get("subscriptionDefaultFormat"); ok {
		return value.(string)
	}
	return a.subDefaultFormat
}

func appendSubscriptionFormat(raw, format string) string {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return ""
	}
	q := u.Query()
	q.Set("format", format)
	u.RawQuery = q.Encode()
	return u.String()
}

func WithSUBLiveFormats() SUBControllerOption {
	return func(config *subControllerConfig) { config.liveFormats = true }
}

func (a *SUBController) formatMode(c *gin.Context) string {
	if value, ok := c.Get("subscriptionOutputMode"); ok {
		return value.(string)
	}
	return a.subFormatMode
}

func (a *SUBController) loadSubscriptionFormats(c *gin.Context) {
	if a.liveFormats {
		mode, format, err := (&service.SettingService{}).GetSubOutputFormats()
		if err != nil {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		c.Set("subscriptionOutputMode", mode)
		c.Set("subscriptionDefaultFormat", format)
	}
	c.Next()
}
