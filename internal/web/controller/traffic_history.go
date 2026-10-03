package controller

import (
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/gin-gonic/gin"
)

var trafficHistoryBuckets = map[string]int64{"5m":300,"15m":900,"30m":1800,"1h":3600,"6h":21600,"12h":43200,"1d":86400}

type TrafficHistoryController struct{}
func NewTrafficHistoryController(g *gin.RouterGroup)*TrafficHistoryController{a:=&TrafficHistoryController{};g.GET("/trafficHistory/:resource/:bucket",a.get);return a}
func (a *TrafficHistoryController)get(c *gin.Context){
	resource:=strings.ToLower(strings.TrimSpace(c.Param("resource"))); tag:=strings.TrimSpace(c.Query("tag")); name:=strings.ToLower(strings.TrimSpace(c.Param("bucket"))); bucket,ok:=trafficHistoryBuckets[name]
	if !ok{jsonMsg(c,"Invalid traffic history bucket",strconv.ErrSyntax);return}
	limit:=360; if raw:=strings.TrimSpace(c.Query("limit"));raw!=""{if n,err:=strconv.Atoi(raw);err==nil&&n>0{limit=n}}
	points,err:=service.GetTrafficHistory(resource,tag,bucket,limit);jsonObj(c,gin.H{"resource":resource,"tag":tag,"bucket":name,"points":points},err)
}
