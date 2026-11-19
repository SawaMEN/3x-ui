//go:build linux

package systemupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

const (
	cleanupTimeout     = 3 * time.Minute
	coreArtifactMaxAge = time.Hour
	tempArtifactMaxAge = 24 * time.Hour
)

// Cleanup clears package-manager caches and abandoned panel-owned temporary
// downloads. Never interrupt a package transaction: killing dpkg/pacman during
// installation can leave an unbootable or partially configured system.
func Cleanup(ctx context.Context) (CleanupResult, error) {
	result := CleanupResult{
		Actions:  []string{},
		Warnings: []string{},
	}
	if os.Geteuid() != 0 {
		return result, fmt.Errorf("cache cleanup requires root privileges")
	}
	if !updateMu.TryLock() {
		return result, fmt.Errorf("a panel-managed system update is running; retry cleanup when it finishes")
	}
	defer updateMu.Unlock()

	active, checkErr := cleanupBlockingPackageManager(ctx, "/proc", hasActivePackageKitTransactions)
	// External package updates must not block cleanup of unrelated 3x-ui
	// caches. Skip only package-manager caches and in-flight downloads.
	packageCacheSafe := packageCacheCleanupAllowed(active, checkErr, &result)

	cleanupCtx, cancel := context.WithTimeout(ctx, cleanupTimeout)
	defer cancel()
	before, beforeOK := availableBytes("/")
	info := detectDistribution()
	commands := cleanupCommands(info.manager)
	if packageCacheSafe {
		if len(commands) == 0 {
			result.Warnings = append(result.Warnings, "Unsupported package manager; its cache was not cleared.")
		}
		for _, args := range commands {
			if err := cleanupCtx.Err(); err != nil {
				return result, err
			}
			if !commandExists(args[0]) {
				result.Warnings = append(result.Warnings, args[0]+": command not found")
				continue
			}
			output, err := runCommand(cleanupCtx, args[0], args[1:]...)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v (%s)", args[0], err, truncateOutput(output, 300)))
			} else {
				result.Actions = append(result.Actions, strings.Join(args, " "))
			}
		}
	}

	// Package managers leave fragments after interrupted *downloads*. Do not
	// remove locks, package databases, installed kernels or complete archives.
	partialLocations := []cleanupLocation{}
	if packageCacheSafe {
		switch info.manager {
		case "apt-get":
			// All regular files left in apt's partial directories are unfinished.
			partialLocations = []cleanupLocation{
				{"/var/cache/apt/archives/partial", isAnyFile},
				{"/var/lib/apt/lists/partial", isAnyFile},
			}
		case "pacman":
			partialLocations = []cleanupLocation{
				{"/var/cache/pacman/pkg", isPartialDownload},
				{"/var/lib/pacman/sync", isPartialDownload},
			}
		case "dnf", "yum":
			partialLocations = []cleanupLocation{
				{"/var/cache/dnf", isPartialDownload},
				{"/var/cache/yum", isPartialDownload},
			}
		}
	}
	for _, location := range partialLocations {
		if err := removeStaleFiles(cleanupCtx, location.dir, 0, location.match, &result); err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
	}
	if len(partialLocations) > 0 {
		result.Actions = append(result.Actions, "Interrupted package download fragments")
	}

	// Standalone VPN engines use these exact, internal staging prefixes. A
	// missing installed binary is NOT a reason to delete any unrelated file.
	binFolders := map[string]bool{}
	binDir := config.GetBinFolderPath()
	if binDir != "/" && binDir != "." {
		binFolders[binDir] = true
	}
	for _, engine := range []string{"xray", "sing-box-upstream"} {
		target := config.StandaloneCoreBinaryPath(engine)
		if target == "" {
			continue
		}
		dir := filepath.Dir(target)
		if dir != "/" && dir != "." {
			binFolders[dir] = true
		}
	}
	for dir := range binFolders {
		if err := removeStaleFiles(cleanupCtx, dir, coreArtifactMaxAge, isStaleCoreArtifact, &result); err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
	}
	result.Actions = append(result.Actions, "Abandoned VPN-core staging files")

	cleanupProtocolInstallArtifacts(cleanupCtx, &result)

	// Only files with panel-specific prefixes are considered for direct removal.
	// Other system temp content follows the OS tmpfiles retention policy instead
	// of indiscriminately deleting potentially active sockets or workspaces.
	for _, dir := range []string{"/tmp", "/var/tmp"} {
		if err := removeStaleFiles(cleanupCtx, dir, tempArtifactMaxAge, isPanelTempArtifact, &result); err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
		if err := removeOldPanelTempDirs(cleanupCtx, dir, tempArtifactMaxAge, &result); err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
	}
	if commandExists("systemd-tmpfiles") {
		if output, err := runCommand(cleanupCtx, "systemd-tmpfiles", "--clean"); err != nil {
			result.Warnings = append(result.Warnings, "systemd-tmpfiles: "+truncateOutput(output+" "+err.Error(), 300))
		} else {
			result.Actions = append(result.Actions, "System temporary files (OS retention rules)")
		}
	} else {
		result.Actions = append(result.Actions, "Expired 3x-ui temporary files")
	}
	if after, ok := availableBytes("/"); ok && beforeOK && after > before {
		result.FreedBytes = after - before
	}
	return result, nil
}

type cleanupLocation struct {
	dir   string
	match func(string) bool
}

func cleanupCommands(manager string) [][]string {
	switch manager {
	case "apt-get":
		return [][]string{{"apt-get", "clean"}}
	case "pacman":
		return [][]string{{"pacman", "-Scc", "--noconfirm"}}
	case "dnf":
		return [][]string{{"dnf", "clean", "all"}}
	case "yum":
		return [][]string{{"yum", "clean", "all"}}
	case "zypper":
		return [][]string{{"zypper", "clean", "--all"}}
	case "apk":
		return [][]string{{"apk", "cache", "clean"}}
	default:
		return nil
	}
}

// packageCacheCleanupAllowed is false for any active or unverified package
// transaction. Independent VPN and system-temp cleanup may still proceed.
func packageCacheCleanupAllowed(active string, checkErr error, result *CleanupResult) bool {
	if checkErr != nil {
		result.Warnings = append(result.Warnings,
			"Пропущена очистка пакетного кэша и незавершённых загрузок: не удалось проверить фоновые обновления ("+checkErr.Error()+").")
		return false
	}
	if active != "" {
		result.Warnings = append(result.Warnings,
			"Пропущена очистка пакетного кэша и незавершённых загрузок: запущен "+active+". Повторите после завершения обновления.")
		return false
	}
	return true
}

func activePackageManager(procDir string) (string, error) {
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return "", err
	}
	packageKitRunning := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		comm, err := os.ReadFile(filepath.Join(procDir, entry.Name(), "comm"))
		if errors.Is(err, os.ErrNotExist) {
			continue // process exited between directory scan and read
		}
		if err != nil {
			return "", err // fail closed if another process cannot be inspected
		}
		name := strings.TrimSpace(string(comm))
		if !isPackageManagerProcess(name) {
			continue
		}
		// A defunct process retains its comm in /proc but cannot hold any
		// package-manager locks or continue downloading packages.
		stat, statErr := os.ReadFile(filepath.Join(procDir, entry.Name(), "stat"))
		if statErr == nil {
			state, stateErr := procProcessState(string(stat))
			if stateErr != nil {
				return "", fmt.Errorf("inspect package manager %s (PID %d): %w", name, pid, stateErr)
			}
			if state == 'Z' || state == 'X' || state == 'x' {
				continue
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("inspect package manager %s (PID %d): %w", name, pid, statErr)
		}
		if name == "packagekitd" {
			// A long-lived PackageKit daemon can be idle. Prefer any actual
			// package-manager process, regardless of /proc iteration order.
			packageKitRunning = true
			continue
		}
		if isPackageManagerProcess(name) {
			return name, nil
		}
	}
	if packageKitRunning {
		return "packagekitd", nil
	}
	return "", nil
}

// procProcessState reads the state after the final ')' of /proc/PID/stat
// because process names can contain spaces and parentheses.
func procProcessState(stat string) (byte, error) {
	closeParen := strings.LastIndexByte(stat, ')')
	if closeParen < 0 || len(stat) <= closeParen+2 || stat[closeParen+1] != ' ' {
		return 0, fmt.Errorf("malformed /proc PID stat")
	}
	return stat[closeParen+2], nil
}

// cleanupBlockingPackageManager distinguishes an idle PackageKit service from
// a PackageKit transaction without ever interrupting the daemon or its jobs.
// If D-Bus cannot be queried, refuse cleanup rather than risk removing an
// active update's download fragments.
func cleanupBlockingPackageManager(
	ctx context.Context,
	procDir string,
	checkPackageKit func(context.Context) (bool, error),
) (string, error) {
	active, err := activePackageManager(procDir)
	if err != nil || active != "packagekitd" {
		return active, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	busy, err := checkPackageKit(queryCtx)
	if err != nil {
		return "", fmt.Errorf("cannot check PackageKit transactions: %w", err)
	}
	if busy {
		return "packagekitd", nil
	}
	return "", nil
}

func hasActivePackageKitTransactions(ctx context.Context) (bool, error) {
	if !commandExists("busctl") {
		return false, fmt.Errorf("busctl is unavailable; cannot verify whether PackageKit is idle")
	}
	// GetTransactionList reports in-progress transactions, not whether the
	// persistent packagekitd process merely exists on the system bus.
	output, err := runCommand(ctx, "busctl",
		"--system", "--timeout=5s", "call",
		"org.freedesktop.PackageKit", "/org/freedesktop/PackageKit",
		"org.freedesktop.PackageKit", "GetTransactionList",
	)
	if err != nil {
		return false, fmt.Errorf("PackageKit GetTransactionList: %w (%s)", err, truncateOutput(output, 300))
	}
	return parsePackageKitTransactionList(output)
}

func parsePackageKitTransactionList(output string) (bool, error) {
	fields := strings.Fields(output)
	if len(fields) < 2 || fields[0] != "ao" {
		return false, fmt.Errorf("unexpected PackageKit transaction response: %q", truncateOutput(output, 120))
	}
	count, err := strconv.Atoi(fields[1])
	if err != nil || count < 0 || count != len(fields)-2 {
		return false, fmt.Errorf("invalid PackageKit transaction count: %q", truncateOutput(output, 120))
	}
	for _, path := range fields[2:] {
		if !strings.HasPrefix(path, "/") {
			return false, fmt.Errorf("invalid PackageKit transaction path: %q", path)
		}
	}
	return count > 0, nil
}

func isPackageManagerProcess(name string) bool {
	switch name {
	case "apt", "apt-get", "aptitude", "dpkg", "dpkg-deb", "unattended-upgr", "pacman",
		"checkupdates", "makepkg", "dnf", "yum", "rpm", "zypper", "apk", "packagekitd":
		return true
	default:
		return false
	}
}

func isAnyFile(string) bool { return true }

func isPartialDownload(name string) bool {
	return strings.HasSuffix(name, ".part") ||
		strings.HasSuffix(name, ".partial") ||
		strings.HasSuffix(name, ".deb.FAILED") ||
		strings.HasSuffix(name, ".pkg.tar.zst.part")
}

func isStaleCoreArtifact(name string) bool {
	for _, prefix := range []string{
		".core-xray-install-", ".core-xray-update-",
		".core-sing-box-upstream-install-", ".core-sing-box-upstream-update-",
		".core-install-", ".core-update-", ".core-geodata-", ".sudoku-tunnel-",
	} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isPanelTempArtifact(name string) bool {
	for _, prefix := range []string{"3x-ui-", "x-ui-", ".3x-ui-", ".x-ui-"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// removeStaleFiles handles only regular, direct children of a known directory;
// symlinks, subdirectories, sockets, and special files are always preserved.
func removeStaleFiles(ctx context.Context, dir string, minAge time.Duration, match func(string) bool, result *CleanupResult) error {
	stat, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s: %w", dir, err)
	}
	if !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing unsafe cleanup directory: %s", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("list %s: %w", dir, err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !match(entry.Name()) || !entry.Type().IsRegular() {
			continue
		}
		file := filepath.Join(dir, entry.Name())
		fileInfo, err := os.Lstat(file)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !fileInfo.Mode().IsRegular() || time.Since(fileInfo.ModTime()) < minAge {
			continue
		}
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", file, err)
		}
		result.RemovedFiles++
	}
	return nil
}

func availableBytes(path string) (int64, bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, false
	}
	return int64(stat.Bavail) * stat.Bsize, true
}

// removeOldPanelTempDirs is shared by all standalone protocol installers.
// The common remover also checks nested file activity and recovery snapshots.
func removeOldPanelTempDirs(ctx context.Context, dir string, minAge time.Duration, result *CleanupResult) error {
	return removeOldKnownDirectories(ctx, dir, minAge, isAbandonedInstallerDir, result)
}
