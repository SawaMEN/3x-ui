package controller

import (

 "github.com/SawaMEN/3x-ui/v3/internal/olcrtc"
 "github.com/gin-gonic/gin"
)

// OlcRTC is a separate WebRTC service, not a sing-box inbound. All endpoints
// inherit the panel's session/token and CSRF middleware.
func NewOlcRTCController(g *gin.RouterGroup) {
 group:=g.Group("/olcrtc")
 group.GET("/status",func(c *gin.Context){jsonObj(c,olcrtc.GetStatus(c.Request.Context()),nil)})
 group.POST("/install",func(c *gin.Context){jsonObj(c,gin.H{"started":true},olcrtc.Start("install"))})
 group.POST("/update",func(c *gin.Context){jsonObj(c,gin.H{"started":true},olcrtc.Start("update"))})
 group.POST("/remove",func(c *gin.Context){jsonObj(c,gin.H{"started":true},olcrtc.Start("remove"))})
 group.GET("/clients",func(c *gin.Context){
  clients,err:=olcrtc.Clients()
  if err!=nil {jsonObj(c,nil,err);return}
  result:=make([]gin.H,0,len(clients))
  for _,client:=range clients {
   result=append(result,gin.H{"id":client.ID,"locations":client.Locations,"link":olcrtc.ClientLink(client)})
  }
  jsonObj(c,result,nil)
 })
 group.POST("/clients",func(c *gin.Context){
  var input olcrtc.ClientInput
  if err:=c.ShouldBindJSON(&input);err!=nil {jsonObj(c,nil,err);return}
  obj,err:=olcrtc.CreateClient(c.Request.Context(),input)
  jsonObj(c,obj,err)
 })
 updateClient:=func(c *gin.Context){
  var input olcrtc.ClientInput
  if err:=c.ShouldBindJSON(&input);err!=nil {jsonObj(c,nil,err);return}
  obj,err:=olcrtc.UpdateClient(c.Request.Context(),c.Param("id"),input)
  jsonObj(c,obj,err)
 }
 group.PUT("/clients/:id",updateClient)
 group.POST("/clients/:id/update",updateClient)
 group.POST("/clients/:id/delete",func(c *gin.Context){
  err:=olcrtc.DeleteClient(c.Request.Context(),c.Param("id"))
  jsonObj(c,gin.H{"removed":err==nil},err)
 })
 group.POST("/clients/:id/rotate-key",func(c *gin.Context){
  err:=olcrtc.RotateKey(c.Request.Context(),c.Param("id"))
  jsonObj(c,gin.H{"rotated":err==nil},err)
 })
 // Deliberately no public proxy to the upstream administrator UI; that UI
 // binds only 127.0.0.1 and the credentials never reach the browser.
}
