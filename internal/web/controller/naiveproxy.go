package controller

import (
	"github.com/SawaMEN/3x-ui/v3/internal/naiveproxy"
	"github.com/gin-gonic/gin"
)

type NaiveProxyController struct{}

func NewNaiveProxyController(g *gin.RouterGroup) *NaiveProxyController {
	a := &NaiveProxyController{}
	group := g.Group("/naiveproxy")
	group.GET("/status", a.status)
	group.POST("/update", a.update)
	return a
}

func (a *NaiveProxyController) status(c *gin.Context) {
	status, err := naiveproxy.GetStatus(c.Request.Context())
	jsonObj(c, status, err)
}

func (a *NaiveProxyController) update(c *gin.Context) {
	status, err := naiveproxy.Update(c.Request.Context())
	jsonObj(c, status, err)
}
