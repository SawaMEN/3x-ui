package controller

import (
	"fmt"
	"strings"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/gateway"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

const xrayGatewayCoreType = "xray"

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

func otherGatewayCore(coreType string) string {
	if coreType == service.CoreTypeSingBox {
		return xrayGatewayCoreType
	}
	return service.CoreTypeSingBox
}

type gatewayStatus struct {
	State     gateway.State
	OwnerCore string
	Conflict  bool
}

// gatewayStatusForCore treats Gateway Mode as one logical feature even though
// Xray and sing-box keep independent recovery backups. This is important after
// switching cores: stale Gateway state in the previously selected core must
// remain visible and removable instead of looking disabled.
func gatewayStatusForCore(coreType string) (gatewayStatus, error) {
	activeState, err := gatewayStateForCore(coreType)
	if err != nil {
		return gatewayStatus{}, fmt.Errorf("get %s Gateway state: %w", coreType, err)
	}

	otherCore := otherGatewayCore(coreType)
	otherState, otherErr := gatewayStateForCore(otherCore)
	if otherErr != nil {
		// An unused core may not have a usable template yet. Do not make Gateway
		// status unavailable for the selected core solely because the inactive
		// core cannot be inspected.
		owner := ""
		if activeState.Enabled {
			owner = coreType
		}
		return gatewayStatus{State: activeState, OwnerCore: owner}, nil
	}

	if activeState.Enabled && otherState.Enabled {
		activeState.Configured = activeState.Configured || otherState.Configured
		activeState.BackupExists = activeState.BackupExists || otherState.BackupExists
		return gatewayStatus{
			State:     activeState,
			OwnerCore: "multiple",
			Conflict:  true,
		}, nil
	}
	if activeState.Enabled {
		return gatewayStatus{State: activeState, OwnerCore: coreType}, nil
	}
	if otherState.Enabled {
		return gatewayStatus{State: otherState, OwnerCore: otherCore}, nil
	}
	return gatewayStatus{State: activeState}, nil
}

func (a *GatewayController) statusPayload() (gin.H, error) {
	coreType, coreErr := a.settingService.GetCoreType()
	payload := gin.H{
		"enabled":         false,
		"configured":      false,
		"recoveryBackup":  false,
		"canEnable":       false,
		"coreType":        coreType,
		"gatewayCoreType": "",
		"coreMismatch":    false,
		"conflict":        false,
		"xrayRunning":     a.xrayService.IsXrayRunning(),
		"port":            gateway.InboundPort(),
	}
	if coreErr != nil {
		return payload, coreErr
	}

	status, stateErr := gatewayStatusForCore(coreType)
	payload["enabled"] = status.State.Enabled
	payload["configured"] = status.State.Configured
	payload["recoveryBackup"] = status.State.BackupExists
	payload["gatewayCoreType"] = status.OwnerCore
	payload["conflict"] = status.Conflict
	payload["coreMismatch"] = status.OwnerCore != "" && status.OwnerCore != "multiple" && status.OwnerCore != coreType
	payload["canEnable"] = !status.Conflict && status.OwnerCore != coreType
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
	otherCore := otherGatewayCore(coreType)
	if otherState, otherErr := gatewayStateForCore(otherCore); otherErr == nil && otherState.Enabled {
		if err := disableGatewayForCore(otherCore); err != nil {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, fmt.Errorf("disable stale %s Gateway Mode before switching to %s: %w", otherCore, coreType, err))
			return
		}
		changed = true
	}

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

	changed := false
	failures := make([]string, 0, 2)
	for _, candidate := range []string{coreType, otherGatewayCore(coreType)} {
		state, stateErr := gatewayStateForCore(candidate)
		if stateErr != nil {
			// The inactive core may not have a template yet. The selected core is
			// always expected to be inspectable; surface that failure immediately.
			if candidate == coreType {
				failures = append(failures, fmt.Sprintf("%s state: %v", candidate, stateErr))
			}
			continue
		}
		if !state.Enabled {
			continue
		}
		if err := disableGatewayForCore(candidate); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", candidate, err))
			continue
		}
		changed = true
	}

	if changed && a.xrayService.IsXrayRunning() {
		if err := a.xrayService.RestartXray(false); err != nil {
			failures = append(failures, fmt.Sprintf("%s restart: %v", coreType, err))
		}
	}

	if len(failures) > 0 {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, fmt.Errorf("Gateway Mode disable incomplete: %s", strings.Join(failures, "; ")))
		return
	}

	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}
