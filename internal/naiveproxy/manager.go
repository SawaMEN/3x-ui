package naiveproxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

const releasesAPI = "https://api.github.com/repos/klzgrad/naiveproxy/releases/latest"

type Status struct {
	Installed       bool   `json:"installed"`
	Version         string `json:"version"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Binary          string `json:"binary"`
}

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

func GetStatus(ctx context.Context) (Status, error) {
	binary := BinaryPath()
	_, statErr := os.Stat(binary)
	status := Status{
		Installed: statErr == nil,
		Version:   installedVersion(binary),
		Binary:    binary,
	}

	rel, err := fetchLatestRelease(ctx)
	if err != nil {
		// A temporary GitHub/API failure must not make an already installed
		// standalone NaiveProxy look absent to the panel.
		return status, nil
	}
	status.LatestVersion = strings.TrimSpace(rel.TagName)
	if status.Installed && status.LatestVersion != "" {
		status.UpdateAvailable = normalizeVersion(status.Version) != normalizeVersion(status.LatestVersion)
	}
	return status, nil
}

func Update(ctx context.Context) (Status, error) {
	rel, err := fetchLatestRelease(ctx)
	if err != nil {
		return Status{}, err
	}
	asset, err := pickAsset(rel.Assets)
	if err != nil {
		return Status{}, err
	}

	tmpDir, err := os.MkdirTemp("", "3x-ui-naiveproxy-*")
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

	binDir := config.GetBinFolderPath()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return Status{}, err
	}
	staged := filepath.Join(binDir, ".naive.new")
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
	if err := os.WriteFile(versionPath(), []byte(strings.TrimSpace(rel.TagName)+"\n"), 0o644); err != nil {
		return Status{}, err
	}

	return GetStatus(ctx)
}

func BinaryPath() string {
	name := "naive"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(config.GetBinFolderPath(), name)
}

func versionPath() string {
	return filepath.Join(config.GetBinFolderPath(), "naiveproxy.version")
}

func installedVersion(binary string) string {
	if data, err := os.ReadFile(versionPath()); err == nil {
		if value := strings.TrimSpace(string(data)); value != "" {
			return value
		}
	}
	if _, err := os.Stat(binary); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "--version").CombinedOutput()
	if err != nil && len(output) == 0 {
		return ""
	}
	return parseVersionOutput(string(output))
}

func parseVersionOutput(output string) string {
	for _, field := range strings.Fields(strings.TrimSpace(output)) {
		candidate := strings.Trim(field, " ,;()[]")
		plain := strings.TrimPrefix(candidate, "v")
		if strings.Count(plain, ".") >= 2 && strings.ContainsAny(plain, "0123456789") {
			return candidate
		}
	}
	return strings.TrimSpace(output)
}

func normalizeVersion(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "v")
}

func fetchLatestRelease(ctx context.Context) (release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesAPI, nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "3x-ui")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return release{}, fmt.Errorf("fetch NaiveProxy release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("NaiveProxy release API returned %s", resp.Status)
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return release{}, err
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return release{}, fmt.Errorf("NaiveProxy release API returned an empty tag")
	}
	return rel, nil
}

func pickAsset(assets []releaseAsset) (releaseAsset, error) {
	if runtime.GOOS != "linux" {
		return releaseAsset{}, fmt.Errorf("standalone NaiveProxy updater is supported on Linux only")
	}
	arch := ""
	switch runtime.GOARCH {
	case "amd64":
		arch = "x64"
	case "386":
		arch = "x86"
	case "arm64":
		arch = "arm64"
	case "arm":
		arch = "arm"
	case "loong64":
		arch = "loong64"
	case "mips64le":
		arch = "mips64el"
	case "mipsle":
		arch = "mipsel"
	case "riscv64":
		arch = "riscv64"
	default:
		return releaseAsset{}, fmt.Errorf("NaiveProxy is not published for linux/%s", runtime.GOARCH)
	}
	needle := "-linux-" + arch + ".tar.xz"
	for _, asset := range assets {
		if strings.HasSuffix(asset.Name, needle) && asset.URL != "" {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("no NaiveProxy release asset for linux/%s", runtime.GOARCH)
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
		return fmt.Errorf("NaiveProxy download returned %s", resp.Status)
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
			return fmt.Errorf("NaiveProxy archive checksum mismatch")
		}
	}
	return nil
}

func extractTarXZ(ctx context.Context, archivePath, dst string) error {
	if _, err := exec.LookPath("tar"); err != nil {
		return fmt.Errorf("tar is required to install standalone NaiveProxy: %w", err)
	}
	cmd := exec.CommandContext(ctx, "tar", "-xJf", archivePath, "-C", dst)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract NaiveProxy: %w: %s", err, strings.TrimSpace(string(output)))
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
		if filepath.Base(path) == "naive" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && found == "" {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("naive binary not found in NaiveProxy archive")
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
