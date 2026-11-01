// Package xray retains legacy configuration and geodata wire compatibility.
// It does not launch or manage an Xray process.
package xray

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/xtls/xray-core/infra/conf"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

func ValidateOutboundConfig(outbound []byte) error {
	ensureXrayAssetLocation()

	detour := new(conf.OutboundDetourConfig)
	if err := json.Unmarshal(outbound, detour); err != nil {
		return err
	}
	_, err := detour.Build()
	return err
}

func IsMissingHandlerErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "not enough information")
}

func ensureXrayAssetLocation() {
	if os.Getenv("XRAY_LOCATION_ASSET") != "" || os.Getenv("xray.location.asset") != "" {
		return
	}
	if abs, err := filepath.Abs(config.GetBinFolderPath()); err == nil {
		os.Setenv("XRAY_LOCATION_ASSET", abs)
	}
}
