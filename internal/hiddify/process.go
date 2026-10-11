package hiddify

import (
	"path/filepath"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func GetBinaryPath() string { return config.OwnerCoreBinaryPath() }
func GetConfigPath() string { return filepath.Join(filepath.Dir(GetBinaryPath()), "hiddify-core.json") }

var requiredBuildTags = []string{"with_xui_panel", "with_v2ray_api", "with_awg", "with_quic", "with_wireguard", "with_clash_api", "with_utls", "with_masque", "with_gvisor", "with_naive_outbound", "with_purego", "tfogo_checklinkname0"}

func NewProcess(path string, isolated bool) *singbox.Process {
	return singbox.NewProcessWithOptions(path, singbox.ProcessOptions{
		Binary: GetBinaryPath(), RunCommand: "srun", VersionPrefix: "hiddify-core version ",
		RequiredVersion: TargetVersion, RequiredTags: requiredBuildTags,
		RequiredRevision: TargetRevision, Name: "sing-box",
		MASQUE: true,
	}, isolated)
}
