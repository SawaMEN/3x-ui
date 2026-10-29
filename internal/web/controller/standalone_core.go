package controller

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/gateway"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// Serialize installs, selections and settings-based core switches in this process.
var coreManagementMu sync.Mutex

func (a *SettingController) coreStatuses(c *gin.Context) {
	selected, err := a.settingService.GetCoreType()
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	statuses := []gin.H{}
	for _, kind := range []string{service.CoreTypeSingBox, service.CoreTypeSingBoxUpstream, service.CoreTypeXray} {
		svc := service.NativeCore(kind)
		version := ""
		if svc.Installed() {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
			version, _ = svc.Version(ctx)
			cancel()
		}
		statuses = append(statuses, gin.H{"type": kind, "installed": svc.Installed(), "running": svc.IsRunning(), "version": version, "selected": selected == kind})
	}
	jsonObj(c, gin.H{"selected": selected, "cores": statuses}, nil)
}

func (a *SettingController) installStandaloneCore(c *gin.Context) {
	coreManagementMu.Lock()
	defer coreManagementMu.Unlock()
	var form struct {
		Version string `json:"version" form:"version"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBind(&form); err != nil {
			jsonObj(c, nil, err)
			return
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	version, err := service.InstallStandaloneCore(ctx, c.Param("core"), form.Version)
	jsonObj(c, gin.H{"version": version}, err)
}

func (a *SettingController) selectStandaloneCore(c *gin.Context) {
	coreManagementMu.Lock()
	defer coreManagementMu.Unlock()
	target := c.Param("core")
	if target != service.CoreTypeSingBox && target != service.CoreTypeSingBoxUpstream && target != service.CoreTypeXray {
		jsonObj(c, nil, fmt.Errorf("unsupported core %q", target))
		return
	}
	if !service.NativeCore(target).Installed() {
		jsonObj(c, nil, fmt.Errorf("install %s first", target))
		return
	}
	old, err := a.settingService.GetCoreType()
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if old == target {
		if !service.NativeCore(target).IsRunning() {
			err = service.NativeCore(target).Restart(c.Request.Context())
		}
		jsonObj(c, gin.H{"selected": target}, err)
		return
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		release, err := gateway.AcquireOperation(c.Request.Context())
		if err != nil {
			jsonObj(c, nil, err)
			return
		}
		defer release()
	}
	finishSwitch := service.BeginCoreSwitch()
	defer finishSwitch()
	if err = a.settingService.SetCoreType(target); err == nil {
		err = a.switchCoreWithGateway(c.Request.Context(), old, target)
	}
	jsonObj(c, gin.H{"selected": target}, err)
}

func (a *SettingController) deleteStandaloneCore(c *gin.Context) {
	coreManagementMu.Lock()
	defer coreManagementMu.Unlock()
	err := service.UninstallStandaloneCore(c.Request.Context(), c.Param("core"))
	jsonObj(c, gin.H{"removed": err == nil}, err)
}

func (a *SettingController) updateStandaloneCore(c *gin.Context) {
	coreManagementMu.Lock()
	defer coreManagementMu.Unlock()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	var form struct {
		Version string `json:"version" form:"version"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBind(&form); err != nil {
			jsonObj(c, nil, err)
			return
		}
	}
	version, err := service.UpdateStandaloneCore(ctx, c.Param("core"), form.Version)
	jsonObj(c, gin.H{"version": version}, err)
}

// Versions are fetched from the same upstream repository used by the installer.
func (a *SettingController) standaloneCoreVersions(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	versions, err := service.StandaloneCoreVersions(ctx, c.Param("core"))
	jsonObj(c, versions, err)
}
