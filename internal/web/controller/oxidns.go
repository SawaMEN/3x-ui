package controller

import (
	"context"
	"errors"
	"os/exec"
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
	g.GET("/logs", c.logs)
	g.POST("/check", c.checkDNS)
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

// logs reads a bounded journal tail of the dedicated OxiDNS systemd unit.
// The endpoint is mounted under the existing authenticated panel API group.
func (a *OxiDNSController) logs(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "journalctl", "-u", "x-ui-oxidns.service",
		"--no-pager", "--output=short-iso", "--lines=80").CombinedOutput()
	if err != nil {
		jsonMsg(c, "Не удалось прочитать журнал службы OxiDNS", err)
		return
	}
	// Avoid sending unbounded or unexpectedly verbose log entries to the browser.
	if len(out) > 65536 {
		out = out[len(out)-65536:]
	}
	jsonObj(c, gin.H{"logs": strings.TrimSpace(string(out))}, nil)
}

// Resolve a public domain via the local OxiDNS listener to test the full path.
func (a *OxiDNSController) checkDNS(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	status, err := oxidns.CurrentStatus(ctx)
	if err != nil {
		jsonMsg(c, "Не удалось проверить DNS", err)
		return
	}
	if !status.Enabled || !status.Running {
		jsonMsg(c, "Не удалось проверить DNS", errors.New("сначала включите собственный DNS"))
		return
	}
	if err := oxidns.CheckDNS(ctx); err != nil {
		jsonMsg(c, "Проверка DNS не пройдена", err)
		return
	}
	jsonObj(c, gin.H{"healthy": true}, nil)
}

func (a *OxiDNSController) restartNative(ctx context.Context) error {
	coreType, err := a.settings.GetCoreType()
	if err != nil {
		return err
	}
	if !service.IsNativeCore(coreType) {
		return nil
	}
	native := service.SelectedNativeCore()
	if !native.Installed() {
		return nil
	}
	if !native.IsRunning() {
		// A stopped sing-box may still have the old OxiDNS overlay on disk.
		// Regenerate it now so uninstall removes every runtime reference.
		return native.WriteConfig()
	}
	return native.Restart(ctx)
}

// Only changes to the sing-box DNS overlay require a sing-box restart.
func needsNativeDNSRestart(before, after oxidns.Settings) bool {
	return (before.Enabled && before.UseSingBox) != (after.Enabled && after.UseSingBox)
}

// Restore the previous usable DNS service if the sing-box change cannot be applied.
func (a *OxiDNSController) restoreDNS(previous oxidns.Settings, certFile, keyFile string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var err error
	if previous.Enabled {
		err = oxidns.Setup(ctx, previous, certFile, keyFile)
	} else {
		err = oxidns.Disable(ctx)
	}
	if err != nil {
		return err
	}
	return a.restartNative(ctx)
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
	if cfg.DoTPort == 0 {
		cfg.DoTPort = 853
	}
	if cfg.DoQPort == 0 {
		cfg.DoQPort = 853
	}
	if cfg.Upstream == "" {
		cfg.Upstream = "quad9"
	}
	if cfg.Domain == "" {
		domain, _ := a.settings.GetWebDomain()
		cfg.Domain = strings.TrimSpace(domain)
	}
	previous, err := oxidns.Load()
	if err != nil {
		jsonMsg(c, "Не удалось прочитать прежние настройки DNS", err)
		return
	}
	certFile, _ := a.settings.GetCertFile()
	keyFile, _ := a.settings.GetKeyFile()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()
	cfg.Enabled = true
	if err := oxidns.Setup(ctx, cfg, certFile, keyFile); err != nil {
		jsonMsg(c, "Не удалось включить OxiDNS", err)
		return
	}
	if needsNativeDNSRestart(previous, cfg) {
		if err := a.restartNative(ctx); err != nil {
			rollbackErr := a.restoreDNS(previous, certFile, keyFile)
			jsonMsg(c, "Не удалось применить OxiDNS к sing-box; восстановлены предыдущие настройки, если это было возможно", errors.Join(err, rollbackErr))
			return
		}
	}
	status, err := oxidns.CurrentStatus(ctx)
	jsonObj(c, status, err)
}

func (a *OxiDNSController) disable(c *gin.Context) {
	previous, err := oxidns.Load()
	if err != nil {
		jsonMsg(c, "Не удалось прочитать прежние настройки DNS", err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	if err := oxidns.Disable(ctx); err != nil {
		jsonMsg(c, "Не удалось отключить OxiDNS", err)
		return
	}
	if previous.Enabled && previous.UseSingBox {
		if err := a.restartNative(ctx); err != nil {
			certFile, _ := a.settings.GetCertFile()
			keyFile, _ := a.settings.GetKeyFile()
			rollbackErr := a.restoreDNS(previous, certFile, keyFile)
			jsonMsg(c, "Не удалось переключить DNS sing-box; предпринята попытка восстановления", errors.Join(err, rollbackErr))
			return
		}
	}
	status, err := oxidns.CurrentStatus(ctx)
	jsonObj(c, status, err)
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
	previous, err := oxidns.Load()
	if err != nil {
		jsonMsg(c, "Не удалось прочитать прежние настройки DNS", err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	if previous.Enabled {
		if err := oxidns.Disable(ctx); err != nil {
			jsonMsg(c, "Не удалось остановить OxiDNS перед удалением", err)
			return
		}
		if previous.UseSingBox {
			if err := a.restartNative(ctx); err != nil {
				certFile, _ := a.settings.GetCertFile()
				keyFile, _ := a.settings.GetKeyFile()
				rollbackErr := a.restoreDNS(previous, certFile, keyFile)
				jsonMsg(c, "OxiDNS оставлен на сервере: sing-box не удалось переключить на прежний DNS", errors.Join(err, rollbackErr))
				return
			}
		}
	}
	if err := oxidns.Uninstall(ctx); err != nil {
		jsonMsg(c, "Не удалось удалить OxiDNS", err)
		return
	}
	status, err := oxidns.CurrentStatus(ctx)
	jsonObj(c, status, err)
}
