package controller

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
)

func (a *SettingController) singBoxConnections(c *gin.Context) {
	if !a.singBoxService.IsRunning() {
		jsonObj(c, []any{}, nil)
		return
	}
	connections, err := a.singBoxService.Connections(
		c.Request.Context(),
		strings.TrimSpace(c.Query("resource")),
		strings.TrimSpace(c.Query("tag")),
	)
	jsonObj(c, connections, err)
}

func (a *SettingController) closeSingBoxConnection(c *gin.Context) {
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
