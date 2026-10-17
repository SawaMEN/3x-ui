package controller

import (
	"context"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/oxidns"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/gin-gonic/gin"
)

type OxiDNSController struct{ settings service.SettingService }

func NewOxiDNSController(g *gin.RouterGroup) *OxiDNSController {
	c := &OxiDNSController{}
	g.GET("/status", c.status)
	g.POST("/install", c.install)
	g.POST("/update", c.update)
	g.POST("/remove", c.remove)
	g.POST("/setup", c.setup)
	g.POST("/disable", c.disable)
	return c
}

func (a *OxiDNSController) status(c *gin.Context) {
	timeout := 5 * time.Second
	if c.Query("check") == "true" {
		timeout = 25 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	var s oxidns.Status
	var err error
	if c.Query("check") == "true" {
		s, err = oxidns.StatusWithUpdates(ctx)
	} else {
		s, err = oxidns.CurrentStatus(ctx)
	}
	if err != nil {
		jsonMsg(c, "Не удалось получить состояние DNS", err)
		return
	}
	if !s.Enabled && s.Domain == "" {
		domain, _ := a.settings.GetWebDomain()
		s.Domain = strings.TrimSpace(domain)
	}
	jsonObj(c, s, nil)
}

func (a *OxiDNSController) restartNative(ctx context.Context) error {
	coreType, err := a.settings.GetCoreType()
	if err != nil {
		return err
	}
	if !service.IsNativeCore(coreType) || !service.SelectedNativeCore().IsRunning() {
		return nil
	}
	return service.SelectedNativeCore().Restart(ctx)
}

func (a *OxiDNSController) setup(c *gin.Context) {
	var cfg oxidns.Settings
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonMsg(c, "Неверные настройки DNS", err)
		return
	}
	if cfg.Port == 0 {
		cfg.Port = 8443
	}
	if cfg.Upstream == "" {
		cfg.Upstream = "quad9"
	}
	if cfg.Domain == "" {
		domain, _ := a.settings.GetWebDomain()
		cfg.Domain = strings.TrimSpace(domain)
	}
	cert, _ := a.settings.GetCertFile()
	key, _ := a.settings.GetKeyFile()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()
	if err := oxidns.Setup(ctx, cfg, cert, key); err != nil {
		jsonMsg(c, "Не удалось включить OxiDNS", err)
		return
	}
	if err := a.restartNative(ctx); err != nil {
		jsonMsg(c, "OxiDNS включён, но sing-box не применил DNS. Проверьте ядро и повторите запуск.", err)
		return
	}
	s, err := oxidns.CurrentStatus(ctx)
	jsonObj(c, s, err)
}

func (a *OxiDNSController) disable(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	if err := oxidns.Disable(ctx); err != nil {
		jsonMsg(c, "Не удалось отключить OxiDNS", err)
		return
	}
	if err := a.restartNative(ctx); err != nil {
		jsonMsg(c, "OxiDNS отключён, но sing-box не обновил DNS", err)
		return
	}
	s, err := oxidns.CurrentStatus(ctx)
	jsonObj(c, s, err)
}

func (a *OxiDNSController) install(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()
	if err := oxidns.InstallOnly(ctx); err != nil {
		jsonMsg(c, "Не удалось установить OxiDNS", err)
		return
	}
	s, err := oxidns.CurrentStatus(ctx)
	jsonObj(c, s, err)
}

func (a *OxiDNSController) update(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()
	if err := oxidns.Upgrade(ctx); err != nil {
		jsonMsg(c, "Не удалось обновить OxiDNS", err)
		return
	}
	s, err := oxidns.CurrentStatus(ctx)
	jsonObj(c, s, err)
}

func (a *OxiDNSController) remove(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	if err := oxidns.Uninstall(ctx); err != nil {
		jsonMsg(c, "Не удалось удалить OxiDNS", err)
		return
	}
	if err := a.restartNative(ctx); err != nil {
		jsonMsg(c, "OxiDNS удалён, но sing-box не обновил DNS", err)
		return
	}
	s, err := oxidns.CurrentStatus(ctx)
	jsonObj(c, s, err)
}
