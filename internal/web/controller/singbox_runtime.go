package controller

import (
	"errors"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// SingBoxRuntimeController exposes live native sing-box runtime state without
// coupling it to the settings controller.
type SingBoxRuntimeController struct {
	singBoxService service.SingBoxService
}

func NewSingBoxRuntimeController(g *gin.RouterGroup) *SingBoxRuntimeController {
	a := &SingBoxRuntimeController{}
	g.GET("/singbox/connections", a.connections)
	g.POST("/singbox/connections/:id/close", a.closeConnection)
	return a
}

func (a *SingBoxRuntimeController) connections(c *gin.Context) {
	if !a.singBoxService.IsRunning() {
		jsonObj(c, []any{}, nil)
		return
	}
	connections, err := a.singBoxService.Connections(
		c.Request.Context(), strings.TrimSpace(c.Query("resource")), strings.TrimSpace(c.Query("tag")),
	)
	jsonObj(c, connections, err)
}

func (a *SingBoxRuntimeController) closeConnection(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("connection id is required"))
		return
	}
	if err := a.singBoxService.CloseConnection(c.Request.Context(), id); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"closed": true}, nil)
}
