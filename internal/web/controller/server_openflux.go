package controller

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	webruntime "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

func (a *ServerController) openFluxInstall(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	nodeID, err := singBoxSessionNodeIDFromQuery(c)
	if err != nil {
		jsonMsg(c, "Invalid OpenFlux installation node", err)
		return
	}
	if nodeID != nil {
		manager := webruntime.GetManager()
		if manager == nil {
			jsonMsg(c, "Cannot access node runtime", fmt.Errorf("runtime manager is not initialized"))
			return
		}
		rt, err := manager.RuntimeFor(nodeID)
		if err != nil {
			jsonMsg(c, "Cannot access node runtime", err)
			return
		}
		installer, ok := rt.(webruntime.OpenFluxInstaller)
		if !ok {
			jsonMsg(c, "Cannot install OpenFlux on this node", fmt.Errorf("selected runtime does not support OpenFlux installation"))
			return
		}
		if c.GetBool("openflux_status_only") {
			result, err := installer.ReadOpenFluxStatus(c.Request.Context())
			jsonObj(c, result, err)
		} else if c.Request.Method == http.MethodPost {
			result, err := installer.StartOpenFluxInstall(c.Request.Context())
			jsonObj(c, result, err)
		} else {
			result, err := installer.ReadOpenFluxInstall(c.Request.Context())
			jsonObj(c, result, err)
		}
		return
	}
	if c.GetBool("openflux_status_only") {
		jsonObj(c, externalvpn.OpenFluxBinaryStatus(c.Request.Context(), true), nil)
	} else if c.Request.Method == http.MethodPost {
		jsonObj(c, externalvpn.StartOpenFluxInstallation(), nil)
	} else {
		jsonObj(c, externalvpn.GetOpenFluxInstallation(), nil)
	}
}

func (a *ServerController) openFluxStatus(c *gin.Context) {
	c.Set("openflux_status_only", true)
	a.openFluxInstall(c)
}

func (a *ServerController) checkOpenFluxTransport(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	nodeID, err := singBoxSessionNodeIDFromQuery(c)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if nodeID != nil {
		jsonObj(c, nil, fmt.Errorf("provider checks on remote nodes are not yet supported; run the test on the selected node"))
		return
	}
	var input struct {
		Type string `json:"type"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		jsonObj(c, nil, err)
		return
	}
	result, err := externalvpn.ProbeOpenFluxProvider(c.Request.Context(), input.Type)
	jsonObj(c, result, err)
}
