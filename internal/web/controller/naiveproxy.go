package controller

import (
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/naiveproxy"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/gin-gonic/gin"
)

type NaiveProxyController struct {
	settingService service.SettingService
}

func NewNaiveProxyController(g *gin.RouterGroup) *NaiveProxyController {
	a := &NaiveProxyController{}
	group := g.Group("/naiveproxy")
	group.GET("/status", a.status)
	group.POST("/update", a.update)
	return a
}

func (a *NaiveProxyController) standaloneAllowed() error {
	coreType, err := a.settingService.GetCoreType()
	if err != nil {
		return err
	}
	if coreType != service.CoreTypeXray {
		return fmt.Errorf("standalone NaiveProxy is available only when Xray is selected")
	}
	return nil
}

func (a *NaiveProxyController) status(c *gin.Context) {
	if err := a.standaloneAllowed(); err != nil {
		jsonObj(c, naiveproxy.Status{}, err)
		return
	}
	status, err := naiveproxy.GetStatus(c.Request.Context())
	jsonObj(c, status, err)
}

func (a *NaiveProxyController) update(c *gin.Context) {
	if err := a.standaloneAllowed(); err != nil {
		jsonObj(c, naiveproxy.Status{}, err)
		return
	}
	status, err := naiveproxy.Update(c.Request.Context())
	jsonObj(c, status, err)
}
