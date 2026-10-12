package hiddify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func GetBinaryPath() string { return config.OwnerCoreBinaryPath() }
func GetConfigPath() string { return filepath.Join(filepath.Dir(GetBinaryPath()), "hiddify-core.json") }

var requiredBuildTags = []string{"with_xui_panel", "with_v2ray_api", "with_awg", "with_quic", "with_wireguard", "with_clash_api", "with_utls", "with_masque", "with_gvisor", "with_naive_outbound", "tfogo_checklinkname0"}

// Releases include target.json alongside the binary. Read their actual build
// identity instead of requiring the panel's historical source-lock revision.
func packagedCoreIdentity() (string, string) {
	version, revision := TargetVersion, TargetRevision
	data, err := os.ReadFile(filepath.Join(filepath.Dir(GetBinaryPath()), "target.json"))
	if os.IsNotExist(err) {
		return version, revision // Legacy packages and isolated process tests.
	}
	if err != nil {
		return "invalid-core-manifest", "invalid-core-manifest"
	}
	var manifest struct {
		Target struct {
			Version string `json:"version"`
			Commit string `json:"commit"`
		} `json:"target"`
	}
	if json.Unmarshal(data, &manifest) != nil ||
		!regexp.MustCompile(`^sawamen-[A-Za-z0-9._-]+$`).MatchString(manifest.Target.Version) ||
		!regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(manifest.Target.Commit) {
		return "invalid-core-manifest", "invalid-core-manifest"
	}
	return manifest.Target.Version, manifest.Target.Commit
}

func NewProcess(path string, isolated bool) *singbox.Process {
	version, revision := packagedCoreIdentity()
	return singbox.NewProcessWithOptions(path, singbox.ProcessOptions{
		Binary: GetBinaryPath(), RunCommand: "srun", VersionPrefix: "hiddify-core version ",
		RequiredVersion: version, RequiredTags: requiredBuildTags,
		RequiredAnyTags: []string{"with_purego", "with_musl"},
		RequiredRevision: revision, Name: "sing-box",
		MASQUE: true,
	}, isolated)
}
