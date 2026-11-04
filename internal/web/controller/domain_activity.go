package controller

import (
	"errors"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/gin-gonic/gin"
)

type domainActivityResponse struct {
	Enabled          bool                          `json:"enabled"`
	Supported        bool                          `json:"supported"`
	AccessLogEnabled bool                          `json:"accessLogEnabled"`
	WindowSeconds    int                           `json:"windowSeconds"`
	Entries          []service.DomainActivityEntry `json:"entries"`
}

func (a *ServerController) domainActivityState() (domainActivityResponse, error) {
	state := domainActivityResponse{
		WindowSeconds: service.DomainActivityWindowSeconds,
		Entries:       make([]service.DomainActivityEntry, 0),
	}
	enabled, err := a.settingService.GetDomainActivityEnabled()
	if err != nil {
		return state, err
	}
	state.Enabled = enabled
	coreType, err := a.settingService.GetCoreType()
	if err != nil {
		return state, err
	}
	state.Supported = coreType == service.CoreTypeXray
	if state.Supported {
		state.AccessLogEnabled, err = a.settingService.GetAccessLogEnable()
		if err != nil {
			return state, err
		}
	}
	return state, nil
}

// getDomainActivity exposes only the last two minutes of domain metadata.
// Existing panel API authentication, token scopes and CSRF middleware apply.
func (a *ServerController) getDomainActivity(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	state, err := a.domainActivityState()
	if err != nil {
		jsonMsg(c, "Cannot read domain activity settings", err)
		return
	}
	if state.Enabled && state.Supported && state.AccessLogEnabled {
		state.Entries = a.serverService.GetRecentDomainActivity(time.Now())
	}
	jsonObj(c, state, nil)
}

func (a *ServerController) setDomainActivity(c *gin.Context) {
	var form struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "Invalid domain activity setting", err)
		return
	}
	if form.Enabled == nil {
		jsonMsg(c, "Invalid domain activity setting", errors.New("enabled is required"))
		return
	}
	if *form.Enabled {
		state, err := a.domainActivityState()
		if err != nil {
			jsonMsg(c, "Cannot read domain activity settings", err)
			return
		}
		if !state.Supported {
			jsonMsg(c, "Domain activity requires the Xray core", errors.New("unsupported core"))
			return
		}
		if !state.AccessLogEnabled {
			jsonMsg(c, "Enable the Xray access log first", errors.New("Xray access log is disabled"))
			return
		}
	}
	if err := a.settingService.SetDomainActivityEnabled(*form.Enabled); err != nil {
		jsonMsg(c, "Cannot update domain activity setting", err)
		return
	}
	state, err := a.domainActivityState()
	jsonObj(c, state, err)
}
