package naiveproxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"

	"github.com/SawaMEN/3x-ui/v3/internal/githubrelease"
)

const (
	releasesAPI      = "https://api.github.com/repos/klzgrad/forwardproxy/releases/latest"
	releaseAssetName = "caddy-forwardproxy-naive.tar.xz"
)

var managerMu sync.Mutex

type Status struct {
	Source            string `json:"source,omitempty"`
	FallbackReason    string `json:"fallbackReason,omitempty"`
	Supported         bool   `json:"supported"`
	UnsupportedReason string `json:"unsupportedReason,omitempty"`
	Installed         bool   `json:"installed"`
	Version           string `json:"version"`
	LatestVersion     string `json:"latestVersion"`
	UpdateAvailable   bool   `json:"updateAvailable"`
	Binary            string `json:"binary"`
	Running           bool   `json:"running"`
}

type releaseAsset = githubrelease.Asset
type release = githubrelease.Release

func GetStatus(ctx context.Context) (Status, error) {
	binary := BinaryPath()
	info, statErr := os.Stat(binary)
	status := Status{
		Supported: runtime.GOOS == "linux" && runtime.GOARCH == "amd64",
		Installed: statErr == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0,
		Version:   installedVersion(binary),
		Binary:    binary,
	}

	if !status.Supported {
		status.Installed = false
		status.UnsupportedReason = fmt.Sprintf("Caddy-Naive server is published only for linux/amd64; current platform: %s/%s", runtime.GOOS, runtime.GOARCH)
		return status, nil
	}
	rel, err := fetchLatestRelease(ctx)
	if err != nil {
		// Network failure must not make an installed component look absent.
		return status, nil
	}
	if _, err := pickAsset(rel.Assets); err != nil {
		status.Supported = false
		status.UnsupportedReason = err.Error()
		return status, nil
	}
	status.LatestVersion = strings.TrimSpace(rel.TagName)
	if status.Installed && status.LatestVersion != "" {
		status.UpdateAvailable = normalizeVersion(status.Version) != normalizeVersion(status.LatestVersion)
	}
	return status, nil
}

// Update installs the official Caddy build carrying klzgrad/forwardproxy@naive.
// The official upstream archive currently targets Linux amd64; other platforms
// must use sing-box's native Naive implementation instead of this sidecar.
func Update(ctx context.Context) (Status, error) {
	managerMu.Lock()
	defer managerMu.Unlock()

	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return Status{}, fmt.Errorf("standalone NaiveProxy server is supported on Linux amd64 only")
	}
	rel, err := fetchLatestRelease(ctx)
	if err != nil {
		return Status{}, err
	}
	asset, err := pickAsset(rel.Assets)
	if err != nil {
		return Status{}, err
	}

	tmpDir, err := os.MkdirTemp("", "3x-ui-caddy-naive-*")
	if err != nil {
		return Status{}, err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, asset.Name)
	if err := downloadAndVerify(ctx, asset, archivePath); err != nil {
		return Status{}, err
	}
	if err := extractTarXZ(ctx, archivePath, tmpDir); err != nil {
		return Status{}, err
	}
	extracted, err := findExtractedBinary(tmpDir)
	if err != nil {
		return Status{}, err
	}

	_ = stopLocked()
	binDir := config.GetBinFolderPath()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return Status{}, err
	}
	staged := filepath.Join(binDir, ".caddy-naive.new")
	if err := copyFile(extracted, staged); err != nil {
		return Status{}, err
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		_ = os.Remove(staged)
		return Status{}, err
	}
	if err := os.Rename(staged, BinaryPath()); err != nil {
		_ = os.Remove(staged)
		return Status{}, err
	}
	if err := os.WriteFile(VersionPath(), []byte(strings.TrimSpace(rel.TagName)+"\n"), 0o644); err != nil {
		return Status{}, err
	}

	// The protocolserver owner restarts current listeners after Update returns.
	status, err := statusWithoutRelease()
	if err != nil {
		return Status{}, err
	}
	status.LatestVersion = strings.TrimSpace(rel.TagName)
	status.UpdateAvailable = false
	return status, nil
}

// stopLocked retires a PID-file process left by older installations before
// replacing its binary. Current listeners are owned by protocolserver.
func stopLocked() error {
	pid, ok := readPID()
	if !ok {
		_ = os.Remove(pidPath())
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(pidPath())
		return nil
	}
	if runtime.GOOS == "linux" && !pidBelongsToBinary(pid) {
		_ = os.Remove(pidPath())
		return nil
	}
	_ = proc.Signal(syscall.SIGTERM)
	for i := 0; i < 20; i++ {
		if !pidAlive(pid) {
			_ = os.Remove(pidPath())
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = proc.Kill()
	_ = os.Remove(pidPath())
	return nil
}

func readPID() (int, bool) {
	data, err := os.ReadFile(pidPath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid, err == nil && pid > 1
}

func pidAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func pidBelongsToBinary(pid int) bool {
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return false
	}
	want, err := filepath.EvalSymlinks(BinaryPath())
	if err != nil {
		want = BinaryPath()
	}
	got, err := filepath.EvalSymlinks(exe)
	if err != nil {
		got = exe
	}
	return got == want
}

func BinaryPath() string  { return filepath.Join(config.GetBinFolderPath(), "caddy-naive") }
func VersionPath() string { return filepath.Join(config.GetBinFolderPath(), "caddy-naive.version") }
func pidPath() string     { return filepath.Join(config.GetBinFolderPath(), "caddy-naive.pid") }
func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func statusWithoutRelease() (Status, error) {
	binary := BinaryPath()
	_, err := os.Stat(binary)
	return Status{Installed: err == nil, Version: installedVersion(binary), Binary: binary}, nil
}

func installedVersion(binary string) string {
	if data, err := os.ReadFile(VersionPath()); err == nil {
		if value := strings.TrimSpace(string(data)); value != "" {
			return value
		}
	}
	if _, err := os.Stat(binary); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "version").CombinedOutput()
	if err != nil && len(output) == 0 {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func fetchLatestRelease(ctx context.Context) (release, error) {
	rel, err := githubrelease.Fetch(ctx, http.DefaultClient, releasesAPI)
	if err != nil {
		return release{}, fmt.Errorf("fetch Caddy-Naive release: %w", err)
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return release{}, fmt.Errorf("Caddy-Naive release API returned an empty tag")
	}
	return rel, nil
}

func pickAsset(assets []releaseAsset) (releaseAsset, error) {
	for _, asset := range assets {
		if asset.Name == releaseAssetName && asset.URL != "" {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("official release does not contain %s", releaseAssetName)
}

func downloadAndVerify(ctx context.Context, asset releaseAsset, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "3x-ui")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Caddy-Naive download returned %s", resp.Status)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, hash), resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if strings.HasPrefix(strings.ToLower(asset.Digest), "sha256:") {
		expected := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(asset.Digest), "sha256:"))
		actual := hex.EncodeToString(hash.Sum(nil))
		if expected != "" && actual != expected {
			return fmt.Errorf("Caddy-Naive archive checksum mismatch")
		}
	}
	return nil
}

func extractTarXZ(ctx context.Context, archivePath, dst string) error {
	if _, err := exec.LookPath("tar"); err != nil {
		return fmt.Errorf("tar with xz support is required to install Caddy-Naive: %w", err)
	}
	cmd := exec.CommandContext(ctx, "tar", "--no-same-owner", "--no-same-permissions", "-xJf", archivePath, "-C", dst)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract Caddy-Naive: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func findExtractedBinary(root string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name := strings.ToLower(filepath.Base(path))
		if name == "caddy" || name == "caddy-forwardproxy-naive" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && found == "" {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("Caddy binary not found in %s", releaseAssetName)
	}
	return found, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
