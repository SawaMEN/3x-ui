// Package coreinstall installs separate upstream engines without replacing the bundled core.
package coreinstall

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	maxArchive = 128 << 20
	maxBinary  = 256 << 20
)

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}
type Release struct {
	Tag    string  `json:"tag_name"`
	Assets []Asset `json:"assets"`
}

func repository(engine string) (string, error) {
	switch engine {
	case "xray":
		return "XTLS/Xray-core", nil
	case "sing-box-upstream":
		return "SagerNet/sing-box", nil
	default:
		return "", fmt.Errorf("unsupported standalone engine %q", engine)
	}
}

func AssetName(engine, tag, goos, arch, arm string) (string, error) {
	if _, err := repository(engine); err != nil {
		return "", err
	}
	if goos != "linux" {
		return "", fmt.Errorf("automatic core installation is supported on Linux only")
	}
	if engine == "sing-box-upstream" {
		switch arch {
		case "amd64", "arm64", "386", "riscv64", "loong64", "s390x", "ppc64le", "mipsle", "mips64le":
		case "mips", "mips64":
			arch += "-softfloat"
		case "arm":
			if arm == "" {
				arm = "7"
			}
			if arm != "5" && arm != "6" && arm != "7" {
				return "", fmt.Errorf("unsupported GOARM %q", arm)
			}
			arch = "armv" + arm
		default:
			return "", fmt.Errorf("unsupported sing-box architecture %s", arch)
		}
		return "sing-box-" + strings.TrimPrefix(tag, "v") + "-linux-" + arch + ".tar.gz", nil
	}
	names := map[string]string{"amd64": "64", "386": "32", "arm64": "arm64-v8a", "riscv64": "riscv64", "loong64": "loong64", "mips": "mips32", "mipsle": "mips32le", "mips64": "mips64", "mips64le": "mips64le", "ppc64": "ppc64", "ppc64le": "ppc64le", "s390x": "s390x"}
	if arch == "arm" {
		switch arm {
		case "5", "6":
			return "Xray-linux-arm32-v" + arm + ".zip", nil
		case "", "7":
			return "Xray-linux-arm32-v7a.zip", nil
		default:
			return "", fmt.Errorf("unsupported GOARM %q", arm)
		}
	}
	name, ok := names[arch]
	if !ok {
		return "", fmt.Errorf("unsupported Xray architecture %s", arch)
	}
	return "Xray-linux-" + name + ".zip", nil
}

// Installer supports an injected HTTP client for deterministic tests.
type Installer struct{ Client *http.Client }

func (i Installer) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "3x-ui-core-installer")
	client := i.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download exceeds size limit")
	}
	return data, nil
}

// Install validates the release checksum and staged executable before an atomic rename.
// Callers serialize installation with core switching and reject replacing a running core.
func (i Installer) Install(ctx context.Context, engine, version, goos, arch, arm, target string, validate func(string) error) (string, error) {
	repo, err := repository(engine)
	if err != nil {
		return "", err
	}
	endpoint := "https://api.github.com/repos/" + repo + "/releases/latest"
	if version != "" && version != "latest" {
		if !regexp.MustCompile(`^v?[0-9][A-Za-z0-9._-]{0,79}$`).MatchString(version) {
			return "", fmt.Errorf("invalid release version")
		}
		endpoint = "https://api.github.com/repos/" + repo + "/releases/tags/" + version
	}
	raw, err := i.get(ctx, endpoint, 4<<20)
	if err != nil {
		return "", err
	}
	var release Release
	if err = json.Unmarshal(raw, &release); err != nil {
		return "", err
	}
	if !regexp.MustCompile(`^v?[0-9][A-Za-z0-9._-]{0,79}$`).MatchString(release.Tag) {
		return "", fmt.Errorf("invalid release tag")
	}
	name, err := AssetName(engine, release.Tag, goos, arch, arm)
	if err != nil {
		return "", err
	}
	var asset *Asset
	for n := range release.Assets {
		if release.Assets[n].Name == name {
			asset = &release.Assets[n]
			break
		}
	}
	if asset == nil {
		return "", fmt.Errorf("release %s has no asset %s", release.Tag, name)
	}
	expectedURL := "https://github.com/" + repo + "/releases/download/" + release.Tag + "/" + name
	if asset.URL != expectedURL {
		return "", fmt.Errorf("unexpected release asset URL")
	}
	data, err := i.get(ctx, asset.URL, maxArchive)
	if err != nil {
		return "", err
	}
	want := strings.TrimPrefix(asset.Digest, "sha256:")
	if !strings.HasPrefix(asset.Digest, "sha256:") || len(want) != 64 {
		if engine != "xray" {
			return "", fmt.Errorf("release asset has no SHA-256 digest")
		}
		sums, err := i.get(ctx, asset.URL+".dgst", 64<<10)
		if err != nil {
			return "", err
		}
		want, err = xrayDigest(sums)
		if err != nil {
			return "", err
		}
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), want) {
		return "", fmt.Errorf("release checksum mismatch")
	}
	binary := "sing-box"
	if engine == "xray" {
		binary = "xray"
	}
	executable, err := extract(data, name, binary)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".core-"+engine+"-install-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(executable); err != nil {
		return "", err
	}
	if err = f.Chmod(0o755); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if validate == nil {
		return "", fmt.Errorf("core binary validation is required")
	}
	if err = validate(f.Name()); err != nil {
		return "", err
	}
	var geofiles []string
	if engine == "xray" {
		geofiles, err = installMissingXrayGeodata(data, filepath.Dir(target))
		if err != nil {
			return "", err
		}
	}
	if err = os.Rename(f.Name(), target); err != nil {
		for _, path := range geofiles {
			_ = os.Remove(path)
		}
		return "", err
	}
	return release.Tag, nil
}

func xrayDigest(data []byte) (string, error) {
	// Xray publishes OpenSSL-style SHA2-256 (file) = digest lines.
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(strings.ToUpper(line), "SHA256") || strings.Contains(strings.ToUpper(line), "SHA2-256") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				v := fields[len(fields)-1]
				if len(v) == 64 {
					if _, err := hex.DecodeString(v); err == nil {
						return v, nil
					}
				}
			}
		}
	}
	return "", fmt.Errorf("missing SHA-256 in Xray digest")
}

func extract(data []byte, archive, binary string) ([]byte, error) {
	var result []byte
	read := func(r io.Reader) error {
		if result != nil {
			return fmt.Errorf("duplicate binary in archive")
		}
		b, e := io.ReadAll(io.LimitReader(r, maxBinary+1))
		if e != nil {
			return e
		}
		if len(b) == 0 || len(b) > maxBinary {
			return fmt.Errorf("invalid binary size")
		}
		result = b
		return nil
	}
	if strings.HasSuffix(archive, ".zip") {
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range z.File {
			if f.Name == binary && f.Mode().IsRegular() {
				r, e := f.Open()
				if e != nil {
					return nil, e
				}
				e = read(r)
				r.Close()
				if e != nil {
					return nil, e
				}
			}
		}
	} else {
		g, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer g.Close()
		t := tar.NewReader(io.LimitReader(g, maxBinary+(16<<20)))
		for {
			h, e := t.Next()
			if errors.Is(e, io.EOF) {
				break
			}
			if e != nil {
				return nil, e
			}
			if h.FileInfo().Mode().IsRegular() && path.Base(h.Name) == binary && h.Name == strings.TrimSuffix(archive, ".tar.gz")+"/"+binary {
				if e = read(t); e != nil {
					return nil, e
				}
			}
		}
	}
	if result == nil {
		return nil, fmt.Errorf("binary %s missing from release archive", binary)
	}
	return result, nil
}

// Xray needs these databases even with the panel's default private-IP rule.
// Existing panel-managed databases are preserved; hard-link publication does
// not overwrite a database created concurrently by the geodata updater.
func installMissingXrayGeodata(archive []byte, dir string) (created []string, err error) {
	defer func() {
		if err != nil {
			for _, p := range created {
				_ = os.Remove(p)
			}
		}
	}()
	for _, name := range []string{"geoip.dat", "geosite.dat"} {
		dest := filepath.Join(dir, name)
		if _, statErr := os.Stat(dest); statErr == nil {
			continue
		} else if !os.IsNotExist(statErr) {
			return created, statErr
		}
		data, readErr := extract(archive, "Xray.zip", name)
		if readErr != nil {
			return created, readErr
		}
		if err = publishMissingFile(dest, data); err != nil {
			if os.IsExist(err) {
				err = nil
				continue
			}
			return created, err
		}
		created = append(created, dest)
	}
	return created, nil
}

func publishMissingFile(dest string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(dest), ".core-geodata-*")
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Chmod(0o644); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Link(f.Name(), dest)
}
