package controller

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	webruntime "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"

	"github.com/gin-gonic/gin"
)

type domainActivityResponse struct {
	Enabled                bool                          `json:"enabled"`
	Supported              bool                          `json:"supported"`
	AccessLogEnabled       bool                          `json:"accessLogEnabled"`
	Core                   string                        `json:"core"`
	Mode                   string                        `json:"mode"`
	WindowSeconds          int                           `json:"windowSeconds"`
	Entries                []service.DomainActivityEntry `json:"entries"`
	TemporarilyUnavailable bool                          `json:"temporarilyUnavailable"`
}

// domainActivityCoreMode identifies the source without probing another core.
func domainActivityCoreMode(coreType string) string {
	switch coreType {
	case service.CoreTypeXray:
		return "recent"
	case service.CoreTypeSingBox, service.CoreTypeSingBoxUpstream:
		return "active"
	default:
		return ""
	}
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
	retention, err := a.settingService.GetDomainActivityRetentionSeconds()
	if err != nil {
		return state, err
	}
	state.WindowSeconds = retention
	coreType, err := a.settingService.GetCoreType()
	if err != nil {
		return state, err
	}
	state.Core = coreType
	state.Mode = domainActivityCoreMode(coreType)
	state.Supported = state.Mode != ""
	if coreType == service.CoreTypeXray {
		state.AccessLogEnabled, err = a.settingService.GetAccessLogEnable()
		if err != nil {
			return state, err
		}
	}
	return state, nil
}

// getDomainActivity exposes the configured rolling window of domain metadata.
// Existing panel API authentication, token scopes and CSRF middleware apply.
func (a *ServerController) getDomainActivity(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	nodeID, err := singBoxSessionNodeIDFromQuery(c)
	if err != nil {
		jsonMsg(c, "Invalid domain activity node", err)
		return
	}
	if nodeID != nil {
		// Node-sync credentials may read only their local state; the shared
		// node-target parser rejects any attempt to proxy via another panel.
		manager := webruntime.GetManager()
		if manager == nil {
			jsonMsg(c, "Unable to read node domain activity", errors.New("runtime manager is not initialized"))
			return
		}
		reader, err := manager.DomainActivityReaderFor(nodeID)
		if err != nil {
			jsonMsg(c, "Unable to read node domain activity", err)
			return
		}
		payload, err := reader.DomainActivity(c.Request.Context())
		if err != nil {
			jsonMsg(c, "Unable to read node domain activity", err)
			return
		}
		var remote domainActivityResponse
		if err := json.Unmarshal(payload, &remote); err != nil {
			jsonMsg(c, "Invalid node domain activity response", err)
			return
		}
		if remote.Entries == nil {
			remote.Entries = make([]service.DomainActivityEntry, 0)
		}
		jsonObj(c, remote, nil)
		return
	}
	state, err := a.domainActivityState()
	if err != nil {
		jsonMsg(c, "Cannot read domain activity settings", err)
		return
	}
	if state.Enabled && state.Supported {
		generation := service.DomainActivityHistoryGeneration()
		if state.Mode == "active" {
			// A stopped/restarting core has no live API to query. This is an
			// unavailable snapshot, not an empty set of active connections.
			if !service.SelectedNativeCore().IsRunning() {
				state.TemporarilyUnavailable = true
				state.Entries = service.DomainActivityHistoryEntries(state.Core, time.Now(), state.WindowSeconds)
				jsonObj(c, state, nil)
				return
			}
			now := time.Now()
			state.Entries, err = a.serverService.GetNativeDomainActivity(c.Request.Context(), now)
			if err != nil {
				if singbox.IsTemporaryClashAPIError(err) {
					state.TemporarilyUnavailable = true
					state.Entries = service.DomainActivityHistoryEntries(state.Core, time.Now(), state.WindowSeconds)
					jsonObj(c, state, nil)
					return
				}
				jsonMsgObj(c, "Unable to read sing-box active connections", state, err)
				return
			}
			service.RecordDomainActivityIfGeneration(state.Core, state.Entries, now, state.WindowSeconds, generation)
		} else if state.AccessLogEnabled {
			now := time.Now()
			state.Entries, err = a.serverService.GetRecentDomainActivity(now)
			if err != nil {
				jsonMsgObj(c, "Unable to read Xray domain activity", state, err)
				return
			}
			service.RecordDomainActivityIfGeneration(state.Core, state.Entries, now, state.WindowSeconds, generation)
		}
		state.Entries = service.DomainActivityHistoryEntries(state.Core, time.Now(), state.WindowSeconds)
	}
	jsonObj(c, state, nil)
}

func (a *ServerController) setDomainActivity(c *gin.Context) {
	var form struct {
		Enabled          *bool `json:"enabled"`
		RetentionSeconds *int  `json:"retentionSeconds"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, "Invalid domain activity setting", err)
		return
	}
	if form.Enabled == nil && form.RetentionSeconds == nil {
		jsonMsg(c, "Invalid domain activity setting", errors.New("enabled or retentionSeconds is required"))
		return
	}
	if form.RetentionSeconds != nil && !service.ValidDomainActivityRetentionSeconds(*form.RetentionSeconds) {
		jsonMsg(c, "Invalid domain activity retention", errors.New("retentionSeconds must be between 10 and 86400"))
		return
	}
	if form.Enabled != nil && *form.Enabled {
		state, err := a.domainActivityState()
		if err != nil {
			jsonMsg(c, "Cannot read domain activity settings", err)
			return
		}
		if !state.Supported {
			jsonMsg(c, "Domain activity is unsupported by the selected core", errors.New("unsupported core"))
			return
		}
		if state.Mode == "recent" && !state.AccessLogEnabled {
			jsonMsg(c, "Enable the Xray access log first", errors.New("Xray access log is disabled"))
			return
		}
	}
	if form.RetentionSeconds != nil {
		if err := a.settingService.SetDomainActivityRetentionSeconds(*form.RetentionSeconds); err != nil {
			jsonMsg(c, "Cannot update domain activity retention", err)
			return
		}
	}
	if form.Enabled != nil {
		if err := a.settingService.SetDomainActivityEnabled(*form.Enabled); err != nil {
			jsonMsg(c, "Cannot update domain activity setting", err)
			return
		}
		if !*form.Enabled {
			service.ClearDomainActivityHistory()
		}
	}
	state, err := a.domainActivityState()
	if err == nil && form.Enabled != nil && state.Mode == "active" {
		// Only the monitoring toggle affects native sniffing/routing rules.
		service.SelectedNativeCore().SetToNeedRestart()
	}
	jsonObj(c, state, err)
}
