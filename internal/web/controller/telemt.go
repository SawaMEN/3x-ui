package controller

import (
  "net"
  "net/http"
  "strings"

  "github.com/gin-gonic/gin"
  "github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

type TelemtController struct { service service.TelemtService }

func NewTelemtController(g *gin.RouterGroup) *TelemtController {
  a:=&TelemtController{}
  g.GET("/status", a.status)
  g.GET("/config", a.config)
  g.POST("/config", a.saveConfig)
  g.GET("/proxy", a.listProxy)
  g.POST("/proxy", a.createProxy)
  g.DELETE("/proxy/:name", a.deleteProxy)
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

func (a *TelemtController) listProxy(c *gin.Context) {
  proxies, err := a.service.ListProxies()
  if err != nil { jsonMsg(c, "failed to read Telemt proxies", err); return }
  jsonObj(c, proxies, nil)
}

func publicHostFromRequest(c *gin.Context) string {
  host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
  if i := strings.IndexByte(host, ','); i >= 0 { host = strings.TrimSpace(host[:i]) }
  if host == "" { host = strings.TrimSpace(c.Request.Host) }
  if h, _, err := net.SplitHostPort(host); err == nil { return strings.Trim(h, "[]") }
  return strings.Trim(host, "[]")
}

func (a *TelemtController) createProxy(c *gin.Context) {
  var req service.TelemtCreateRequest
  if err := c.ShouldBindJSON(&req); err != nil { jsonMsg(c, "invalid Telemt proxy request", err); return }
  req.Host = publicHostFromRequest(c)
  if req.Host == "" { jsonMsg(c, "unable to determine public host", nil); return }
  proxy, err := a.service.CreateProxy(req)
  if err != nil { jsonMsg(c, err.Error(), err); return }
  jsonObj(c, proxy, nil)
}

func (a *TelemtController) deleteProxy(c *gin.Context) {
  if err := a.service.DeleteProxy(c.Param("name")); err != nil { jsonMsg(c, err.Error(), err); return }
  jsonObj(c, a.service.Status(), nil)
}

func (a *TelemtController) action(c *gin.Context) {
  var req struct { Action string `json:"action"` }
  if err:=c.ShouldBindJSON(&req); err!=nil { jsonMsg(c,"invalid Telemt action",err); return }
  if err:=a.service.Apply(req.Action); err!=nil { jsonMsg(c,err.Error(),err); return }
  c.JSON(http.StatusOK,gin.H{"success":true,"obj":a.service.Status()})
}
