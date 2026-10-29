package externalvpn

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func installVetroff(ctx context.Context) error {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return fmt.Errorf("VetroffProxy installation supports Linux amd64/arm64")
	}
	externalVPNUpdateMu.Lock()
	defer externalVPNUpdateMu.Unlock()
	if err := cleanupVetroffStages(config.GetBinFolderPath()); err != nil {
		return err
	}
	spec, err := releaseSpecFor(model.VetroffProxy)
	if err != nil {
		return err
	}
	release, err := fetchLatestRelease(ctx, spec)
	if err != nil {
		return err
	}
	asset, err := pickReleaseAsset(spec, release, runtime.GOARCH)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(asset.Digest, "sha256:") {
		return fmt.Errorf("VetroffProxy release has no SHA-256 digest")
	}
	dir := config.GetBinFolderPath()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(dir, ".vetroff-stage-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	archive := filepath.Join(stage, "release.tar.gz")
	if err := downloadReleaseAsset(ctx, asset.URL, archive); err != nil {
		return err
	}
	if err := verifyReleaseDigest(archive, asset.Digest); err != nil {
		return err
	}
	bundle := filepath.Join(stage, "bundle")
	if err := extractVetroffBundle(archive, bundle); err != nil {
		return err
	}
	executable := vetroffExecutable(bundle)
	version, err := binaryVersion(ctx, executable, spec.versionArgs)
	if err != nil {
		return fmt.Errorf("validate VetroffProxy: %w", err)
	}
	if version != normalizeReleaseVersion(release.TagName) {
		return fmt.Errorf("VetroffProxy release version mismatch")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	target := filepath.Join(dir, "vetroff-bundle")
	previous := filepath.Join(stage, "previous")
	manager := GetManager()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for id, proc := range manager.procs {
		if proc.protocol == model.VetroffProxy {
			manager.removeLocked(id)
		}
	}
	if _, err := os.Lstat(target); err == nil {
		if err := os.Rename(target, previous); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(bundle, target); err != nil {
		if restoreErr := os.Rename(previous, target); restoreErr != nil && !os.IsNotExist(restoreErr) {
			return fmt.Errorf("install: %w; restore: %w", err, restoreErr)
		}
		return err
	}
	return nil
}

func extractVetroffBundle(archive, destination string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	var total int64
	archiveRoot := ""
	for count := 0; ; count++ {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(h.Name, "/")
		if (name == "foxtunnel-necko/." || name == "vetroff-necko/.") && h.Typeflag == tar.TypeDir {
			name = strings.TrimSuffix(name, "/.")
		}
		memberRoot := ""
		for _, candidate := range []string{"vetroff-necko", "foxtunnel-necko"} {
			if name == candidate || strings.HasPrefix(name, candidate+"/") {
				memberRoot = candidate
				break
			}
		}
		if archiveRoot == "" {
			archiveRoot = memberRoot
		}
		clean := filepath.ToSlash(filepath.Clean(name))
		if count > 50000 || !filepath.IsLocal(name) || clean != name || strings.Contains(name, "\\") || (memberRoot == "" || memberRoot != archiveRoot) || seen[name] || h.Mode&0o7000 != 0 {
			return fmt.Errorf("unsafe VetroffProxy archive member")
		}
		seen[name] = true
		path := filepath.FromSlash(name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if h.Size < 0 || h.Size > (2<<30)-total {
				return fmt.Errorf("VetroffProxy bundle exceeds size limit")
			}
			total += h.Size
			if err := root.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			out, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(out, tr, h.Size)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("VetroffProxy archive links and special files are forbidden")
		}
	}
	for _, name := range []string{"vetroff", "runtime/xpcshell", archiveRoot} {
		info, err := root.Stat(filepath.Join(archiveRoot, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("VetroffProxy bundle missing executable %s", name)
		}
	}
	for _, name := range []string{"lib", "runtime"} {
		info, err := root.Stat(filepath.Join(archiveRoot, name))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("VetroffProxy bundle missing %s", name)
		}
	}
	return nil
}

func RemoveVetroff(ctx context.Context) error {
	externalVPNUpdateMu.Lock()
	defer externalVPNUpdateMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	manager := GetManager()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for id, proc := range manager.procs {
		if proc.protocol == model.VetroffProxy {
			manager.removeLocked(id)
		}
	}
	dir := config.GetBinFolderPath()
	if err := cleanupVetroffStages(dir); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(dir, "vetroff-bundle"))
}

func vetroffExecutable(bundle string) string {
	for _, name := range []string{"vetroff-necko", "foxtunnel-necko"} {
		path := filepath.Join(bundle, name, "vetroff")
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return filepath.Join(bundle, "vetroff-necko", "vetroff")
}

func cleanupVetroffStages(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "vetroff-bundle")
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".vetroff-stage-") {
			continue
		}
		stage := filepath.Join(dir, entry.Name())
		// Recover a previous runtime if a crash interrupted the directory swap.
		if entry.IsDir() {
			previous := filepath.Join(stage, "previous")
			if _, err := os.Lstat(target); os.IsNotExist(err) {
				if _, err := os.Lstat(previous); err == nil {
					if err := os.Rename(previous, target); err != nil {
						return err
					}
				}
			}
		}
		if err := os.RemoveAll(stage); err != nil {
			return fmt.Errorf("remove abandoned VetroffProxy download: %w", err)
		}
	}
	return nil
}
