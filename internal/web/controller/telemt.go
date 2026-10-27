package controller

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type TelemtController struct {
	service        service.TelemtService
	settingService service.SettingService
}

func NewTelemtController(g *gin.RouterGroup, settingService service.SettingService) *TelemtController {
	a := &TelemtController{settingService: settingService}
	g.GET("/status", a.status)
	g.GET("/subscription/:subid", a.subscriptionProfile)
	g.POST("/install", a.install)
	// Only discovery and installation are available before the binary exists.
	g.Use(a.requireInstalled)
	g.POST("/remove", a.remove)
	g.GET("/webproxy/status", a.webProxyStatus)
	g.POST("/webproxy/preflight", a.webProxyPreflight)
	g.POST("/webproxy/enable", a.enableWebProxy)
	g.POST("/webproxy/disable", a.disableWebProxy)
	g.GET("/config", a.config)
	g.POST("/config", a.saveConfig)
	g.GET("/meko/config", a.mekoConfig)
	g.POST("/meko/config", a.saveMekoConfig)
	g.POST("/meko/reconcile", a.reconcileMeko)
	g.GET("/subscription-proxy", a.subscriptionProxySetting)
	g.POST("/subscription-proxy", a.saveSubscriptionProxySetting)
	g.GET("/proxy", a.listProxy)
	g.POST("/proxy", a.createProxy)
	g.DELETE("/proxy/:name", a.deleteProxy)
	g.GET("/connections", a.connections)
	g.POST("/action", a.action)
	return a
}

func (a *TelemtController) requireInstalled(c *gin.Context) {
	if !a.service.IsInstalled() {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{
			"success": false,
			"msg":     "Telemt не установлен. Установите компонент в настройках системы.",
		})
		return
	}
	c.Next()
}

func (a *TelemtController) install(c *gin.Context) {
	if err := a.service.Apply("install"); err != nil {
		jsonMsg(c, "failed to install Telemt", err)
		return
	}
	jsonObj(c, a.service.Status(), nil)
}

func (a *TelemtController) remove(c *gin.Context) {
	if err := a.service.Apply("remove"); err != nil {
		jsonMsg(c, "failed to remove Telemt", err)
		return
	}
	jsonObj(c, a.service.Status(), nil)
}

func (a *TelemtController) status(c *gin.Context) {
	status := a.service.Status()
	status.WebProxy = a.getWebProxyStatus(c)
	jsonObj(c, status, nil)
}

func (a *TelemtController) getWebProxyStatus(c *gin.Context) service.TelemtWebProxyStatus {
	defaultDomain, _ := a.settingService.GetWebDomain()
	if strings.TrimSpace(defaultDomain) == "" {
		defaultDomain = publicHostFromRequest(c)
	}
	certFile, _ := a.settingService.GetCertFile()
	keyFile, _ := a.settingService.GetKeyFile()
	webStatus, err := a.service.WebProxyStatus(defaultDomain, certFile, keyFile)
	if err != nil {
		webStatus.Error = err.Error()
	}
	appendWebProxyError := func(message string) {
		if webStatus.Error == "" {
			webStatus.Error = message
		} else {
			webStatus.Error += "; " + message
		}
	}
	if webStatus.Enabled && !a.service.WebProxyTelemtActive() {
		appendWebProxyError("Telemt остановлен: WEB Proxy настроен, но локальный backend не работает")
	}
	if webStatus.Enabled && !webStatus.FrontendActive {
		appendWebProxyError(webStatus.Frontend + " остановлен: HTTPS endpoint WEB Proxy недоступен")
	}
	if webStatus.Enabled && !webStatus.CertificateReady {
		appendWebProxyError("TLS-сертификат WEB Proxy отсутствует, просрочен или не подходит домену")
	}
	return webStatus
}

func (a *TelemtController) webProxyStatus(c *gin.Context) {
	jsonObj(c, a.getWebProxyStatus(c), nil)
}

func (a *TelemtController) webProxyPreflight(c *gin.Context) {
	var req struct {
		Domain string `json:"domain"`
		service.TelemtWebProxyOptions
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, "invalid WEB Proxy preflight request", err)
		return
	}
	certFile, _ := a.settingService.GetCertFile()
	keyFile, _ := a.settingService.GetKeyFile()
	result, err := a.service.PreflightWebProxy(c.Request.Context(), req.Domain, req.TelemtWebProxyOptions, certFile, keyFile)
	jsonObj(c, result, err)
}

func (a *TelemtController) enableWebProxy(c *gin.Context) {
	var req struct {
		Domain string `json:"domain"`
		service.TelemtWebProxyOptions
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, "invalid WEB Proxy request", err)
		return
	}
	if strings.TrimSpace(req.Domain) == "" {
		req.Domain = a.getWebProxyStatus(c).DefaultDomain
	}
	certFile, _ := a.settingService.GetCertFile()
	keyFile, _ := a.settingService.GetKeyFile()
	_, err := a.service.EnableWebProxy(c.Request.Context(), req.Domain, certFile, keyFile, req.TelemtWebProxyOptions)
	if err != nil {
		jsonMsg(c, "WEB Proxy setup failed", err)
		return
	}
	status := a.getWebProxyStatus(c)
	status.DefaultDomain = req.Domain
	jsonObj(c, status, nil)
}

func (a *TelemtController) disableWebProxy(c *gin.Context) {
	if err := a.service.DisableWebProxy(); err != nil {
		jsonMsg(c, "failed to disable WEB Proxy", err)
		return
	}
	jsonObj(c, a.getWebProxyStatus(c), nil)
}

func (a *TelemtController) config(c *gin.Context) {
	cfg, err := a.service.GetConfig()
	if err != nil {
		jsonMsg(c, "failed to read Telemt config", err)
		return
	}
	jsonObj(c, cfg, nil)
}

func (a *TelemtController) saveConfig(c *gin.Context) {
	var cfg service.TelemtConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonMsg(c, "invalid Telemt configuration", err)
		return
	}
	if err := a.service.SaveConfigAtomic(cfg); err != nil {
		jsonMsg(c, err.Error(), err)
		return
	}
	jsonObj(c, a.service.Status(), nil)
}

func (a *TelemtController) mekoConfig(c *gin.Context) {
	jsonObj(c, a.service.GetMekoConfig(), nil)
}

func (a *TelemtController) saveMekoConfig(c *gin.Context) {
	var cfg service.TelemtMekoConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonMsg(c, "invalid MEKO V3 configuration", err)
		return
	}
	if err := a.service.SaveMekoConfig(cfg); err != nil {
		jsonMsg(c, err.Error(), err)
		return
	}
	jsonObj(c, a.service.GetMekoConfig(), nil)
}

// Force an immediate apply and verify actual firewall state after a user
// changes MEKO settings, without waiting for the periodic MTProto poll.
func (a *TelemtController) reconcileMeko(c *gin.Context) {
	if err := a.service.RefreshMekoFix(); err != nil {
		jsonMsg(c, "MEKO firewall rules could not be applied", err)
		return
	}
	jsonObj(c, a.service.GetMekoConfig(), nil)
}

func (a *TelemtController) subscriptionProxySetting(c *gin.Context) {
	enabled, err := a.settingService.GetTelemtSubscriptionProxyEnable()
	if err != nil {
		jsonMsg(c, "failed to read Telemt subscription proxy setting", err)
		return
	}
	jsonObj(c, gin.H{"enabled": enabled}, nil)
}

func (a *TelemtController) saveSubscriptionProxySetting(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, "invalid Telemt subscription proxy setting", err)
		return
	}

	oldEnabled, err := a.settingService.GetTelemtSubscriptionProxyEnable()
	if err != nil {
		jsonMsg(c, "failed to read current Telemt subscription proxy setting", err)
		return
	}
	host := publicHostFromRequest(c)
	if host == "" {
		defaultDomain, _ := a.settingService.GetWebDomain()
		host = strings.TrimSpace(defaultDomain)
	}

	if err := a.service.ReconcileSubscriptionProxies(req.Enabled, host); err != nil {
		if oldEnabled != req.Enabled {
			if rollbackErr := a.service.ReconcileSubscriptionProxies(oldEnabled, host); rollbackErr != nil {
				err = errors.New(err.Error() + "; rollback failed: " + rollbackErr.Error())
			}
		}
		jsonMsg(c, "failed to synchronize Telemt subscription proxies", err)
		return
	}

	if err := a.settingService.SetTelemtSubscriptionProxyEnable(req.Enabled); err != nil {
		if oldEnabled != req.Enabled {
			if rollbackErr := a.service.ReconcileSubscriptionProxies(oldEnabled, host); rollbackErr != nil {
				err = errors.New(err.Error() + "; proxy rollback failed: " + rollbackErr.Error())
			}
		}
		jsonMsg(c, "failed to save Telemt subscription proxy setting", err)
		return
	}
	jsonObj(c, gin.H{"enabled": req.Enabled}, nil)
}

func (a *TelemtController) listProxy(c *gin.Context) {
	proxies, err := a.service.ListProxies()
	if err != nil {
		jsonMsg(c, "failed to read Telemt proxies", err)
		return
	}
	host := publicHostFromRequest(c)
	if host != "" {
		for i := range proxies {
			if u, err := url.Parse(proxies[i].Link); err == nil {
				q := u.Query()
				q.Set("server", host)
				u.RawQuery = q.Encode()
				proxies[i].Link = u.String()
				proxies[i].Host = host
			}
		}
	}
	jsonObj(c, proxies, nil)
}

func (a *TelemtController) connections(c *gin.Context) {
	connections, err := a.service.ConnectedClients()
	if err != nil {
		jsonMsg(c, "failed to read Telemt connections", err)
		return
	}
	jsonObj(c, connections, nil)
}

func publicHostFromRequest(c *gin.Context) string {
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if i := strings.IndexByte(host, ','); i >= 0 {
		host = strings.TrimSpace(host[:i])
	}
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return strings.Trim(h, "[]")
	}
	return strings.Trim(host, "[]")
}

func (a *TelemtController) createProxy(c *gin.Context) {
	var req service.TelemtCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, "invalid Telemt proxy request", err)
		return
	}
	req.Host = publicHostFromRequest(c)
	if req.Host == "" {
		jsonMsg(c, "unable to determine public host", nil)
		return
	}
	proxy, err := a.service.CreateProxyAtomic(req)
	if err != nil {
		jsonMsg(c, err.Error(), err)
		return
	}
	jsonObj(c, proxy, nil)
}

func (a *TelemtController) deleteProxy(c *gin.Context) {
	if err := a.service.DeleteProxy(c.Param("name")); err != nil {
		jsonMsg(c, err.Error(), err)
		return
	}
	jsonObj(c, a.service.Status(), nil)
}

func (a *TelemtController) action(c *gin.Context) {
	var req struct {
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, "invalid Telemt action", err)
		return
	}

	// Keep the legacy action endpoint compatible, but route MEKO mutations
	// through the same persisted/validated configuration path as the modern UI.
	// The old Apply implementation downloaded a separate script and could leave
	// the service state out of sync with /meko/config.
	if req.Action == "meko-enable" || req.Action == "meko-disable" {
		cfg := a.service.GetMekoConfig()
		cfg.Enabled = req.Action == "meko-enable"
		if err := a.service.SaveMekoConfig(cfg); err != nil {
			jsonMsg(c, err.Error(), err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "obj": a.service.Status()})
		return
	}

	if err := a.service.Apply(req.Action); err != nil {
		jsonMsg(c, err.Error(), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "obj": a.service.Status()})
}

func (a *TelemtController) subscriptionProfile(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	profile, err := a.service.SubscriptionProxyView(ctx, c.Param("subid"), publicHostFromRequest(c))
	jsonObj(c, profile, err)
}
