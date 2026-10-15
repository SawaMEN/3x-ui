package sudoku

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

type Status struct {
	Supported         bool   `json:"supported"`
	UnsupportedReason string `json:"unsupportedReason,omitempty"`
	Installed         bool   `json:"installed"`
	Version           string `json:"version"`
	LatestVersion     string `json:"latestVersion"`
	UpdateAvailable   bool   `json:"updateAvailable"`
}

func GetStatus(ctx context.Context, localOnly ...bool) (Status, error) {
	binDir := config.GetBinFolderPath()
	status := Status{
		Supported: runtime.GOOS == "linux" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"),
		Installed: IsInstalled(binDir),
		Version:   readInstalledVersion(binDir),
	}
	if !status.Supported {
		status.Installed = false
		status.UnsupportedReason = fmt.Sprintf("Sudoku is not published for %s/%s", runtime.GOOS, runtime.GOARCH)
		return status, nil
	}
	if len(localOnly) > 0 && localOnly[0] {
		return status, nil
	}
	release, latestErr := fetchLatestRelease(ctx)
	if latestErr != nil {
		return status, nil
	}
	if _, err := pickAsset(release.Assets); err != nil {
		status.Supported = false
		status.UnsupportedReason = err.Error()
		return status, nil
	}
	latest := strings.TrimSpace(release.TagName)
	status.LatestVersion = latest
	if status.Installed && latest != "" {
		status.UpdateAvailable = status.Version == "" || normalizeReleaseVersion(status.Version) != normalizeReleaseVersion(latest)
	}
	return status, nil
}

func Update(ctx context.Context) error {
	_, err := InstallLatest(ctx, config.GetBinFolderPath())
	return err
}

func normalizeReleaseVersion(value string) string {
	value = strings.TrimSpace(value)
	return strings.TrimPrefix(value, "v")
}
