package controller

import (
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/web/session"
	"github.com/gin-gonic/gin"
)

type nodeRouteEnsureRequest struct {
	InboundID int    `json:"inboundId"`
	Core      string `json:"core"`
}

func (a *XraySettingController) getNodeRouteTargets(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		jsonObj(c, nil, fmt.Errorf("not authenticated"))
		return
	}
	core := strings.TrimSpace(c.Query("core"))
	if core != "xray" && core != "sing-box" {
		jsonObj(c, nil, fmt.Errorf("invalid route core"))
		return
	}
	targets, err := service.NodeRouteTargets(user.Id, core)
	jsonObj(c, targets, err)
}

func (a *XraySettingController) ensureNodeRoute(c *gin.Context) {
	user := session.GetLoginUser(c)
	if user == nil {
		jsonObj(c, nil, fmt.Errorf("not authenticated"))
		return
	}
	var payload nodeRouteEnsureRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		jsonObj(c, nil, err)
		return
	}
	if payload.Core != "xray" && payload.Core != "sing-box" {
		jsonObj(c, nil, fmt.Errorf("invalid route core"))
		return
	}
	tag, needRestart, err := service.EnsureNodeRoute(user.Id, payload.InboundID, payload.Core)
	if needRestart {
		markSelectedCoreNeedRestart(&a.SettingService, &a.CoreService, &service.SingBoxService{})
	}
	if needRestart || err == nil {
		notifyClientsChanged()
	}
	jsonObj(c, map[string]string{"tag": tag}, err)
}
