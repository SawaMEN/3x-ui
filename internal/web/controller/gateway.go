package controller

import (
	"fmt"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/gateway"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// GatewayController exposes the transparent gateway controls used by the panel
// UI. The active core decides which config template receives the Gateway
// inbound, while the gateway package owns the core-specific config mutation.
type GatewayController struct {
	settingService service.SettingService
	xrayService    service.XrayService
	operationMu    sync.Mutex
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

func gatewayStateForCore(coreType string) (gateway.State, error) {
	if coreType == service.CoreTypeSingBox {
		return gateway.GetSingBoxState()
	}
	return gateway.GetState()
}

func enableGatewayForCore(coreType string) error {
	if coreType == service.CoreTypeSingBox {
		return gateway.EnableSingBox()
	}
	return gateway.Enable()
}

func disableGatewayForCore(coreType string) error {
	if coreType == service.CoreTypeSingBox {
		return gateway.DisableSingBox()
	}
	return gateway.Disable()
}

func (a *GatewayController) statusPayload() (gin.H, error) {
	coreType, coreErr := a.settingService.GetCoreType()
	state, stateErr := gatewayStateForCore(coreType)

	payload := gin.H{
		"enabled":        state.Enabled,
		"configured":     state.Configured,
		"recoveryBackup": state.BackupExists,
		"canEnable":      !state.Enabled,
		"coreType":       coreType,
		"xrayRunning":    a.xrayService.IsXrayRunning(),
		"port":           gateway.InboundPort(),
	}
	if coreErr != nil {
		return payload, coreErr
	}
	if stateErr != nil {
		return payload, stateErr
	}
	return payload, nil
}

func (a *GatewayController) status(c *gin.Context) {
	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}

func (a *GatewayController) enable(c *gin.Context) {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()

	coreType, err := a.settingService.GetCoreType()
	if err != nil {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, err)
		return
	}

	state, err := gatewayStateForCore(coreType)
	if err != nil {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, err)
		return
	}

	changed := false
	if !state.Enabled {
		if err := enableGatewayForCore(coreType); err != nil {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, err)
			return
		}
		changed = true
	}

	// A manually stopped core must stay stopped. Repeated enable requests are
	// idempotent and do not restart an unchanged core process.
	if changed && a.xrayService.IsXrayRunning() {
		if err := a.xrayService.RestartXray(false); err != nil {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, fmt.Errorf("Gateway Mode was enabled, but %s restart failed: %w", coreType, err))
			return
		}
	}

	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}

func (a *GatewayController) disable(c *gin.Context) {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()

	coreType, err := a.settingService.GetCoreType()
	if err != nil {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, err)
		return
	}

	state, err := gatewayStateForCore(coreType)
	if err != nil {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, err)
		return
	}

	changed := false
	if state.Enabled {
		if err := disableGatewayForCore(coreType); err != nil {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, err)
			return
		}
		changed = true
	}

	if changed && a.xrayService.IsXrayRunning() {
		if err := a.xrayService.RestartXray(false); err != nil {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, fmt.Errorf("Gateway Mode was disabled, but %s restart failed: %w", coreType, err))
			return
		}
	}

	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}
