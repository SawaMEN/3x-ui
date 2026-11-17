package controller

import (
	"context"
	"fmt"
	"runtime"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/naiveproxy"
	"github.com/SawaMEN/3x-ui/v3/internal/protocolserver"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type NaiveProxyController struct{}

func NewNaiveProxyController(g *gin.RouterGroup) *NaiveProxyController {
	a := &NaiveProxyController{}
	group := g.Group("/naiveproxy")
	group.GET("/status", a.status)
	group.POST("/update", a.update)
	return a
}

func (a *NaiveProxyController) standaloneAllowed() error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("official Caddy-Naive server release supports linux/amd64 only; current platform is %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return nil
}

func (a *NaiveProxyController) status(c *gin.Context) {
	status, err := naiveproxy.GetStatus(c.Request.Context())
	status.Running = protocolserver.Running(model.NaiveProxy)
	status.Source, status.FallbackReason = service.ProtocolServerStatus(model.NaiveProxy)
	jsonObj(c, status, err)
}

func (a *NaiveProxyController) update(c *gin.Context) {
	if err := a.standaloneAllowed(); err != nil {
		jsonObj(c, naiveproxy.Status{}, err)
		return
	}
	err := service.InstallProtocolServer(c.Request.Context(), model.NaiveProxy, func(ctx context.Context) error { _, err := naiveproxy.Update(ctx); return err })
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	a.status(c)
}
