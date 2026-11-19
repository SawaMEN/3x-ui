package controller

import (
 "encoding/json"
 "fmt"

 "github.com/gin-gonic/gin"

 "github.com/SawaMEN/3x-ui/v3/internal/zapret2"
)

func (a *SettingController) zapret2Status(c *gin.Context) {
 status,err:=zapret2.GetStatus(c.Request.Context())
 jsonObj(c,status,err)
}
func (a *SettingController) zapret2Update(c *gin.Context) {
 status,err:=zapret2.Update(c.Request.Context())
 jsonObj(c,status,err)
}
func (a *SettingController) zapret2Remove(c *gin.Context) {
 // Never remove binaries while an outbound template still references them.
 raw,err:=a.settingService.GetXrayConfigTemplate()
 if err!=nil {jsonObj(c,nil,err);return}
 var template struct{Outbounds []struct{Protocol string `json:"protocol"`; Tag string `json:"tag"`} `json:"outbounds"`}
 if err:=json.Unmarshal([]byte(raw),&template);err!=nil{jsonObj(c,nil,err);return}
 for _,outbound:=range template.Outbounds {
  if outbound.Protocol=="zapret2" {
   jsonObj(c,nil,fmt.Errorf("remove zapret2 outbound %q from routing and outbounds before uninstalling the component",outbound.Tag))
   return
  }
 }
 err=zapret2.Remove()
 jsonObj(c,gin.H{"removed":err==nil},err)
}
