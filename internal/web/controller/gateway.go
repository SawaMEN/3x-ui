package controller

import (
	"fmt"
	"strings"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/gateway"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

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
	switch coreType {
	case service.CoreTypeXray:
		return gateway.GetState()
	case service.CoreTypeSingBox:
		return gateway.GetSingBoxState()
	default:
		return gateway.State{}, fmt.Errorf("unsupported core type %q", coreType)
	}
}

func enableGatewayForCore(coreType string) error {
	switch coreType {
	case service.CoreTypeXray:
		return gateway.Enable()
	case service.CoreTypeSingBox:
		return gateway.EnableSingBox()
	default:
		return fmt.Errorf("unsupported core type %q", coreType)
	}
}

func disableGatewayForCore(coreType string) error {
	switch coreType {
	case service.CoreTypeXray:
		return gateway.Disable()
	case service.CoreTypeSingBox:
		return gateway.DisableSingBox()
	default:
		return fmt.Errorf("unsupported core type %q", coreType)
	}
}

func otherGatewayCore(coreType string) string {
	if coreType == service.CoreTypeSingBox {
		return service.CoreTypeXray
	}
	return service.CoreTypeSingBox
}

func rollbackGatewayChange(currentCore, previousCore string, disableCurrent, restorePrevious bool) error {
	failures := make([]string, 0, 2)
	if disableCurrent {
		if err := disableGatewayForCore(currentCore); err != nil {
			failures = append(failures, fmt.Sprintf("disable %s: %v", currentCore, err))
		}
	}
	if restorePrevious {
		if err := enableGatewayForCore(previousCore); err != nil {
			failures = append(failures, fmt.Sprintf("restore %s: %v", previousCore, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

func restoreGatewayCores(cores []string) error {
	failures := make([]string, 0, len(cores))
	for i := len(cores) - 1; i >= 0; i-- {
		coreType := cores[i]
		if err := enableGatewayForCore(coreType); err != nil {
			failures = append(failures, fmt.Sprintf("restore %s: %v", coreType, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

type gatewayStatus struct {
	State     gateway.State
	OwnerCore string
	Conflict  bool
}

func gatewayStatusForCore(coreType string) (gatewayStatus, error) {
	activeState, err := gatewayStateForCore(coreType)
	if err != nil {
		return gatewayStatus{}, fmt.Errorf("get %s Gateway state: %w", coreType, err)
	}

	otherCore := otherGatewayCore(coreType)
	otherState, otherErr := gatewayStateForCore(otherCore)
	if otherErr != nil {
		owner := ""
		if activeState.Enabled {
			owner = coreType
		}
		return gatewayStatus{State: activeState, OwnerCore: owner}, nil
	}

	if activeState.Enabled && otherState.Enabled {
		activeState.Configured = activeState.Configured || otherState.Configured
		activeState.BackupExists = activeState.BackupExists || otherState.BackupExists
		return gatewayStatus{State: activeState, OwnerCore: "multiple", Conflict: true}, nil
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
		"enabled":          false,
		"configured":       false,
		"recoveryBackup":   false,
		"canEnable":        false,
		"coreType":         coreType,
		"gatewayCoreType":  "",
		"coreMismatch":     false,
		"conflict":         false,
		"xrayRunning":      a.xrayService.IsXrayRunning(),
		"port":             gateway.InboundPort(),
		"systemConfigured": false,
		"systemActive":     false,
		"ipForward":        false,
		"policyRoute":      false,
		"firewall":         false,
		"nat":              false,
		"lanInterface":     "",
		"lanIP":            "",
		"lanPrefix":        24,
		"lanNetwork":       "",
		"wanInterface":     "",
		"systemError":      "",
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
	if stateErr != nil {
		return payload, stateErr
	}

	systemState, systemErr := gateway.GetSystemState()
	payload["systemConfigured"] = systemState.Configured
	payload["systemActive"] = systemState.Active
	payload["ipForward"] = systemState.IPForward
	payload["policyRoute"] = systemState.PolicyRoute
	payload["firewall"] = systemState.Firewall
	payload["nat"] = systemState.NAT
	payload["lanInterface"] = systemState.Config.LANInterface
	payload["lanIP"] = systemState.Config.LANIP
	if systemState.Config.LANPrefix != 0 || systemState.Config.LANIP != "" {
		payload["lanPrefix"] = systemState.Config.LANPrefix
	}
	payload["lanNetwork"] = systemState.LANNetwork
	payload["wanInterface"] = systemState.Config.WANInterface
	if systemErr != nil {
		payload["systemError"] = systemErr.Error()
	}

	canRepair := status.OwnerCore == coreType && status.State.Configured && !systemState.Active
	payload["canEnable"] = !status.Conflict && (status.OwnerCore != coreType || canRepair)
	return payload, nil
}

func (a *GatewayController) status(c *gin.Context) {
	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}

func (a *GatewayController) restoreCoreAfterFailedSystemChange(coreType, otherCore string, currentEnabledByRequest, previousCoreDisabled, wasRunning bool) error {
	rollbackErr := rollbackGatewayChange(coreType, otherCore, currentEnabledByRequest, previousCoreDisabled)
	if rollbackErr != nil {
		return rollbackErr
	}
	if wasRunning && (currentEnabledByRequest || previousCoreDisabled) {
		if err := a.xrayService.RestartXray(false); err != nil {
			return fmt.Errorf("restore core runtime: %w", err)
		}
	}
	return nil
}

func (a *GatewayController) enable(c *gin.Context) {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()

	var requested gateway.SystemConfig
	if err := c.ShouldBindJSON(&requested); err != nil {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, fmt.Errorf("invalid Gateway network settings: %w", err))
		return
	}
	systemConfig, err := gateway.ValidateSystemConfig(requested)
	if err != nil {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, err)
		return
	}

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

	wasRunning := a.xrayService.IsXrayRunning()
	changed := false
	currentEnabledByRequest := false
	previousCoreDisabled := false
	otherCore := otherGatewayCore(coreType)
	if otherState, otherErr := gatewayStateForCore(otherCore); otherErr == nil && otherState.Enabled {
		if err := disableGatewayForCore(otherCore); err != nil {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, fmt.Errorf("disable stale %s Gateway Mode before switching to %s: %w", otherCore, coreType, err))
			return
		}
		previousCoreDisabled = true
		changed = true
	}

	if !state.Enabled {
		if err := enableGatewayForCore(coreType); err != nil {
			if previousCoreDisabled {
				if rollbackErr := rollbackGatewayChange(coreType, otherCore, false, true); rollbackErr != nil {
					err = fmt.Errorf("enable %s Gateway Mode: %w; rollback failed: %w", coreType, err, rollbackErr)
				} else {
					err = fmt.Errorf("enable %s Gateway Mode: %w; previous %s Gateway Mode restored", coreType, err, otherCore)
				}
			}
			payload, _ := a.statusPayload()
			jsonObj(c, payload, err)
			return
		}
		currentEnabledByRequest = true
		changed = true
	}

	if changed && wasRunning {
		if restartErr := a.xrayService.RestartXray(false); restartErr != nil {
			rollbackErr := rollbackGatewayChange(coreType, otherCore, currentEnabledByRequest, previousCoreDisabled)
			if rollbackErr != nil {
				payload, _ := a.statusPayload()
				jsonObj(c, payload, fmt.Errorf("apply %s Gateway Mode restart failed: %w; rollback failed: %w", coreType, restartErr, rollbackErr))
				return
			}
			if restoreErr := a.xrayService.RestartXray(false); restoreErr != nil {
				payload, _ := a.statusPayload()
				jsonObj(c, payload, fmt.Errorf("apply %s Gateway Mode restart failed: %w; config rollback succeeded but runtime restore failed: %w", coreType, restartErr, restoreErr))
				return
			}
			payload, _ := a.statusPayload()
			jsonObj(c, payload, fmt.Errorf("apply %s Gateway Mode restart failed and the Gateway configuration was rolled back: %w", coreType, restartErr))
			return
		}
	}

	if err := gateway.EnableSystem(systemConfig); err != nil {
		rollbackErr := a.restoreCoreAfterFailedSystemChange(coreType, otherCore, currentEnabledByRequest, previousCoreDisabled, wasRunning)
		payload, _ := a.statusPayload()
		if rollbackErr != nil {
			jsonObj(c, payload, fmt.Errorf("configure Linux Gateway networking: %w; core rollback failed: %v", err, rollbackErr))
			return
		}
		jsonObj(c, payload, fmt.Errorf("configure Linux Gateway networking: %w", err))
		return
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

	wasRunning := a.xrayService.IsXrayRunning()
	disabledCores := make([]string, 0, 2)
	for _, candidate := range []string{coreType, otherGatewayCore(coreType)} {
		state, stateErr := gatewayStateForCore(candidate)
		if stateErr != nil {
			if candidate == coreType {
				payload, _ := a.statusPayload()
				jsonObj(c, payload, fmt.Errorf("%s Gateway state: %w", candidate, stateErr))
				return
			}
			continue
		}
		if !state.Enabled {
			continue
		}
		if err := disableGatewayForCore(candidate); err != nil {
			rollbackErr := restoreGatewayCores(disabledCores)
			payload, _ := a.statusPayload()
			if rollbackErr != nil {
				jsonObj(c, payload, fmt.Errorf("disable %s Gateway Mode: %w; rollback failed: %w", candidate, err, rollbackErr))
			} else {
				jsonObj(c, payload, fmt.Errorf("disable %s Gateway Mode: %w; earlier changes rolled back", candidate, err))
			}
			return
		}
		disabledCores = append(disabledCores, candidate)
	}

	if len(disabledCores) > 0 && wasRunning {
		if restartErr := a.xrayService.RestartXray(false); restartErr != nil {
			rollbackErr := restoreGatewayCores(disabledCores)
			if rollbackErr != nil {
				payload, _ := a.statusPayload()
				jsonObj(c, payload, fmt.Errorf("disable Gateway Mode restart failed: %w; rollback failed: %w", restartErr, rollbackErr))
				return
			}
			if restoreErr := a.xrayService.RestartXray(false); restoreErr != nil {
				payload, _ := a.statusPayload()
				jsonObj(c, payload, fmt.Errorf("disable Gateway Mode restart failed: %w; config rollback succeeded but runtime restore failed: %w", restartErr, restoreErr))
				return
			}
			payload, _ := a.statusPayload()
			jsonObj(c, payload, fmt.Errorf("disable Gateway Mode restart failed and the Gateway configuration was rolled back: %w", restartErr))
			return
		}
	}

	if systemErr := gateway.DisableSystem(); systemErr != nil {
		rollbackErr := restoreGatewayCores(disabledCores)
		if rollbackErr == nil && wasRunning && len(disabledCores) > 0 {
			rollbackErr = a.xrayService.RestartXray(false)
		}
		payload, _ := a.statusPayload()
		if rollbackErr != nil {
			jsonObj(c, payload, fmt.Errorf("disable Linux Gateway networking: %w; core rollback failed: %v", systemErr, rollbackErr))
			return
		}
		jsonObj(c, payload, fmt.Errorf("disable Linux Gateway networking: %w; core configuration restored", systemErr))
		return
	}

	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}
