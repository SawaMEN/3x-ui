package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/protocolserver"
	"github.com/SawaMEN/3x-ui/v3/internal/sudoku"
)

func RemoveProtocolComponent(ctx context.Context, component string) error {
	var protocol model.Protocol
	var path string
	switch component {
	case "naiveproxy":
		protocol = model.NaiveProxy
	case "hysteria2":
		protocol = model.Hysteria
	case "sudoku":
		protocol = model.Sudoku
		path = sudoku.GetBinaryPath(config.GetBinFolderPath())
	case "pingtunnel":
		protocol = model.Pingtunnel
		path = filepath.Join(config.GetBinFolderPath(), "pingtunnel")
	case "trusttunnel":
		protocol = model.TrustTunnel
		path = filepath.Join(config.GetBinFolderPath(), "trusttunnel_endpoint")
	default:
		return fmt.Errorf("unsupported removable component %q", component)
	}
	if protocol == model.NaiveProxy || protocol == model.Hysteria {
		return mutateProtocolServer(ctx, protocol, func(context.Context) error { return removeComponentFile(protocolserver.BinaryPath(protocol)) }, false)
	}
	protocolInstallMu.Lock()
	defer protocolInstallMu.Unlock()
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	var count int64
	if err := database.GetDB().Model(&model.Inbound{}).Where("protocol = ? AND enable = ? AND node_id IS NULL", protocol, true).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%s is used by enabled inbounds; disable them before removing the component", component)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := removeComponentFile(path); err != nil {
		return err
	}
	if protocol == model.Sudoku {
		sudoku.GetManager().StopAll()
	} else {
		externalvpn.GetManager().StopProtocol(protocol)
	}
	return nil
}

func removeComponentFile(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
