package controller

import (
	"errors"

	"github.com/SawaMEN/3x-ui/v3/internal/gateway"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// GatewayController exposes the transparent Xray gateway controls used by the
// panel UI. The underlying gateway package remains the single implementation
// shared with the `x-ui gateway` CLI commands.
type GatewayController struct {
	settingService service.SettingService
	xrayService    service.XrayService
}

func NewGatewayController(g *gin.RouterGroup) *GatewayController {
	a := &GatewayController{}
	a.initRouter(g)
	return a
}

func (a *GatewayController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/gateway")
	g.GET("/status", a.status)
	g.POST("/enable", a.enable)
	g.POST("/disable", a.disable)
}

func (a *GatewayController) statusPayload() (gin.H, error) {
	coreType, err := a.settingService.GetCoreType()
	if err != nil {
		return gin.H{"enabled": gateway.IsEnabled()}, err
	}
	return gin.H{
		"enabled":     gateway.IsEnabled(),
		"canEnable":   coreType != service.CoreTypeSingBox,
		"coreType":    coreType,
		"xrayRunning": a.xrayService.IsXrayRunning(),
	}, nil
}

func (a *GatewayController) status(c *gin.Context) {
	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}

func (a *GatewayController) enable(c *gin.Context) {
	coreType, err := a.settingService.GetCoreType()
	if err != nil {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, err)
		return
	}
	if coreType == service.CoreTypeSingBox {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, errors.New("Gateway Mode is available only when Xray is selected"))
		return
	}

	if !gateway.IsEnabled() {
		if err := gateway.Enable(); err != nil {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, err)
			return
		}
	}

	// Keep the same semantics as the Xray settings editor: apply the changed
	// template immediately only when Xray is already running. A manually stopped
	// core must stay stopped.
	if a.xrayService.IsXrayRunning() {
		if err := a.xrayService.RestartXray(false); err != nil {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, err)
			return
		}
	}

	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}

func (a *GatewayController) disable(c *gin.Context) {
	coreType, coreErr := a.settingService.GetCoreType()
	if coreErr != nil {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, coreErr)
		return
	}

	if gateway.IsEnabled() {
		if err := gateway.Disable(); err != nil {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, err)
			return
		}
	}

	// Disabling remains available even after the operator has switched to
	// sing-box, so a stale Gateway backup can always be restored. Restart Xray
	// only when it is the selected running core.
	if coreType != service.CoreTypeSingBox && a.xrayService.IsXrayRunning() {
		if err := a.xrayService.RestartXray(false); err != nil {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, err)
			return
		}
	}

	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}
