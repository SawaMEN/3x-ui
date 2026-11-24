package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/zapret2"
)

func (a *SettingController) zapret2Status(c *gin.Context) {
	status, err := zapret2.GetStatus(c.Request.Context())
	jsonObj(c, status, err)
}

func (a *SettingController) zapret2Update(c *gin.Context) {
	status, err := zapret2.Update(c.Request.Context())
	jsonObj(c, status, err)
}

func (a *SettingController) zapret2Diagnose(c *gin.Context) {
	var payload zapret2.DiagnosticOptions
	if err := c.ShouldBindJSON(&payload); err != nil {
		jsonObj(c, nil, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 31*time.Minute)
	defer cancel()
	result, err := zapret2.RunBlockcheck(ctx, payload)
	jsonObj(c, result, err)
}

func (a *SettingController) zapret2Remove(c *gin.Context) {
	if zapret2.Running() {
		jsonObj(c, nil, fmt.Errorf("stop all active zapret2 outbounds before uninstalling"))
		return
	}
	// Never remove binaries while an outbound template still references them.
	raw, err := a.settingService.GetXrayConfigTemplate()
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	templates := []string{raw}
	for _, core := range []string{service.CoreTypeSingBox, service.CoreTypeSingBoxUpstream} {
		native, err := a.settingService.GetNativeCoreConfigTemplate(core)
		if err != nil {
			jsonObj(c, nil, err)
			return
		}
		templates = append(templates, native)
	}
	for _, raw := range templates {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var template struct {
			Outbounds []struct {
				Protocol string `json:"protocol"`
				Type     string `json:"type"`
				Tag      string `json:"tag"`
			} `json:"outbounds"`
		}
		if err := json.Unmarshal([]byte(raw), &template); err != nil {
			jsonObj(c, nil, err)
			return
		}
		for _, outbound := range template.Outbounds {
			if outbound.Protocol == "zapret2" || outbound.Type == "zapret2" {
				jsonObj(c, nil, fmt.Errorf("remove zapret2 outbound %q from routing and outbounds before uninstalling the component", outbound.Tag))
				return
			}
		}
	}
	err = zapret2.Remove()
	jsonObj(c, gin.H{"removed": err == nil}, err)
}
