package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/core"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/sudoku"
	"github.com/SawaMEN/3x-ui/v3/internal/vkturnproxy"
)

func ProtocolAvailability(kind, version string, installed bool) map[string]model.ProtocolAvailability {
	result := map[string]model.ProtocolAvailability{}
	for protocol, capability := range model.InboundProtocolCapabilities {
		supported := capability.SupportsCore(core.Type(kind))
		ready := installed
		component := capability.Component
		if component != "" {
			componentPath := filepath.Join(config.GetBinFolderPath(), component)
			switch protocol {
			case model.MTProto:
				componentPath = mtproto.GetBinaryPath()
			case model.Mieru:
				componentPath = mieru.GetBinaryPath()
			case model.Sudoku:
				componentPath = sudoku.GetBinaryPath(config.GetBinFolderPath())
			case model.VKTurnProxy:
				componentPath = vkturnproxy.GetBinaryPath()
			case model.VetroffProxy:
				componentPath = filepath.Join(config.GetBinFolderPath(), "vetroff-bundle", "vetroff-necko", "vetroff")
			default:
				if path, err := exec.LookPath(component); err == nil {
					componentPath = path
				}
			}
			if runtime.GOOS == "windows" && filepath.Ext(componentPath) == "" {
				componentPath += ".exe"
			}
			info, err := os.Stat(componentPath)
			ready = err == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0)
		}
		if protocol == model.MASQUE && IsNativeCore(kind) {
			supported = supported && NativeCore(kind).process().SupportsMASQUE()
		}
		componentVersion := version
		if component != "" {
			componentVersion = ""
		}
		result[string(protocol)] = model.ProtocolAvailability{Supported: supported, Ready: supported && ready, Version: componentVersion, SpeedLimit: capability.SpeedLimit, Component: component}
	}
	return result
}
