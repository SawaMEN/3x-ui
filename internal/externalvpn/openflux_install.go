package externalvpn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

const openFluxReleaseAPI = "https://api.github.com/repos/p1neappleXpress/OpenFlux/releases/latest"

type OpenFluxInstallStatus struct {
	RunID string `json:"runId"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

type openFluxInstaller struct {
	mu      sync.Mutex
	status  OpenFluxInstallStatus
	install func(context.Context) error
}

var openFluxInstallJob = &openFluxInstaller{install: InstallOpenFlux}

// Installation is detached from a short HTTP request and bounded independently.
// Repeated clicks attach to the same job instead of queuing more downloads.
func StartOpenFluxInstallation() OpenFluxInstallStatus { return openFluxInstallJob.start() }
func GetOpenFluxInstallation() OpenFluxInstallStatus   { return openFluxInstallJob.read() }

func (j *openFluxInstaller) read() OpenFluxInstallStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	status := j.status
	if status.State == "" {
		status.State = "idle"
	}
	return status
}

func (j *openFluxInstaller) start() OpenFluxInstallStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.status.State == "running" {
		return j.status
	}
	j.status = OpenFluxInstallStatus{RunID: fmt.Sprint(time.Now().UnixNano()), State: "running"}
	result := j.status
	go func() {
		var failure error
		defer func() {
			if recover() != nil {
				failure = fmt.Errorf("OpenFlux installer failed unexpectedly")
			}
			j.mu.Lock()
			defer j.mu.Unlock()
			j.status.State = "success"
			if failure != nil {
				j.status.State, j.status.Error = "failed", failure.Error()
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		failure = j.install(ctx)
	}()
	return result
}

type openFluxInstallReceipt struct {
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

func writeOpenFluxReceipt(version, digest string) error {
	data, err := json.Marshal(openFluxInstallReceipt{Version: version, Digest: digest})
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(config.GetBinFolderPath(), ".openflux-install.json"), data)
}

// Upstream has no version flag. Only claim a version when the installed bytes
// still match the digest recorded by our verified installer.
func OpenFluxBinaryStatus(ctx context.Context, localOnly ...bool) BinaryStatus {
	status := BinaryStatus{
		Supported: runtime.GOOS == "linux" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"),
	}
	if !status.Supported {
		status.UnsupportedReason = "OpenFlux installation supports Linux amd64/arm64"
		return status
	}
	path := binary(model.OpenFlux)
	info, err := os.Stat(path)
	status.Installed = err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
	if status.Installed {
		var receipt openFluxInstallReceipt
		data, err := os.ReadFile(filepath.Join(config.GetBinFolderPath(), ".openflux-install.json"))
		if err == nil && json.Unmarshal(data, &receipt) == nil && receipt.Version != "" &&
			len(receipt.Digest) == len("sha256:")+64 && verifyReleaseDigest(path, receipt.Digest) == nil {
			status.Version = receipt.Version
		}
	}
	// Saving a configuration only reads local state. Checking components for
	// updates uses the official latest stable release, even before installation.
	if len(localOnly) > 0 && localOnly[0] {
		return status
	}
	spec := releaseSpec{protocol: model.OpenFlux, api: openFluxReleaseAPI, binaryName: "openflux"}
	release, err := fetchLatestRelease(ctx, spec)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	if _, err := openFluxReleaseAsset(release, runtime.GOARCH); err != nil {
		status.Supported = false
		status.UnsupportedReason = err.Error()
		return status
	}
	status.LatestVersion = normalizeReleaseVersion(release.TagName)
	status.UpdateAvailable = status.Installed && status.Version != status.LatestVersion
	return status
}

func openFluxReleaseAsset(release githubRelease, arch string) (releaseAsset, error) {
	if arch != "amd64" && arch != "arm64" {
		return releaseAsset{}, fmt.Errorf("OpenFlux installation supports Linux amd64/arm64")
	}
	name := "openflux-linux-" + arch
	for _, asset := range release.Assets {
		if asset.Name != name {
			continue
		}
		if !strings.HasPrefix(asset.URL, "https://github.com/p1neappleXpress/OpenFlux/releases/download/") ||
			!strings.HasSuffix(asset.URL, "/"+name) ||
			!strings.HasPrefix(asset.Digest, "sha256:") || len(asset.Digest) != len("sha256:")+64 {
			return releaseAsset{}, fmt.Errorf("OpenFlux release has no digest-verified official %s asset", name)
		}
		return asset, nil
	}
	return releaseAsset{}, fmt.Errorf("OpenFlux release has no %s asset", name)
}

// Context is also passed as an exact CLI argument. Do not let INI comment
// syntax truncate legacy contexts such as OpenFlux's "http://#" placeholder.
func openFluxINIContext(value string) string {
	if strings.ContainsAny(value, "#;") {
		return ""
	}
	return value
}

func RemoveOpenFlux(ctx context.Context) error {
	externalVPNUpdateMu.Lock()
	defer externalVPNUpdateMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	outboundMu.Lock()
	inUse := false
	for _, plans := range []map[string]*managedOutbound{outboundPlans, outboundProcesses} {
		for _, plan := range plans {
			protocol, _ := plan.raw["protocol"].(string)
			inUse = inUse || strings.EqualFold(protocol, "openflux")
		}
	}
	outboundMu.Unlock()
	if inUse {
		return fmt.Errorf("OpenFlux is used by outbound routes; remove those routes before uninstalling")
	}
	GetManager().StopProtocol(model.OpenFlux)
	for _, name := range []string{"openflux", ".openflux-install.json"} {
		if err := os.Remove(filepath.Join(config.GetBinFolderPath(), name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Keep the previous executable and its receipt intact if committing either
// part fails. Only a verified, launch-compatible staged binary reaches here.
func commitOpenFluxBinary(staged, version, digest string) error {
	dir := config.GetBinFolderPath()
	target := filepath.Join(dir, "openflux")
	backup, err := os.CreateTemp(dir, ".openflux-previous-*")
	if err != nil {
		return err
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(backupPath)
		return err
	}
	defer func() {
		if backupPath != "" {
			_ = os.Remove(backupPath)
		}
	}()
	previous := true
	if err := os.Rename(target, backupPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		previous = false
	}
	restore := func() error {
		if previous {
			if err := os.Rename(backupPath, target); err != nil {
				retained := backupPath
				backupPath = ""
				return fmt.Errorf("restore previous OpenFlux from %s: %w", retained, err)
			}
			return nil
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.Rename(staged, target); err != nil {
		return errors.Join(err, restore())
	}
	if err := writeOpenFluxReceipt(version, digest); err != nil {
		return errors.Join(err, restore())
	}
	return nil
}
