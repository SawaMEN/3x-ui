//go:build linux

package swap

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

const (
	managedSwapPath = "/var/lib/3x-ui/swapfile"
	configPath      = "/etc/x-ui/swap.json"
	sysctlPath      = "/etc/sysctl.d/99-3x-ui-swap.conf"
	minSizeMiB      = 16
	maxSizeMiB      = 65536
)

var configMu sync.Mutex

func GetConfig() (Config, error) {
	configMu.Lock()
	defer configMu.Unlock()
	cfg := Config{
		SwapFile: SwapFileConfig{Path: managedSwapPath, Priority: 100},
		Zram:     ZramConfig{Priority: 200},
	}
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		if value, readErr := readSwappiness(); readErr == nil {
			cfg.Swappiness = value
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.SwapFile.Path == "" {
		cfg.SwapFile.Path = managedSwapPath
	}
	if cfg.SwapFile.Priority < 0 {
		cfg.SwapFile.Priority = 100
	}
	if cfg.Zram.Priority < 0 {
		cfg.Zram.Priority = 200
	}
	if cfg.Swappiness == 0 {
		if value, readErr := readSwappiness(); readErr == nil {
			cfg.Swappiness = value
		}
	}
	return cfg, nil
}

func GetStatus() (Status, error) {
	cfg, err := loadConfigLocked()
	if err != nil {
		return Status{}, err
	}
	areas, err := readProcSwaps()
	if err != nil {
		return Status{}, err
	}
	var total, used uint64
	zram := make([]ZramArea, 0)
	var zramAlgorithms []string
	for _, area := range areas {
		total += area.SizeBytes
		used += area.UsedBytes
	}
	for id := range zramIDs() {
		area, ok := readZram(id, areas, cfg)
		if ok {
			zram = append(zram, area)
			if len(zramAlgorithms) == 0 {
				zramAlgorithms = area.Algorithms
			}
		}
	}
	swappiness, _ := readSwappiness()
	ramTotal, ramAvailable := readMemoryInfo()
	installInfo, _ := GetZramInstallInfo()
	recommendation := buildRecommendation(ramTotal, len(zram) > 0 || cfg.Zram.Enabled, len(areas) > 0)
	status := Status{
		TotalBytes:        total,
		UsedBytes:         used,
		Swappiness:        swappiness,
		Areas:             areas,
		Zram:              zram,
		ZramAlgorithms:    zramAlgorithms,
		SwapFileConfig:    cfg.SwapFile,
		ZramConfig:        cfg.Zram,
		RAMTotalBytes:     ramTotal,
		RAMAvailableBytes: ramAvailable,
		CPUCount:          runtime.NumCPU(),
		Recommendation:    recommendation,
		ZramInstall:       installInfo,
	}
	return status, nil
}

func GetRecommendations() (Recommendation, error) {
	total, _ := readMemoryInfo()
	return buildRecommendation(total, len(zramIDs()) > 0, false), nil
}

func GetZramInstallInfo() (ZramInstallInfo, error) {
	id, version, packageManager, packageName := detectDistribution()
	configPath := "/etc/systemd/zram-generator.conf.d/60-3x-ui.conf"
	if id == "alpine" {
		configPath = "/etc/conf.d/zram-init"
	}
	info := ZramInstallInfo{
		Distribution:       id,
		Version:            version,
		PackageManager:     packageManager,
		Package:            packageName,
		Supported:          packageManager != "" && packageName != "",
		RecommendedPackage: packageName,
		ConfigPath:         configPath,
	}
	for _, candidate := range zramPackageCandidates(id, packageName) {
		version, installed := queryInstalledPackage(packageManager, candidate)
		info.InstalledPackages = append(info.InstalledPackages, ZramPackage{
			Name:      candidate,
			Version:   version,
			Installed: installed,
		})
		if candidate == packageName {
			info.RecommendedInstalled = installed
			info.RecommendedVersion = version
		}
	}
	info.Installed = info.RecommendedInstalled
	info.UsingGenerator = generatorInstalled()
	info.ActiveBackend = detectActiveZramBackend(info)
	if info.Supported {
		info.InstallCommand = installCommand(info.PackageManager, info.Package)
		info.ReinstallCommand = reinstallCommand(info.PackageManager, info.Package)
	}
	return info, nil
}

func zramPackageCandidates(distribution, recommended string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, 4)
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		result = append(result, name)
	}
	add(recommended)
	switch distribution {
	case "ubuntu", "debian":
		add("systemd-zram-generator")
		add("zram-config")
		add("zram-tools")
	case "fedora":
		add("zram-generator-defaults")
		add("zram-generator")
	case "rhel", "rocky", "alma":
		add("zram-generator")
		add("zram-generator-defaults")
	default:
		add(recommended)
	}
	return result
}

func queryInstalledPackage(manager, packageName string) (string, bool) {
	if manager == "" || packageName == "" {
		return "", false
	}
	switch manager {
	case "apt-get":
		if _, err := exec.LookPath("dpkg-query"); err != nil {
			return "", false
		}
		queryFormat := `$` + "{Status}\t" + "$" + "{Version}\n"
		output, err := exec.Command("dpkg-query", "-f="+queryFormat, packageName).Output()
		if err != nil {
			return "", false
		}
		fields := strings.Split(strings.TrimSpace(string(output)), "\t")
		if len(fields) < 2 || fields[0] != "install ok installed" {
			return "", false
		}
		return fields[1], true
	case "dnf", "yum", "zypper":
		if _, err := exec.LookPath("rpm"); err != nil {
			return "", false
		}
		output, err := exec.CommandContext(context.Background(), "rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", packageName).Output()
		if err != nil {
			return "", false
		}
		version := strings.TrimSpace(string(output))
		return version, version != ""
	case "pacman":
		if _, err := exec.LookPath("pacman"); err != nil {
			return "", false
		}
		output, err := exec.CommandContext(context.Background(), "pacman", "-Q", packageName).Output()
		if err != nil {
			return "", false
		}
		fields := strings.Fields(string(output))
		if len(fields) < 2 {
			return "", false
		}
		return fields[1], true
	case "apk":
		if _, err := exec.LookPath("apk"); err != nil {
			return "", false
		}
		output, err := exec.CommandContext(context.Background(), "apk", "info", "-e", packageName).Output()
		if err != nil {
			return "", false
		}
		line := strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0])
		if line == "" {
			return "", false
		}
		version := strings.TrimPrefix(line, packageName+"-")
		if version == line {
			version = ""
		}
		return version, true
	default:
		return "", false
	}
}

func detectActiveZramBackend(info ZramInstallInfo) string {
	if _, err := exec.LookPath("systemctl"); err == nil {
		if commandSucceeded("systemctl", "is-active", "--quiet", "zram-config.service") {
			return "zram-config"
		}
		if commandSucceeded("systemctl", "is-active", "--quiet", "zramswap.service") {
			return "zram-tools"
		}
	}
	if len(zramIDs()) == 0 {
		return ""
	}
	if info.UsingGenerator {
		return "systemd-zram-generator"
	}
	if info.Distribution == "alpine" && info.RecommendedInstalled {
		return "zram-init"
	}
	return ""
}

func commandSucceeded(command string, args ...string) bool {
	return exec.CommandContext(context.Background(), command, args...).Run() == nil
}

func installZramPackage(ctx context.Context, info ZramInstallInfo, reinstall bool) error {
	if !info.Supported {
		return fmt.Errorf("unsupported Linux distribution or package manager")
	}
	args := packageInstallArgs(info.PackageManager, info.Package)
	if reinstall && info.RecommendedInstalled {
		args = packageReinstallArgs(info.PackageManager, info.Package)
	}
	if len(args) == 0 {
		return fmt.Errorf("no supported package manager")
	}
	return runContext(ctx, args[0], args[1:]...)
}

func InstallZram(ctx context.Context) error {
	return configureZram(ctx, false)
}

func ReinstallZram(ctx context.Context) error {
	return configureZram(ctx, true)
}

func configureZram(ctx context.Context, reinstall bool) error {
	info, err := GetZramInstallInfo()
	if err != nil {
		return err
	}
	if !info.Supported {
		return fmt.Errorf("unsupported Linux distribution or package manager")
	}

	if info.ActiveBackend != "" && info.ActiveBackend != "systemd-zram-generator" && info.ActiveBackend != "zram-init" {
		return fmt.Errorf("another ZRAM backend is active: %s; disable it before switching to %s", info.ActiveBackend, info.Package)
	}

	if reinstall || !info.RecommendedInstalled {
		if err := installZramPackage(ctx, info, reinstall); err != nil {
			return err
		}
	}

	if info.Distribution == "alpine" {
		return installAlpineZram(ctx)
	}

	// Re-detect after package installation. Some distributions install the
	// generator in a path that was not present when the initial status was read.
	info, err = GetZramInstallInfo()
	if err != nil {
		return err
	}
	if !info.UsingGenerator {
		return fmt.Errorf("zram-generator was installed, but the systemd generator was not found")
	}

	configDir := filepath.Dir(info.ConfigPath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	config := "[zram0]\n" +
		"zram-size = min(ram / 2, 8192)\n" +
		"swap-priority = 100\n"
	if err := os.WriteFile(info.ConfigPath, []byte(config), 0o644); err != nil {
		return err
	}

	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("systemd is required to activate zram: %w", err)
	}

	if err := runContext(ctx, systemctl, "daemon-reload"); err != nil {
		return err
	}
	if err := runContext(ctx, systemctl, "start", "dev-zram0.swap"); err != nil {
		return fmt.Errorf("failed to activate zram swap: %w", err)
	}
	if !isSwapActive("/dev/zram0") {
		return fmt.Errorf("zram generator completed, but /dev/zram0 is not active swap")
	}
	return nil
}

func installAlpineZram(ctx context.Context) error {
	total, _ := readMemoryInfo()
	recommended := int(total / 1024 / 1024 / 2)
	if recommended < 256 {
		recommended = 256
	}
	if recommended > 8192 {
		recommended = 8192
	}
	config := "load_on_start=yes\n" +
		"unload_on_stop=yes\n" +
		"num_devices=1\n" +
		"type0=swap\n" +
		"size0=" + strconv.Itoa(recommended) + "\n"
	if err := os.MkdirAll("/etc/conf.d", 0o755); err != nil {
		return err
	}
	if err := os.WriteFile("/etc/conf.d/zram-init", []byte(config), 0o644); err != nil {
		return err
	}
	if _, err := exec.LookPath("rc-update"); err == nil {
		if err := runContext(ctx, "rc-update", "add", "zram-init", "default"); err != nil {
			return err
		}
	}
	if _, err := exec.LookPath("rc-service"); err == nil {
		return runContext(ctx, "rc-service", "zram-init", "restart")
	}
	return nil
}

func buildRecommendation(totalRAM uint64, zramEnabled, diskSwapEnabled bool) Recommendation {
	if totalRAM == 0 {
		return Recommendation{}
	}
	ramMiB := int(totalRAM / 1024 / 1024)
	zramMiB := ramMiB / 2
	if zramMiB < 1024 {
		zramMiB = 1024
	}
	if zramMiB > 8192 {
		zramMiB = 8192
	}
	var swapMiB int
	switch {
	case ramMiB < 2048:
		swapMiB = 2048
	case ramMiB <= 8192:
		swapMiB = ramMiB
	default:
		swapMiB = 4096
	}
	swappiness := 60
	mode := "без ZRAM"
	if zramEnabled {
		swappiness = 100
		mode = "с ZRAM"
	} else if diskSwapEnabled {
		swappiness = 60
	}
	return Recommendation{
		TotalRAMBytes:         totalRAM,
		CPUCount:              runtime.NumCPU(),
		RecommendedSwapMiB:    swapMiB,
		RecommendedZramMiB:    zramMiB,
		RecommendedSwappiness: swappiness,
		Reason:                fmt.Sprintf("Рекомендация 3x-ui для %d МиБ RAM (%s): ZRAM ≈ %d МиБ, swap-файл ≈ %d МиБ, vm.swappiness=%d.", ramMiB, mode, zramMiB, swapMiB, swappiness),
	}
}

func readMemoryInfo() (uint64, uint64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	var total, available uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = value * 1024
		case "MemAvailable:":
			available = value * 1024
		}
	}
	return total, available
}

func detectDistribution() (string, string, string, string) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", "", "", ""
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		values[parts[0]] = strings.Trim(strings.TrimSpace(parts[1]), "\"")
	}
	id := strings.ToLower(values["ID"])
	version := values["VERSION_ID"]
	like := strings.ToLower(values["ID_LIKE"])
	switch {
	case id == "fedora":
		return id, version, firstExistingCommand("dnf", "yum"), "zram-generator-defaults"
	case strings.Contains(like, "fedora") || strings.Contains(like, "rhel") || id == "rhel" || strings.Contains(id, "rocky") || strings.Contains(id, "alma"):
		return id, version, firstExistingCommand("dnf", "yum"), "zram-generator"
	case id == "ubuntu" || id == "debian" || strings.Contains(like, "debian"):
		return id, version, "apt-get", "systemd-zram-generator"
	case id == "arch" || strings.Contains(like, "arch"):
		return id, version, "pacman", "zram-generator"
	case strings.Contains(id, "opensuse") || strings.Contains(like, "suse"):
		return id, version, "zypper", "zram-generator"
	case id == "alpine":
		return id, version, "apk", "zram-init"
	default:
		return id, version, firstExistingCommand("apt-get", "dnf", "yum", "pacman", "zypper", "apk"), "systemd-zram-generator"
	}
}

func firstExistingCommand(commands ...string) string {
	for _, command := range commands {
		if _, err := exec.LookPath(command); err == nil {
			return command
		}
	}
	return ""
}

func generatorInstalled() bool {
	for _, path := range []string{
		"/usr/lib/systemd/system-generators/zram-generator",
		"/lib/systemd/system-generators/zram-generator",
	} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func installCommand(manager, packageName string) string {
	switch manager {
	case "apt-get":
		return "apt-get install -y " + packageName
	case "dnf":
		return "dnf install -y " + packageName
	case "yum":
		return "yum install -y " + packageName
	case "pacman":
		return "pacman -S --noconfirm " + packageName
	case "zypper":
		return "zypper --non-interactive install " + packageName
	case "apk":
		return "apk add --no-cache " + packageName
	default:
		return ""
	}
}

func reinstallCommand(manager, packageName string) string {
	switch manager {
	case "apt-get":
		return "apt-get install -y --reinstall " + packageName
	case "dnf":
		return "dnf reinstall -y " + packageName
	case "yum":
		return "yum reinstall -y " + packageName
	case "pacman":
		return "pacman -S --noconfirm " + packageName
	case "zypper":
		return "zypper --non-interactive install --force " + packageName
	case "apk":
		return "apk fix --no-cache " + packageName
	default:
		return ""
	}
}

func packageInstallArgs(manager, packageName string) []string {
	switch manager {
	case "apt-get":
		return []string{"apt-get", "install", "-y", packageName}
	case "dnf":
		return []string{"dnf", "install", "-y", packageName}
	case "yum":
		return []string{"yum", "install", "-y", packageName}
	case "pacman":
		return []string{"pacman", "-S", "--noconfirm", packageName}
	case "zypper":
		return []string{"zypper", "--non-interactive", "install", packageName}
	case "apk":
		return []string{"apk", "add", "--no-cache", packageName}
	default:
		return nil
	}
}

func packageReinstallArgs(manager, packageName string) []string {
	switch manager {
	case "apt-get":
		return []string{"apt-get", "install", "-y", "--reinstall", packageName}
	case "dnf":
		return []string{"dnf", "reinstall", "-y", packageName}
	case "yum":
		return []string{"yum", "reinstall", "-y", packageName}
	case "pacman":
		return []string{"pacman", "-S", "--noconfirm", packageName}
	case "zypper":
		return []string{"zypper", "--non-interactive", "install", "--force", packageName}
	case "apk":
		return []string{"apk", "fix", "--no-cache", packageName}
	default:
		return nil
	}
}

func Summary() (uint64, uint64, error) {
	areas, err := readProcSwaps()
	if err != nil {
		return 0, 0, err
	}
	var total, used uint64
	for _, area := range areas {
		total += area.SizeBytes
		used += area.UsedBytes
	}
	return total, used, nil
}

func CreateSwap(sizeMiB, priority int) error {
	if err := validateSizePriority(sizeMiB, priority); err != nil {
		return err
	}
	configMu.Lock()
	defer configMu.Unlock()

	if isSwapActive(managedSwapPath) {
		return fmt.Errorf("managed swap is already active")
	}
	if _, err := os.Stat(managedSwapPath); err == nil {
		return fmt.Errorf("managed swap file already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(managedSwapPath), 0o755); err != nil {
		return err
	}
	if err := createSizedFile(managedSwapPath, int64(sizeMiB)*1024*1024); err != nil {
		return err
	}
	if err := os.Chmod(managedSwapPath, 0o600); err != nil {
		_ = os.Remove(managedSwapPath)
		return err
	}
	if err := run("mkswap", managedSwapPath); err != nil {
		_ = os.Remove(managedSwapPath)
		return err
	}
	if err := run("swapon", "-p", strconv.Itoa(priority), managedSwapPath); err != nil {
		_ = os.Remove(managedSwapPath)
		return err
	}
	cfg, _ := loadConfigLocked()
	cfg.SwapFile = SwapFileConfig{Enabled: true, Path: managedSwapPath, SizeBytes: uint64(sizeMiB) * 1024 * 1024, Priority: priority}
	if err := saveConfig(cfg); err != nil {
		_ = run("swapoff", managedSwapPath)
		_ = os.Remove(managedSwapPath)
		return err
	}
	if err := persistFstab(managedSwapPath, priority, true); err != nil {
		_ = run("swapoff", managedSwapPath)
		_ = os.Remove(managedSwapPath)
		return err
	}
	return nil
}

func DeleteSwap() error {
	configMu.Lock()
	defer configMu.Unlock()
	cfg, err := loadConfigLocked()
	if err != nil {
		return err
	}
	path := cfg.SwapFile.Path
	if path == "" {
		path = managedSwapPath
	}
	if path != managedSwapPath {
		return fmt.Errorf("refusing to remove unmanaged swap path")
	}
	if isSwapActive(path) {
		if err := run("swapoff", path); err != nil {
			return err
		}
	}
	if err := persistFstab(path, 0, false); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	cfg.SwapFile = SwapFileConfig{Path: managedSwapPath, Priority: 100}
	return saveConfig(cfg)
}

func CreateZram(sizeMiB int, algorithm string, streams int, memoryLimitMiB, priority int) error {
	if err := validateSizePriority(sizeMiB, priority); err != nil {
		return err
	}
	if streams < 1 || streams > 64 {
		return fmt.Errorf("zram streams must be between 1 and 64")
	}
	if memoryLimitMiB < 0 || memoryLimitMiB > maxSizeMiB {
		return fmt.Errorf("zram memory limit must be between 0 and %d MiB", maxSizeMiB)
	}
	configMu.Lock()
	defer configMu.Unlock()
	cfg, err := loadConfigLocked()
	if err != nil {
		return err
	}
	if cfg.Zram.Enabled && cfg.Zram.Device != "" && isSwapActive(cfg.Zram.Device) {
		return fmt.Errorf("managed zram is already active")
	}
	id, err := addZram()
	if err != nil {
		return err
	}
	device := fmt.Sprintf("/dev/zram%d", id)
	sys := fmt.Sprintf("/sys/block/zram%d", id)
	algorithms := parseAlgorithms(readText(filepath.Join(sys, "comp_algorithm")))
	algorithm = strings.TrimSpace(algorithm)
	if algorithm == "" {
		algorithm = chooseDefaultAlgorithm(algorithms)
	}
	if !contains(algorithms, algorithm) {
		_ = os.WriteFile(filepath.Join(sys, "reset"), []byte("1"), 0o644)
		_ = os.WriteFile("/sys/class/zram-control/hot_remove", []byte(strconv.Itoa(id)), 0o644)
		return fmt.Errorf("unsupported zram compression algorithm %q", algorithm)
	}
	cleanup := func() {
		_ = run("swapoff", device)
		_ = os.WriteFile(filepath.Join(sys, "reset"), []byte("1"), 0o644)
		_ = os.WriteFile("/sys/class/zram-control/hot_remove", []byte(strconv.Itoa(id)), 0o644)
	}
	if err := writeSysfs(filepath.Join(sys, "comp_algorithm"), algorithm); err != nil {
		cleanup()
		return err
	}
	if err := writeSysfs(filepath.Join(sys, "max_comp_streams"), strconv.Itoa(streams)); err != nil {
		cleanup()
		return err
	}
	if err := writeSysfs(filepath.Join(sys, "mem_limit"), formatMiB(memoryLimitMiB)); err != nil {
		cleanup()
		return err
	}
	if err := writeSysfs(filepath.Join(sys, "disksize"), formatMiB(sizeMiB)); err != nil {
		cleanup()
		return err
	}
	if err := run("mkswap", device); err != nil {
		cleanup()
		return err
	}
	if err := run("swapon", "-p", strconv.Itoa(priority), device); err != nil {
		cleanup()
		return err
	}
	cfg.Zram = ZramConfig{
		Enabled:          true,
		Device:           device,
		SizeBytes:        uint64(sizeMiB) * 1024 * 1024,
		Algorithm:        algorithm,
		Streams:          uint64(streams),
		MemoryLimitBytes: uint64(memoryLimitMiB) * 1024 * 1024,
		Priority:         priority,
	}
	return saveConfig(cfg)
}

func DeleteZram() error {
	configMu.Lock()
	defer configMu.Unlock()
	cfg, err := loadConfigLocked()
	if err != nil {
		return err
	}
	if cfg.Zram.Device == "" {
		return nil
	}
	device := cfg.Zram.Device
	id := zramIDFromDevice(device)
	if id < 0 {
		return fmt.Errorf("managed zram device is invalid")
	}
	if cfg.Zram.Device != fmt.Sprintf("/dev/zram%d", id) {
		return fmt.Errorf("refusing to remove unmanaged zram device")
	}
	if isSwapActive(device) {
		if err := run("swapoff", device); err != nil {
			return err
		}
	}
	sys := fmt.Sprintf("/sys/block/zram%d", id)
	if err := writeSysfs(filepath.Join(sys, "reset"), "1"); err != nil {
		return err
	}
	if err := writeSysfs("/sys/class/zram-control/hot_remove", strconv.Itoa(id)); err != nil {
		return err
	}
	cfg.Zram = ZramConfig{Priority: 200}
	return saveConfig(cfg)
}

func SetSwappiness(value int) error {
	if value < 0 || value > 200 {
		return fmt.Errorf("swappiness must be between 0 and 200")
	}
	configMu.Lock()
	defer configMu.Unlock()
	if err := writeSysfs("/proc/sys/vm/swappiness", strconv.Itoa(value)); err != nil {
		return err
	}
	cfg, err := loadConfigLocked()
	if err != nil {
		return err
	}
	cfg.Swappiness = value
	if err := saveConfig(cfg); err != nil {
		return err
	}
	return writeSysctl(value)
}

func Ensure(ctx context.Context) error {
	_ = ctx
	configMu.Lock()
	defer configMu.Unlock()
	cfg, err := loadConfigLocked()
	if err != nil {
		return err
	}
	if cfg.Swappiness >= 0 {
		_ = writeSysfs("/proc/sys/vm/swappiness", strconv.Itoa(cfg.Swappiness))
	}
	if cfg.SwapFile.Enabled {
		path := cfg.SwapFile.Path
		if path == "" {
			path = managedSwapPath
		}
		if !isSwapActive(path) {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return err
				}
				if err := createSizedFile(path, int64(cfg.SwapFile.SizeBytes)); err != nil {
					return err
				}
				_ = os.Chmod(path, 0o600)
				if err := run("mkswap", path); err != nil {
					return err
				}
			}
			if err := run("swapon", "-p", strconv.Itoa(cfg.SwapFile.Priority), path); err != nil {
				return err
			}
		}
		_ = persistFstab(path, cfg.SwapFile.Priority, true)
	}
	if cfg.Zram.Enabled {
		if !isSwapActive(cfg.Zram.Device) {
			id, err := recreateZram(cfg.Zram)
			if err != nil {
				return err
			}
			cfg.Zram.Device = fmt.Sprintf("/dev/zram%d", id)
			if err := saveConfig(cfg); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadConfigLocked() (Config, error) {
	cfg := Config{
		SwapFile: SwapFileConfig{Path: managedSwapPath, Priority: 100},
		Zram:     ZramConfig{Priority: 200},
	}
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		if value, readErr := readSwappiness(); readErr == nil {
			cfg.Swappiness = value
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.SwapFile.Path == "" {
		cfg.SwapFile.Path = managedSwapPath
	}
	return cfg, nil
}

func saveConfig(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}

func validateSizePriority(sizeMiB, priority int) error {
	if sizeMiB < minSizeMiB || sizeMiB > maxSizeMiB {
		return fmt.Errorf("swap size must be between %d and %d MiB", minSizeMiB, maxSizeMiB)
	}
	if priority < 0 || priority > 32767 {
		return fmt.Errorf("swap priority must be between 0 and 32767")
	}
	return nil
}

func createSizedFile(path string, size int64) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := run("fallocate", "-l", strconv.FormatInt(size, 10), path); err == nil {
		return nil
	}
	return run("dd", "if=/dev/zero", "of="+path, "bs=1M", "count="+strconv.FormatInt(size/(1024*1024), 10), "status=none")
}

func isSwapActive(path string) bool {
	if path == "" {
		return false
	}
	areas, err := readProcSwaps()
	if err != nil {
		return false
	}
	for _, area := range areas {
		if area.Path == path {
			return true
		}
	}
	return false
}

func readProcSwaps() ([]SwapArea, error) {
	f, err := os.Open("/proc/swaps")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var areas []SwapArea
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 {
			continue
		}
		size, _ := strconv.ParseUint(fields[2], 10, 64)
		used, _ := strconv.ParseUint(fields[3], 10, 64)
		priority, _ := strconv.Atoi(fields[4])
		areas = append(areas, SwapArea{
			Path: fields[0], Type: fields[1], SizeBytes: size * 1024,
			UsedBytes: used * 1024, Priority: priority, Active: true,
			Managed: fields[0] == managedSwapPath || fields[0] == managedZramDevice(),
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return areas, nil
}

func zramIDs() []int {
	entries, err := filepath.Glob("/sys/block/zram*")
	if err != nil {
		return nil
	}
	out := make([]int, 0, len(entries))
	for _, path := range entries {
		id, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(path), "zram"))
		if err == nil {
			out = append(out, id)
		}
	}
	return out
}

func readZram(id int, areas []SwapArea, cfg Config) (ZramArea, bool) {
	sys := fmt.Sprintf("/sys/block/zram%d", id)
	if _, err := os.Stat(sys); err != nil {
		return ZramArea{}, false
	}
	device := fmt.Sprintf("/dev/zram%d", id)
	size := readUint(filepath.Join(sys, "disksize"))
	compressed := readUint(filepath.Join(sys, "compr_data_size"))
	memUsed := readUint(filepath.Join(sys, "mem_used_total"))
	limit := readUint(filepath.Join(sys, "mem_limit"))
	streams := readUint(filepath.Join(sys, "max_comp_streams"))
	alg := parseSelectedAlgorithm(readText(filepath.Join(sys, "comp_algorithm")))
	active := false
	priority := 0
	used := uint64(0)
	for _, area := range areas {
		if area.Path == device {
			active = area.Active
			priority = area.Priority
			used = area.UsedBytes
			break
		}
	}
	managed := cfg.Zram.Device == device
	return ZramArea{
		Device: device, ID: id, SizeBytes: size, UsedBytes: used,
		CompressedBytes: compressed, MemoryUsedBytes: memUsed,
		MemoryLimitBytes: limit, Algorithm: alg,
		Algorithms: parseAlgorithms(readText(filepath.Join(sys, "comp_algorithm"))),
		Streams:    streams, Priority: priority, Active: active, Managed: managed,
	}, true
}

func chooseDefaultAlgorithm(values []string) string {
	for _, preferred := range []string{"zstd", "lz4", "lzo-rle", "lzo"} {
		if contains(values, preferred) {
			return preferred
		}
	}
	if len(values) > 0 {
		return values[0]
	}
	return "zstd"
}

func addZram() (int, error) {
	data, err := os.ReadFile("/sys/class/zram-control/hot_add")
	if err != nil {
		return -1, err
	}
	id, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return -1, err
	}
	return id, nil
}

func recreateZram(cfg ZramConfig) (int, error) {
	algorithm := cfg.Algorithm
	id, err := addZram()
	if err != nil {
		return -1, err
	}
	sys := fmt.Sprintf("/sys/block/zram%d", id)
	if algorithm == "" {
		algorithm = chooseDefaultAlgorithm(readZramCompAlgorithms(sys))
	}
	if err := writeSysfs(filepath.Join(sys, "comp_algorithm"), algorithm); err != nil {
		return -1, err
	}
	if cfg.Streams > 0 {
		if err := writeSysfs(filepath.Join(sys, "max_comp_streams"), strconv.FormatUint(cfg.Streams, 10)); err != nil {
			return -1, err
		}
	}
	if cfg.MemoryLimitBytes > 0 {
		if err := writeSysfs(filepath.Join(sys, "mem_limit"), strconv.FormatUint(cfg.MemoryLimitBytes, 10)); err != nil {
			return -1, err
		}
	}
	if err := writeSysfs(filepath.Join(sys, "disksize"), strconv.FormatUint(cfg.SizeBytes, 10)); err != nil {
		return -1, err
	}
	device := fmt.Sprintf("/dev/zram%d", id)
	if err := run("mkswap", device); err != nil {
		return -1, err
	}
	if err := run("swapon", "-p", strconv.Itoa(cfg.Priority), device); err != nil {
		return -1, err
	}
	return id, nil
}

func readZramCompAlgorithms(path string) []string {
	return parseAlgorithms(readText(filepath.Join(path, "comp_algorithm")))
}

func managedZramDevice() string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	var cfg Config
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg.Zram.Device
}

func readSwappiness() (int, error) {
	data, err := os.ReadFile("/proc/sys/vm/swappiness")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func writeSysctl(value int) error {
	if err := os.MkdirAll(filepath.Dir(sysctlPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(sysctlPath, []byte("vm.swappiness = "+strconv.Itoa(value)+"\n"), 0o644)
}

func persistFstab(path string, priority int, enable bool) error {
	data, err := os.ReadFile("/etc/fstab")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	filtered := lines[:0]
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == path {
			continue
		}
		filtered = append(filtered, line)
	}
	if enable {
		filtered = append(filtered, fmt.Sprintf("%s none swap defaults,nofail,pri=%d 0 0", path, priority))
	}
	return os.WriteFile("/etc/fstab", []byte(strings.Join(filtered, "\n")), 0o644)
}

func writeSysfs(path, value string) error {
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		return err
	}
	return nil
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readUint(path string) uint64 {
	value, _ := strconv.ParseUint(strings.TrimSpace(readText(path)), 10, 64)
	return value
}

func parseAlgorithms(raw string) []string {
	fields := strings.Fields(raw)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, "[]")
		if field != "" && !contains(out, field) {
			out = append(out, field)
		}
	}
	return out
}

func parseSelectedAlgorithm(raw string) string {
	for _, field := range strings.Fields(raw) {
		if strings.HasPrefix(field, "[") && strings.HasSuffix(field, "]") {
			return strings.Trim(field, "[]")
		}
	}
	values := parseAlgorithms(raw)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

func formatMiB(mib int) string {
	if mib <= 0 {
		return "0"
	}
	return strconv.Itoa(mib) + "M"
}

func zramIDFromDevice(device string) int {
	base := filepath.Base(device)
	if !strings.HasPrefix(base, "zram") {
		return -1
	}
	id, err := strconv.Atoi(strings.TrimPrefix(base, "zram"))
	if err != nil {
		return -1
	}
	return id
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func run(name string, args ...string) error {
	return runContext(context.Background(), name, args...)
}

func runContext(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
