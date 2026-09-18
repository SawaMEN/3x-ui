package controller

import (
  "net/http"
  "github.com/mhsanaei/3x-ui/v3/internal/web/service"
  "github.com/gin-gonic/gin"
)

type TelemtController struct { service service.TelemtService }

func NewTelemtController(g *gin.RouterGroup) *TelemtController {
  a:=&TelemtController{}
  g.GET("/status", a.status)
  g.GET("/config", a.config)
  g.POST("/config", a.saveConfig)
  g.POST("/action", a.action)
  return a
}

func (a *TelemtController) status(c *gin.Context) { jsonObj(c, a.service.Status(), nil) }

func (a *TelemtController) config(c *gin.Context) {
  cfg,err:=a.service.GetConfig()
  if err!=nil { jsonMsg(c, "failed to read Telemt config", err); return }
  jsonObj(c,cfg,nil)
}

func (a *TelemtController) saveConfig(c *gin.Context) {
  var cfg service.TelemtConfig
  if err:=c.ShouldBindJSON(&cfg); err!=nil { jsonMsg(c,"invalid Telemt configuration",err); return }
  if err:=a.service.SaveConfig(cfg); err!=nil { jsonMsg(c,err.Error(),err); return }
  jsonObj(c,a.service.Status(),nil)
}

func (a *TelemtController) action(c *gin.Context) {
  var req struct { Action string `json:"action"` }
  if err:=c.ShouldBindJSON(&req); err!=nil { jsonMsg(c,"invalid Telemt action",err); return }
  if err:=a.service.Apply(req.Action); err!=nil { jsonMsg(c,err.Error(),err); return }
  c.JSON(http.StatusOK,gin.H{"success":true,"obj":a.service.Status()})
}
