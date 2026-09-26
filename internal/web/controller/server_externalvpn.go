package controller

import (
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"

	"github.com/gin-gonic/gin"
)

func (a *ServerController) initExternalVPNRouter(g *gin.RouterGroup) {
	g.GET("/externalvpn/status", a.getExternalVPNUpdateStatus)
	g.POST("/externalvpn/update/:protocol", a.updateExternalVPNBinary)
}

func parseExternalVPNProtocol(value string) (model.Protocol, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(model.Pingtunnel):
		return model.Pingtunnel, nil
	case string(model.TrustTunnel):
		return model.TrustTunnel, nil
	default:
		return "", fmt.Errorf("unsupported external VPN component %q", value)
	}
}

func (a *ServerController) getExternalVPNUpdateStatus(c *gin.Context) {
	jsonObj(c, externalvpn.GetUpdateStatus(c.Request.Context()), nil)
}

func (a *ServerController) updateExternalVPNBinary(c *gin.Context) {
	protocol, err := parseExternalVPNProtocol(c.Param("protocol"))
	if err != nil {
		jsonObj(c, gin.H{"updated": false}, err)
		return
	}
	if err := externalvpn.Update(c.Request.Context(), protocol); err != nil {
		jsonObj(c, gin.H{"updated": false}, err)
		return
	}
	jsonObj(c, externalvpn.GetUpdateStatus(c.Request.Context()), nil)
}
