package controller

import (
	"net/http"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// FirewallController exposes managed host-firewall controls. The service keeps
// its rules isolated from administrator-owned UFW/firewalld rules and updates
// the allowed inbound ports automatically.
type FirewallController struct {
	firewallService service.FirewallService
}

func NewFirewallController(g *gin.RouterGroup) *FirewallController {
	a := &FirewallController{}
	a.initRouter(g)
	service.StartFirewallReconciler()
	return a
}

func (a *FirewallController) initRouter(g *gin.RouterGroup) {
	g.GET("/status", a.status)
	g.POST("/enable", a.setEnabled)
	g.POST("/autoSync", a.setAutoSync)
	g.POST("/sync", a.sync)
	g.POST("/manual/add", a.addManualRule)
	g.POST("/manual/delete", a.deleteManualRule)
}

func (a *FirewallController) status(c *gin.Context) {
	status, err := a.firewallService.Status()
	jsonObj(c, status, err)
}

func (a *FirewallController) setEnabled(c *gin.Context) {
	var form struct {
		Enabled bool `json:"enabled" form:"enabled"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "invalid firewall request", err)
		return
	}
	if err := a.firewallService.SetEnabled(form.Enabled); err != nil {
		jsonMsg(c, "failed to change firewall state", err)
		return
	}
	status, err := a.firewallService.Status()
	jsonObj(c, status, err)
}

func (a *FirewallController) setAutoSync(c *gin.Context) {
	var form struct {
		AutoSync bool `json:"autoSync" form:"autoSync"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "invalid firewall request", err)
		return
	}
	if err := a.firewallService.SetAutoSync(form.AutoSync); err != nil {
		jsonMsg(c, "failed to update firewall automation", err)
		return
	}
	status, err := a.firewallService.Status()
	jsonObj(c, status, err)
}

func (a *FirewallController) sync(c *gin.Context) {
	if err := a.firewallService.Sync(); err != nil {
		jsonMsg(c, "failed to synchronize firewall", err)
		return
	}
	status, err := a.firewallService.Status()
	jsonObj(c, status, err)
}

func (a *FirewallController) addManualRule(c *gin.Context) {
	var rule service.FirewallPortRule
	if err := c.ShouldBind(&rule); err != nil {
		jsonMsg(c, "invalid firewall rule", err)
		return
	}
	if err := a.firewallService.AddManualRule(rule); err != nil {
		jsonMsg(c, "failed to add firewall rule", err)
		return
	}
	status, err := a.firewallService.Status()
	jsonObj(c, status, err)
}

func (a *FirewallController) deleteManualRule(c *gin.Context) {
	var form struct {
		Port     int    `json:"port" form:"port"`
		Protocol string `json:"protocol" form:"protocol"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonMsg(c, "invalid firewall rule", err)
		return
	}
	if err := a.firewallService.DeleteManualRule(form.Port, form.Protocol); err != nil {
		jsonMsg(c, "failed to delete firewall rule", err)
		return
	}
	status, err := a.firewallService.Status()
	jsonObj(c, status, err)
}

// firewallAutoSyncMiddleware turns inbound mutations into a coalesced firewall
// reconciliation. It runs after the handler, so the worker always reads the
// committed database state. The periodic worker is a second safety net for DB
// imports and any future mutation path that does not pass this middleware.
func firewallAutoSyncMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Request.Method != http.MethodPost || c.Writer.Status() >= http.StatusBadRequest {
			return
		}
		path := c.FullPath()
		if strings.Contains(path, "/panel/api/inbounds/") {
			service.TriggerFirewallSync()
		}
	}
}
