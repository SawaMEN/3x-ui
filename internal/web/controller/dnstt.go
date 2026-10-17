package controller

import (
	"context"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/dnstt"
	"github.com/gin-gonic/gin"
)

type DNSTTController struct{}

func NewDNSTTController(group *gin.RouterGroup) *DNSTTController {
	c := &DNSTTController{}
	group.GET("/status", c.status)
	group.POST("/install", c.install)
	group.POST("/update", c.update)
	group.POST("/setup", c.setup)
	group.POST("/disable", c.disable)
	group.POST("/remove", c.remove)
	return c
}
func (d *DNSTTController) result(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	s, err := dnstt.StatusNow(ctx)
	jsonObj(c, s, err)
}
func (d *DNSTTController) status(c *gin.Context) { d.result(c) }
func (d *DNSTTController) install(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()
	if err := dnstt.Install(ctx); err != nil {
		jsonMsg(c, "Не удалось установить DNSTT", err)
		return
	}
	d.result(c)
}
func (d *DNSTTController) update(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()
	if err := dnstt.Upgrade(ctx); err != nil {
		jsonMsg(c, "Не удалось обновить DNSTT", err)
		return
	}
	d.result(c)
}
func (d *DNSTTController) setup(c *gin.Context) {
	var settings dnstt.Config
	if err := c.ShouldBindJSON(&settings); err != nil {
		jsonMsg(c, "Проверьте поля DNSTT", err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	if err := dnstt.Setup(ctx, settings); err != nil {
		jsonMsg(c, "Не удалось настроить DNSTT", err)
		return
	}
	d.result(c)
}
func (d *DNSTTController) disable(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if err := dnstt.Disable(ctx); err != nil {
		jsonMsg(c, "Не удалось отключить DNSTT", err)
		return
	}
	d.result(c)
}
func (d *DNSTTController) remove(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if err := dnstt.Remove(ctx); err != nil {
		jsonMsg(c, "Не удалось удалить DNSTT", err)
		return
	}
	d.result(c)
}
