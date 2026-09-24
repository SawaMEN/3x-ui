package sudoku

import (
    "archive/tar"
    "archive/zip"
    "compress/gzip"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "runtime"
    "strings"
)

const releasesAPI = "https://api.github.com/repos/SUDOKU-ASCII/sudoku/releases"

type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func LatestRelease(ctx context.Context) (string, error) {
	rel, err := fetchLatestRelease(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(rel.TagName), nil
}

func fetchLatestRelease(ctx context.Context) (release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesAPI+"/latest", nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "3x-ui")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return release{}, fmt.Errorf("fetch Sudoku release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("Sudoku release API returned %s", resp.Status)
	}

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return release{}, err
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return release{}, fmt.Errorf("Sudoku release API returned an empty tag")
	}
	return rel, nil
}

func GetBinaryPath(binDir string) string {
    name := "sudoku-tunnel"
    if runtime.GOOS == "windows" {
        name += ".exe"
    }
    return filepath.Join(binDir, name)
}

func EnsureInstalled(ctx context.Context, binDir string) (string, error) {
    path := GetBinaryPath(binDir)
    if st, err := os.Stat(path); err == nil && !st.IsDir() {
        return path, nil
    }
    return InstallLatest(ctx, binDir)
}

func InstallLatest(ctx context.Context, binDir string) (string, error) {
	rel, err := fetchLatestRelease(ctx)
	if err != nil {
		return "", err
	}
	asset, err := pickAsset(rel.Assets)
    if err != nil {
        return "", err
    }

    tmp, err := os.MkdirTemp("", "3x-ui-sudoku-*")
    if err != nil {
        return "", err
    }
    defer os.RemoveAll(tmp)

    archivePath := filepath.Join(tmp, asset.Name)
    if err := download(ctx, asset.URL, archivePath); err != nil {
        return "", err
    }
    extracted, err := extractBinary(archivePath, tmp)
    if err != nil {
        return "", err
    }
    if err := os.MkdirAll(binDir, 0o755); err != nil {
        return "", err
    }
    staged := filepath.Join(tmp, filepath.Base(GetBinaryPath(binDir)))
    if err := copyFile(extracted, staged); err != nil {
        return "", err
    }
    if err := os.Chmod(staged, 0o755); err != nil {
        return "", err
    }
	if err := os.Rename(staged, GetBinaryPath(binDir)); err != nil {
		return "", err
	}
	if err := writeInstalledVersion(binDir, rel.TagName); err != nil {
		return "", err
	}
	return GetBinaryPath(binDir), nil
}

func installedVersionPath(binDir string) string {
	return filepath.Join(binDir, "sudoku.version")
}

func readInstalledVersion(binDir string) string {
	b, err := os.ReadFile(installedVersionPath(binDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func writeInstalledVersion(binDir, version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return fmt.Errorf("Sudoku release tag is empty")
	}
	return os.WriteFile(installedVersionPath(binDir), []byte(version+"\n"), 0o644)
}

func pickAsset(assets []struct {
    Name string `json:"name"`
    URL  string `json:"browser_download_url"`
}) (struct {
    Name string
    URL  string
}, error) {
    arch := runtime.GOARCH
    switch arch {
    case "amd64", "arm64":
    default:
        return struct {
            Name string
            URL  string
        }{}, fmt.Errorf("Sudoku is not published for %s/%s", runtime.GOOS, runtime.GOARCH)
    }
    prefix := "sudoku-" + runtime.GOOS + "-" + arch
    var out struct {
        Name string
        URL  string
    }
    for _, a := range assets {
        if strings.HasPrefix(a.Name, prefix+".") &&
            (strings.HasSuffix(strings.ToLower(a.Name), ".tar.gz") || strings.HasSuffix(strings.ToLower(a.Name), ".zip")) {
            out.Name, out.URL = a.Name, a.URL
            break
        }
    }
    if out.Name == "" {
        return out, fmt.Errorf("no Sudoku release asset for %s/%s", runtime.GOOS, runtime.GOARCH)
    }
    return out, nil
}

func download(ctx context.Context, url, dst string) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
        return fmt.Errorf("Sudoku download returned %s", resp.Status)
    }
    out, err := os.Create(dst)
    if err != nil {
        return err
    }
    defer out.Close()
    _, err = io.Copy(out, resp.Body)
    return err
}

func extractBinary(archivePath, dir string) (string, error) {
    lower := strings.ToLower(archivePath)
    if strings.HasSuffix(lower, ".zip") {
        z, err := zip.OpenReader(archivePath)
        if err != nil {
            return "", err
        }
        defer z.Close()
        for _, f := range z.File {
            if filepath.Base(f.Name) == "sudoku-tunnel" || filepath.Base(f.Name) == "sudoku-tunnel.exe" {
                dst := filepath.Join(dir, filepath.Base(f.Name))
                r, err := f.Open()
                if err != nil {
                    return "", err
                }
                out, err := os.Create(dst)
                if err != nil {
                    r.Close()
                    return "", err
                }
                _, copyErr := io.Copy(out, r)
                closeErr := out.Close()
                r.Close()
                if copyErr != nil {
                    return "", copyErr
                }
                if closeErr != nil {
                    return "", closeErr
                }
                return dst, nil
            }
        }
        return "", fmt.Errorf("sudoku-tunnel binary not found in archive")
    }

    f, err := os.Open(archivePath)
    if err != nil {
        return "", err
    }
    defer f.Close()
    gz, err := gzip.NewReader(f)
    if err != nil {
        return "", err
    }
    defer gz.Close()
    tr := tar.NewReader(gz)
    for {
        h, err := tr.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return "", err
        }
        if filepath.Base(h.Name) != "sudoku-tunnel" {
            continue
        }
        dst := filepath.Join(dir, "sudoku-tunnel")
        out, err := os.Create(dst)
        if err != nil {
            return "", err
        }
        if _, err := io.Copy(out, tr); err != nil {
            out.Close()
            return "", err
        }
        if err := out.Close(); err != nil {
            return "", err
        }
        return dst, nil
    }
    return "", fmt.Errorf("sudoku-tunnel binary not found in archive")
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
