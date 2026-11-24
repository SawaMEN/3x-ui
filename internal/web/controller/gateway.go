package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/gateway"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// GatewayController exposes the transparent gateway controls used by the panel
// UI. The active core decides which config template receives the Gateway
// inbound, while the gateway package owns the core-specific config mutation.
type GatewayController struct {
	settingService service.SettingService
	xrayService    service.CoreService
	networkService service.GatewayNetworkService
}

func NewGatewayController(g *gin.RouterGroup) *GatewayController {
	a := &GatewayController{}
	a.initRouter(g)
	return a
}

func (a *GatewayController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/gateway")
	g.GET("/status", a.status)
	g.GET("/interfaces", a.interfaces)
	g.POST("/preflight", a.preflight)
	g.POST("/enable", a.enable)
	g.POST("/disable", a.disable)
}

func gatewayStateForCore(coreType string) (gateway.State, error) {
	return gateway.StateForCore(coreType)
}

func enableGatewayForCore(coreType string) error {
	return gateway.EnableForCore(coreType)
}

func disableGatewayForCore(coreType string) error {
	return gateway.DisableForCore(coreType)
}

func gatewayOtherCores(coreType string) []string {
	var others []string
	for _, candidate := range []string{service.CoreTypeXray, service.CoreTypeSingBox, service.CoreTypeSingBoxUpstream} {
		if candidate != coreType {
			others = append(others, candidate)
		}
	}
	return others
}

// rollbackGatewayChange restores the two template states after a failed core
// migration. It deliberately operates on config only; the caller decides
// whether the running core also needs to be reconciled afterwards.
func rollbackGatewayChange(currentCore, previousCore string, disableCurrent, restorePrevious bool) error {
	failures := make([]string, 0, 2)
	if disableCurrent {
		if err := disableGatewayForCore(currentCore); err != nil {
			failures = append(failures, fmt.Sprintf("disable %s: %v", currentCore, err))
		}
	}
	if restorePrevious {
		if err := gateway.RestoreForCore(previousCore); err != nil {
			failures = append(failures, fmt.Sprintf("restore %s: %v", previousCore, err))
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

// gatewayStatusForCore treats Gateway Mode as one logical feature even though
// Xray and sing-box keep independent recovery backups. This is important after
// switching cores: stale Gateway state in the previously selected core must
// remain visible and removable instead of looking disabled.
func gatewayStatusForCore(coreType string) (gatewayStatus, error) {
	activeState, err := gatewayStateForCore(coreType)
	if err != nil {
		return gatewayStatus{}, fmt.Errorf("get %s Gateway state: %w", coreType, err)
	}

	result := gatewayStatus{State: activeState}
	if activeState.Enabled {
		result.OwnerCore = coreType
	}
	for _, candidate := range gatewayOtherCores(coreType) {
		state, err := gatewayStateForCore(candidate)
		if err != nil || !state.Enabled {
			continue
		}
		if result.OwnerCore == "" {
			result = gatewayStatus{State: state, OwnerCore: candidate}
			continue
		}
		result.State.Configured = result.State.Configured || state.Configured
		result.State.BackupExists = result.State.BackupExists || state.BackupExists
		result.OwnerCore = "multiple"
		result.Conflict = true
	}
	return result, nil
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
		"xrayRunning":     false,
		"coreRunning":     a.gatewayCoreRunning(coreType),
		"port":            gateway.InboundPort(),
		"network":         a.networkService.Status(context.Background()),
	}
	if coreErr != nil {
		return payload, coreErr
	}

	status, stateErr := gatewayStatusForCore(coreType)
	network := payload["network"].(service.GatewayNetworkStatus)
	payload["enabled"] = status.State.Enabled || network.Configured
	payload["configured"] = status.State.Configured
	payload["recoveryBackup"] = status.State.BackupExists
	payload["gatewayCoreType"] = status.OwnerCore
	payload["conflict"] = status.Conflict
	payload["coreMismatch"] = status.OwnerCore != "" && status.OwnerCore != "multiple" && status.OwnerCore != coreType
	payload["canEnable"] = !status.Conflict && (!status.State.Enabled || status.State.Configured) && a.gatewayCoreRunning(coreType)
	if stateErr != nil {
		return payload, stateErr
	}
	return payload, nil
}

func (a *GatewayController) status(c *gin.Context) {
	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}

type gatewayEnableRequest struct {
	Network service.GatewayNetworkConfig `json:"network"`
}

func (a *GatewayController) enable(c *gin.Context) {
	release, lockErr := gateway.AcquireOperation(c.Request.Context())
	if lockErr != nil {
		jsonObj(c, nil, lockErr)
		return
	}
	defer release()

	req := gatewayEnableRequest{Network: service.GatewayNetworkConfig{LANPrefix: 24}}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, fmt.Errorf("invalid Gateway network settings: %w", err))
			return
		}
	}

	coreType, err := a.settingService.GetCoreType()
	if err != nil {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, err)
		return
	}

	networkStatus := a.networkService.Status(c.Request.Context())
	networkConfig := req.Network
	if networkStatus.Configured {
		if networkStatus.Error != "" {
			jsonObj(c, nil, fmt.Errorf("Gateway recovery state: %s", networkStatus.Error))
			return
		}
		networkConfig = networkStatus.Config
	}
	if err := a.networkService.Validate(c.Request.Context(), networkConfig); err != nil {
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

	wasRunning := a.gatewayCoreRunning(coreType)
	if !wasRunning {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, fmt.Errorf("start the selected core before enabling Gateway Mode"))
		return
	}
	if state.Enabled && !state.Configured {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, fmt.Errorf("Gateway template is incomplete; disable recovery state before enabling"))
		return
	}
	if networkStatus.Configured && !state.Enabled {
		if err := a.networkService.Suspend(c.Request.Context()); err != nil {
			jsonObj(c, nil, err)
			return
		}
	}
	changed := false
	currentEnabledByRequest := false
	previousCoreDisabled := false
	otherCore := ""
	for _, candidate := range gatewayOtherCores(coreType) {
		if otherState, otherErr := gatewayStateForCore(candidate); otherErr == nil && otherState.Enabled {
			if otherCore != "" {
				payload, _ := a.statusPayload()
				jsonObj(c, payload, fmt.Errorf("multiple stale Gateway templates; disable Gateway first"))
				return
			}
			otherCore = candidate
		}
	}
	if otherCore != "" {
		if err := disableGatewayForCore(otherCore); err != nil {
			if restoreErr := gateway.RestoreForCore(otherCore); restoreErr != nil {
				err = fmt.Errorf("%w; restore stale Gateway: %w", err, restoreErr)
			}
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

	// A manually stopped core must stay stopped. Repeated enable requests are
	// idempotent and do not restart an unchanged core process.
	if changed && wasRunning {
		if restartErr := a.restartGatewayCore(context.Background(), coreType); restartErr != nil {
			rollbackErr := rollbackGatewayChange(coreType, otherCore, currentEnabledByRequest, previousCoreDisabled)
			if rollbackErr != nil {
				payload, _ := a.statusPayload()
				jsonObj(c, payload, fmt.Errorf("apply %s Gateway Mode restart failed: %w; rollback failed: %w", coreType, restartErr, rollbackErr))
				return
			}

			// The process was running before this request. Once the templates are
			// restored, reconcile the runtime again so a failed/partial hot reload
			// cannot leave it serving the attempted Gateway configuration.
			if restoreErr := a.restartGatewayCore(context.Background(), coreType); restoreErr != nil {
				payload, _ := a.statusPayload()
				jsonObj(c, payload, fmt.Errorf("apply %s Gateway Mode restart failed: %w; config rollback succeeded but runtime restore failed: %w", coreType, restartErr, restoreErr))
				return
			}

			payload, _ := a.statusPayload()
			jsonObj(c, payload, fmt.Errorf("apply %s Gateway Mode restart failed and the Gateway configuration was rolled back: %w", coreType, restartErr))
			return
		}
	}

	if err := a.networkService.Enable(c.Request.Context(), networkConfig); err != nil {
		if !networkStatus.Configured && a.networkService.Status(c.Request.Context()).Configured {
			payload, _ := a.statusPayload()
			jsonObj(c, payload, fmt.Errorf("%w; partial network setup requires cleanup; Gateway listener retained", err))
			return
		}
		if changed {
			rollbackErr := rollbackGatewayChange(coreType, otherCore, currentEnabledByRequest, previousCoreDisabled)
			if rollbackErr == nil && wasRunning {
				rollbackErr = a.restartGatewayCore(context.Background(), coreType)
			}
			if rollbackErr != nil {
				err = fmt.Errorf("configure Linux Gateway networking: %w; core rollback failed: %w", err, rollbackErr)
			} else {
				err = fmt.Errorf("configure Linux Gateway networking: %w; core changes rolled back", err)
			}
		}
		payload, _ := a.statusPayload()
		jsonObj(c, payload, err)
		return
	}

	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}

func (a *GatewayController) disable(c *gin.Context) {
	release, err := gateway.AcquireOperation(c.Request.Context())
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	defer release()
	// Once accepted, a browser disconnect must not interrupt deactivation.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fail := func(cause error) {
		payload, _ := a.statusPayload()
		jsonObj(c, payload, cause)
	}
	// Persist OFF before inspecting templates. Never re-enable on failure.
	if err := a.networkService.Suspend(ctx); err != nil {
		fail(err)
		return
	}
	coreType, err := a.settingService.GetCoreType()
	if err != nil {
		fail(err)
		return
	}
	for _, candidate := range append([]string{coreType}, gatewayOtherCores(coreType)...) {
		state, stateErr := gatewayStateForCore(candidate)
		if stateErr != nil {
			fail(fmt.Errorf("inspect %s Gateway: %w", candidate, stateErr))
			return
		}
		if state.Enabled {
			if err := disableGatewayForCore(candidate); err != nil {
				fail(err)
				return
			}
		}
	}
	// Retry even if an earlier request already removed the templates.
	if a.gatewayCoreRunning(coreType) {
		if err := a.restartGatewayCore(ctx, coreType); err != nil {
			fail(err)
			return
		}
	}
	if err := a.networkService.Disable(ctx); err != nil {
		fail(err)
		return
	}
	payload, err := a.statusPayload()
	jsonObj(c, payload, err)
}

func (a *GatewayController) gatewayCoreRunning(coreType string) bool {
	if service.IsNativeCore(coreType) {
		return service.NativeCore(coreType).IsRunning()
	}
	return a.xrayService.IsCoreRunning()
}

func (a *GatewayController) restartGatewayCore(ctx context.Context, coreType string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if service.IsNativeCore(coreType) {
		return service.NativeCore(coreType).Restart(ctx)
	}
	return a.xrayService.RestartXray(false)
}
