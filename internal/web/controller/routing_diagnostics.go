package controller

import (
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/web/session"
	"github.com/gin-gonic/gin"
)

// testRoute asks the live Xray router which outbound would handle the
// synthetic connection. This does not send traffic to the destination.
func (a *XraySettingController) testRoute(c *gin.Context) {
	if session.GetLoginUser(c) == nil {
		jsonObj(c, nil, fmt.Errorf("not authenticated"))
		return
	}
	var request service.RouteTestRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonObj(c, nil, err)
		return
	}
	result, err := service.EvaluateXrayRoute(c.Request.Context(), request)
	jsonObj(c, result, err)
}
