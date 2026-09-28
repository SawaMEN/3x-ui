package sub

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type telemtSubscriptionPayload struct {
	Personal *service.TelemtProxy `json:"personal,omitempty"`
	WebProxy string               `json:"webProxy,omitempty"`
}

func registerTelemtSubscriptionRoute(g *gin.RouterGroup) {
	g.GET("/telemt/:subid", serveTelemtSubscription)
}

func serveTelemtSubscription(c *gin.Context) {
	subID := strings.TrimSpace(c.Param("subid"))
	if subID == "" || database.GetDB() == nil {
		c.Status(http.StatusNotFound)
		return
	}

	var count int64
	if err := database.GetDB().Table("clients").Where("sub_id = ?", subID).Count(&count).Error; err != nil || count == 0 {
		c.Status(http.StatusNotFound)
		return
	}

	resolver := NewSubService("").ForRequest("")
	_, host, _, _ := resolver.ResolveRequest(c)
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		c.Status(http.StatusServiceUnavailable)
		return
	}

	telemt := service.TelemtService{}
	payload := telemtSubscriptionPayload{}
	if personal, err := telemt.EnsureSubscriptionProxy(subID, host); err == nil {
		payload.Personal = &personal
	} else {
		logger.Debug("sub: Telemt personal profile unavailable:", err)
	}

	settings := service.SettingService{}
	defaultDomain, _ := settings.GetWebDomain()
	if strings.TrimSpace(defaultDomain) == "" {
		defaultDomain = host
	}
	certFile, _ := settings.GetCertFile()
	keyFile, _ := settings.GetKeyFile()
	if web, err := telemt.WebProxyStatus(defaultDomain, certFile, keyFile); err == nil && web.Enabled && strings.TrimSpace(web.Link) != "" {
		payload.WebProxy = web.Link
	}

	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, payload)
}
