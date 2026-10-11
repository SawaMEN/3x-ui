package hiddify

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func GetBinaryPath() string {
	name := fmt.Sprintf("hiddify-core-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(singbox.GetBinaryPath()), name)
}
func GetConfigPath() string { return filepath.Join(filepath.Dir(GetBinaryPath()), "hiddify-core.json") }

var requiredBuildTags = []string{"with_xui_panel", "with_v2ray_api", "with_awg", "with_quic", "with_wireguard", "with_clash_api", "with_utls", "with_masque"}

func NewProcess(path string, isolated bool) *singbox.Process {
	return singbox.NewProcessWithOptions(path, singbox.ProcessOptions{
		Binary: GetBinaryPath(), RunCommand: "srun", VersionPrefix: "hiddify-core version ",
		RequiredVersion: TargetVersion, RequiredTags: requiredBuildTags,
		RequiredRevision: TargetRevision, Name: "hiddify-core",
		MASQUE: true,
	}, isolated)
}
