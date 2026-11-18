package controller

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/web/session"
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

 // Run only one active probe at a time. OpenFlux exits support one client
 // session, so concurrent tests would invalidate each other's observations.
 var openFluxDiagnosticSlot = make(chan struct{}, 1)

 // diagnoseOpenFluxTransport uses only a persisted inbound owned by the
 // authenticated user. It never accepts arbitrary URLs, IPs, passwords or
 // executable paths in the request.
 func (a *ServerController) diagnoseOpenFluxTransport(c *gin.Context) {
 	c.Header("Cache-Control", "no-store")
 	var input struct {
 		InboundID int `json:"inboundId"`
 		Type string `json:"type"`
 		ClientToken string `json:"clientToken"`
 		ConfirmInterruption bool `json:"confirmInterruption"`
 	}
 	if err := c.ShouldBindJSON(&input); err != nil {
 		jsonObj(c, nil, err)
 		return
 	}
 	if input.InboundID <= 0 || !externalvpn.OpenFluxDiagnosticTypeSupported(input.Type) {
 		jsonObj(c, nil, fmt.Errorf("select a saved OpenFlux inbound and a known transport"))
 		return
 	}
 	if !input.ConfirmInterruption {
 		jsonObj(c, nil, fmt.Errorf("confirm that an end-to-end probe can temporarily interrupt an active OpenFlux client"))
 		return
 	}
 	user := session.GetLoginUser(c)
 	if user == nil {
 		jsonObj(c, nil, fmt.Errorf("authentication required"))
 		return
 	}
 	if nodeID, err := singBoxSessionNodeIDFromQuery(c); err != nil || nodeID != nil {
 		jsonObj(c, nil, fmt.Errorf("OpenFlux end-to-end probes are local-only; run the test on the inbound's node"))
 		return
 	}
 	var inboundService service.InboundService
 	ib, err := inboundService.GetInbound(input.InboundID)
 	if err != nil || ib == nil || ib.UserId != user.Id || ib.Protocol != model.OpenFlux {
 		jsonObj(c, nil, fmt.Errorf("saved OpenFlux inbound not found for this user"))
 		return
 	}
 	if ib.NodeID != nil || !ib.Enable {
 		jsonObj(c, nil, fmt.Errorf("the saved OpenFlux inbound must be enabled and running locally"))
 		return
 	}
 	inst, err := externalvpn.FromInbound(ib)
 	if err != nil {
 		jsonObj(c, nil, err)
 		return
 	}
	job, err := startOpenFluxDiagnosticJob(user.Id, inst, input.Type, input.ClientToken)
	jsonObj(c, job, err)
 }
