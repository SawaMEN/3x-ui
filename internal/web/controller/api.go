package controller

import (
	"net/http"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/middleware"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service/panel"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service/tgbot"
	"github.com/SawaMEN/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

// APIController handles the main API routes for the 3x-ui panel, including inbounds and server management.
type APIController struct {
	BaseController
	inboundController     *InboundController
	serverController      *ServerController
	nodeController        *NodeController
	hostController        *HostController
	settingController     *SettingController
	xraySettingController *XraySettingController
	userService           panel.UserService
	apiTokenService       panel.ApiTokenService
	settingService        service.SettingService
	Tgbot                 tgbot.Tgbot
}

// NewAPIController creates a new APIController instance and initializes its routes.
func NewAPIController(g *gin.RouterGroup, settingService service.SettingService) *APIController {
	a := &APIController{settingService: settingService}
	a.initRouter(g)
	return a
}

func (a *APIController) checkAPIAuth(c *gin.Context) {
	if c.Request.TLS != nil && len(c.Request.TLS.VerifiedChains) > 0 {
		if u, err := a.userService.GetFirstUser(); err == nil {
			session.SetAPIAuthUser(c, u)
		}
		c.Set("api_authed", true)
		c.Set("api_token_scope", model.ApiScopeNodeSync)
		c.Next()
		return
	}
	auth := c.GetHeader("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		if row, ok := a.apiTokenService.MatchToken(after); ok {
			if u, err := a.userService.GetFirstUser(); err == nil {
				session.SetAPIAuthUser(c, u)
			}
			c.Set("api_authed", true)
			c.Set("api_token_scope", row.Scope)
			c.Next()
			return
		}
	}
	if !session.IsLogin(c) {
		if strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") || c.GetHeader("X-Requested-With") == "XMLHttpRequest" {
			c.AbortWithStatus(http.StatusUnauthorized)
		} else {
			c.AbortWithStatus(http.StatusNotFound)
		}
		return
	}
	c.Next()
}

// monitorScopeAllow exposes only status/metrics routes without sensitive data.
var monitorScopeAllow = map[string]struct{}{
	"/server/status":                              {},
	"/server/cpuHistory/:bucket":                  {},
	"/server/history/:metric/:bucket":             {},
	"/server/trafficHistory/:resource/:bucket":    {},
	"/server/xrayMetricsState":                    {},
	"/server/xrayMetricsHistory/:metric/:bucket":  {},
	"/server/xrayObservatory":                     {},
	"/server/xrayObservatoryHistory/:tag/:bucket": {},
	"/server/getXrayVersion":                      {},
	"/server/getPanelUpdateInfo":                  {},
	"/nodes/history/:id/:metric/:bucket":          {},
}

var nodeSyncScopeAllow = map[string]map[string]struct{}{
	"/server/status":               {http.MethodGet: {}},
	"/inbounds/list":               {http.MethodGet: {}},
	"/inbounds/add":                {http.MethodPost: {}},
	"/inbounds/del/:id":            {http.MethodPost: {}},
	"/inbounds/update/:id":         {http.MethodPost: {}},
	"/inbounds/:id/subSortIndex":   {http.MethodPost: {}},
	"/clients/add":                 {http.MethodPost: {}},
	"/clients/del/:email":          {http.MethodPost: {}},
	"/clients/:email/detach":       {http.MethodPost: {}},
	"/clients/update/:email":       {http.MethodPost: {}},
	"/server/restartXrayService":   {http.MethodPost: {}},
	"/server/restartCoreService":   {http.MethodPost: {}},
	"/server/updatePanel":          {http.MethodPost: {}},
	"/server/getWebCertFiles":      {http.MethodGet: {}},
	"/server/descendants":          {http.MethodGet: {}},
	"/clients/resetTraffic/:email": {http.MethodPost: {}},
	"/inbounds/resetAllTraffics":   {http.MethodPost: {}},
	"/inbounds/:id/resetTraffic":   {http.MethodPost: {}},
	"/clients/onlinesByGuid":       {http.MethodPost: {}},
	"/clients/onlines":             {http.MethodPost: {}},
	"/clients/activeInbounds":      {http.MethodPost: {}},
	"/clients/lastOnline":          {http.MethodPost: {}},
	"/inbounds/pushClientTraffics": {http.MethodPost: {}},
	"/server/clientIps":            {http.MethodGet: {}, http.MethodPost: {}},
	"/clients/clientIpsByGuid":     {http.MethodPost: {}},
	"/hosts/list":                  {http.MethodGet: {}},
}

func (a *APIController) enforceTokenScope(c *gin.Context) {
	scopeVal, ok := c.Get("api_token_scope")
	if !ok {
		c.Next()
		return
	}
	scope, _ := scopeVal.(string)
	if scope == model.ApiScopeAdmin {
		c.Next()
		return
	}
	deny := func() {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"msg":     "this API token is not permitted to access this endpoint",
		})
	}
	rel := relAPIPath(c.FullPath())
	switch scope {
	case model.ApiScopeMonitor:
		if _, allowed := monitorScopeAllow[rel]; allowed && (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead) {
			c.Next()
			return
		}
	case model.ApiScopeNodeSync:
		if methods, allowed := nodeSyncScopeAllow[rel]; allowed {
			if _, allowedMethod := methods[c.Request.Method]; allowedMethod {
				c.Next()
				return
			}
		}
	default:
		deny()
		return
	}
	deny()
}

func relAPIPath(fullPath string) string {
	const marker = "/panel/api"
	_, after, ok := strings.Cut(fullPath, marker)
	if !ok {
		return ""
	}
	return after
}

func (a *APIController) initRouter(g *gin.RouterGroup) {
	api := g.Group("/panel/api")
	api.Use(a.checkAPIAuth)
	api.Use(a.enforceTokenScope)
	api.Use(middleware.ConfigEnvelopeMiddleware())
	api.Use(middleware.CSRFMiddleware())
	api.Use(middleware.AuditMutationMiddleware())
	api.Use(firewallAutoSyncMiddleware())

	api.GET("/openapi.json", ServeOpenAPISpec)

	inbounds := api.Group("/inbounds")
	a.inboundController = NewInboundController(inbounds)

	clients := api.Group("/clients")
	NewClientController(clients)
	NewGroupController(clients)

	server := api.Group("/server")
	a.serverController = NewServerController(server)
	NewTrafficHistoryController(server)
	NewFirewallController(server)

	nodes := api.Group("/nodes")
	a.nodeController = NewNodeController(nodes)

	hosts := api.Group("/hosts")
	a.hostController = NewHostController(hosts)

	telemt := api.Group("/telemt")
	NewTelemtController(telemt, a.settingService)

	NewNaiveProxyController(api)

	routingPresets := api.Group("/xray/routingPresets")
	NewRoutingPresetController(routingPresets)

	a.settingController = NewSettingController(api)
	a.xraySettingController = NewXraySettingController(api)
	NewGatewayController(api)
	NewAdBlockController(api)
	NewSubBalancerController(api)
	api.POST("/backuptotgbot", a.BackuptoTgbot)
}

func (a *APIController) BackuptoTgbot(c *gin.Context) {
	a.Tgbot.SendBackupToAdmins()
}
