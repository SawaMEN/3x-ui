package controller

import (
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/gin-gonic/gin"
)

// Interface discovery only inspects network addresses and the default route.
// It never changes forwarding, firewall rules or running core state.
func (a *GatewayController) interfaces(c *gin.Context) {
	result, err := a.networkService.Discover(c.Request.Context())
	jsonObj(c, result, err)
}

type gatewayPreflightResult struct {
	Ready    bool     `json:"ready"`
	Problems []string `json:"problems"`
	Notes    []string `json:"notes"`
}

// Preflight is advisory: enable repeats the host checks under the Gateway
// operation lock. A successful preflight never activates interception.
func (a *GatewayController) preflight(c *gin.Context) {
	result := gatewayPreflightResult{Problems: []string{}, Notes: []string{}}
	var req gatewayEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonObj(c, nil, fmt.Errorf("invalid Gateway preflight request: %w", err))
		return
	}
	core, err := a.settingService.GetCoreType()
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	state, err := gatewayStatusForCore(core)
	if err != nil {
		result.Problems = append(result.Problems, err.Error())
	} else {
		if state.Conflict {
			result.Problems = append(result.Problems, "конфигурация шлюза одновременно присутствует в обоих ядрах")
		}
		if state.State.Enabled && !state.State.Configured && !state.Conflict {
			result.Problems = append(result.Problems, "сохранено неполное состояние шлюза; сначала выключите его")
		}
		if state.OwnerCore != "" && state.OwnerCore != core && !state.Conflict {
			result.Notes = append(result.Notes, "при включении шлюз будет перенесён с предыдущего ядра")
		}
	}
	if !a.gatewayCoreRunning(core) {
		result.Problems = append(result.Problems, "выбранное ядро остановлено; запустите его перед включением")
	}
	network := a.networkService.Status(c.Request.Context())
	if network.Configured && network.Error != "" {
		result.Problems = append(result.Problems, "повреждено сохранённое состояние сети: "+network.Error)
	} else {
		config := req.Network
		if network.Configured {
			if network.Config != req.Network {
				result.Problems = append(result.Problems, "сетевые настройки отличаются от сохранённых; выключите шлюз перед их изменением")
			}
			config = network.Config
		}
		if err := a.networkService.Preflight(c.Request.Context(), config); err != nil {
			result.Problems = append(result.Problems, err.Error())
		}
	}
	if len(result.Problems) == 0 {
		result.Ready = true
		result.Notes = append(result.Notes, "проверка выполнена без изменения системы; окончательная проверка пройдёт при включении")
	}
	jsonObj(c, result, nil)
}

// Keep this compile-time reference so the service's public request format is
// explicitly owned by the same validated Gateway network configuration.
var _ = service.GatewayNetworkConfig{}
