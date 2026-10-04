package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/gateway"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// Settings have already persisted newCore. The caller serializes Gateway changes.
func (a *SettingController) switchCoreWithGateway(ctx context.Context, oldCore, newCore string) (err error) {
	network := &service.GatewayNetworkService{}
	snapshot := network.Status(ctx)
	rollbackTemplate := func() error { return nil }
	compatibilityChanged, runtimeChanged := false, false
	defer func() {
		if err == nil {
			return
		}
		recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if snapshot.Configured {
			err = errors.Join(err, network.Suspend(recovery))
		}
		// Never start the old engine while the new one still owns its listeners.
		if runtimeChanged {
			err = errors.Join(err, a.stopGatewayCore(recovery, newCore))
		}
		err = errors.Join(err, a.settingService.SetCoreType(oldCore), rollbackTemplate())
		if compatibilityChanged {
			err = errors.Join(err, service.SyncClientCoreCompatibility(newCore, oldCore))
		}
		if runtimeChanged {
			if restartErr := a.startGatewayCore(recovery, oldCore); restartErr != nil {
				err = errors.Join(err, restartErr)
				return
			}
		}
		if snapshot.Configured && snapshot.Error == "" {
			err = errors.Join(err, network.Enable(recovery, snapshot.Config))
		}
	}()
	if snapshot.Error != "" {
		return fmt.Errorf("Gateway recovery required before switching cores: %s", snapshot.Error)
	}
	if err = service.SyncClientCoreCompatibility(oldCore, newCore); err != nil {
		return err
	}
	compatibilityChanged = true
	if snapshot.Configured {
		if err = network.Suspend(ctx); err != nil {
			return err
		}
	}
	if rollbackTemplate, err = gateway.MigrateCore(oldCore, newCore); err != nil {
		rollbackTemplate = func() error { return nil }
		return err
	}
	// Mark before stopping: even a failed Stop can change the old runtime.
	runtimeChanged = true
	if err = a.stopGatewayCore(ctx, oldCore); err != nil {
		return err
	}
	if err = a.startGatewayCore(ctx, newCore); err != nil {
		return err
	}
	if snapshot.Configured {
		err = network.Enable(ctx, snapshot.Config)
	}
	return err
}
func (a *SettingController) stopGatewayCore(ctx context.Context, core string) error {
	if core == service.CoreTypeSingBox {
		return a.singBoxService.Stop(ctx)
	}
	return a.xrayService.StopXray()
}
func (a *SettingController) startGatewayCore(ctx context.Context, core string) error {
	if core == service.CoreTypeSingBox {
		return a.singBoxService.Restart(ctx)
	}
	return a.xrayService.RestartXray(true)
}
