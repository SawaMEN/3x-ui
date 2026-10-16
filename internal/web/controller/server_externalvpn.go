package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/util/link"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func (a *ServerController) initExternalVPNRouter(g *gin.RouterGroup) {
	g.GET("/externalvpn/status", a.getExternalVPNUpdateStatus)
	g.POST("/externalvpn/parse-link", a.parseExternalVPNLink)
	g.POST("/externalvpn/update/:protocol", a.updateExternalVPNBinary)
	g.POST("/components/remove/:protocol", a.removeProtocolComponent)
}

func parseExternalVPNProtocol(value string) (model.Protocol, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hysteria2":
		return model.Hysteria, nil
	case string(model.Pingtunnel):
		return model.Pingtunnel, nil
	case string(model.FPTN):
		return model.FPTN, nil
	case string(model.OpenFlux):
		return model.OpenFlux, nil
	case string(model.TrustTunnel):
		return model.TrustTunnel, nil
	default:
		return "", fmt.Errorf("unsupported external VPN component %q", value)
	}
}

func (a *ServerController) getExternalVPNUpdateStatus(c *gin.Context) {
	status := externalvpn.GetUpdateStatus(c.Request.Context(), c.Query("local") == "true")
	if c.Query("local") != "true" {
		status.Hysteria2.Source, status.Hysteria2.FallbackReason = service.ProtocolServerStatus(model.Hysteria)
	}
	jsonObj(c, status, nil)
}

func (a *ServerController) updateExternalVPNBinary(c *gin.Context) {
	protocol, err := parseExternalVPNProtocol(c.Param("protocol"))
	if err != nil {
		jsonObj(c, gin.H{"updated": false}, err)
		return
	}
	update := func(ctx context.Context) error { return externalvpn.Update(ctx, protocol) }
	if protocol == model.Hysteria {
		base := update
		update = func(ctx context.Context) error { return service.InstallProtocolServer(ctx, protocol, base) }
	}
	if err := update(c.Request.Context()); err != nil {
		jsonObj(c, gin.H{"updated": false}, err)
		return
	}
	jsonObj(c, externalvpn.GetUpdateStatus(c.Request.Context()), nil)
}

func (a *ServerController) parseExternalVPNLink(c *gin.Context) {
	var input struct {
		Link string `json:"link"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		jsonObj(c, nil, err)
		return
	}
	if len(input.Link) > 64<<10 {
		jsonObj(c, nil, fmt.Errorf("connection link too large"))
		return
	}
	parsed, err := link.ParseLink(input.Link)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	jsonObj(c, parsed.Outbound, nil)
}

func (a *ServerController) removeProtocolComponent(c *gin.Context) {
	err := service.RemoveProtocolComponent(c.Request.Context(), c.Param("protocol"))
	jsonObj(c, gin.H{"removed": err == nil}, err)
}
