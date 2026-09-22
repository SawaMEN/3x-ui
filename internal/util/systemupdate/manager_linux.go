//go:build linux

package systemupdate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

const commandTimeout = 30 * time.Minute

var updateMu sync.Mutex

type distroInfo struct {
	id      string
	version string
	manager string
}

var packageVersionPattern = regexp.MustCompile(`^(.+)-([0-9][^[:space:]]*)[[:space:]]+<[[:space:]]+(.+)$`)

func GetStatus(ctx context.Context) (Status, error) {
	info := detectDistribution()
	status := Status{
		Distribution:   info.id,
		Version:        info.version,
		PackageManager: info.manager,
		Supported:      info.manager != "",
		RunningAsRoot:  os.Geteuid() == 0,
	}
	if !status.Supported {
		return status, fmt.Errorf("unsupported Linux distribution or package manager")
	}

	upgrades, err := listAvailableUpdates(ctx, info.manager)
	if err != nil {
		return status, err
	}

	required := requiredPackages(info.id)
	for _, name := range required {
		installedVersion, installed := installedPackageVersion(info.manager, name)
		availableVersion := upgrades[name]
		status.Packages = append(status.Packages, PackageStatus{
			Name:             name,
			InstalledVersion: installedVersion,
			AvailableVersion: availableVersion,
			Installed:        installed,
			Required:         true,
			UpdateAvailable:  installed && availableVersion != "" && availableVersion != installedVersion,
		})
		if !installed {
			status.MissingPackages = true
		}
	}

	kernelPackages := make([]PackageStatus, 0)
	for name, version := range upgrades {
		if !isKernelPackage(name) {
			continue
		}
		installedVersion, installed := installedPackageVersion(info.manager, name)
		kernelPackages = append(kernelPackages, PackageStatus{
			Name:             name,
			InstalledVersion: installedVersion,
			AvailableVersion: version,
			Installed:        installed,
			Kernel:           true,
			UpdateAvailable: packageUpdateAvailable(installed, installedVersion, version),
		})
	}
	sort.Slice(kernelPackages, func(i, j int) bool { return kernelPackages[i].Name < kernelPackages[j].Name })
	status.Packages = append(status.Packages, kernelPackages...)

	kernel := KernelStatus{RunningVersion: runtimeKernelVersion()}
	kernel.RebootRequired = rebootRequired(info.manager)

	for _, item := range kernelPackages {
		if !item.UpdateAvailable {
			continue
		}
		kernel.UpdateAvailable = true
		kernel.PackageNames = append(kernel.PackageNames, item.Name)
		if kernel.AvailableVersion == "" {
			kernel.AvailableVersion = item.AvailableVersion
		} else if strings.Compare(kernel.AvailableVersion, item.AvailableVersion) < 0 {
			kernel.AvailableVersion = item.AvailableVersion
		}
	}
	sort.Strings(kernel.PackageNames)
	status.Kernel = kernel
	status.UpdatesAvailable = kernel.UpdateAvailable
	for _, item := range status.Packages {
		if item.UpdateAvailable {
			status.UpdatesAvailable = true
		}
	}
	status.CanUpdate = status.RunningAsRoot && (status.UpdatesAvailable || status.MissingPackages)

	if info.manager == "pacman" {
		status.Notes = append(status.Notes, "Arch Linux требует полного обновления системы через pacman -Syu; частичные обновления не поддерживаются.")
	}
	if kernel.UpdateAvailable {
		status.Notes = append(status.Notes, "После обновления ядра потребуется перезагрузка сервера, чтобы запустить новое ядро.")
	}
	if kernel.RebootRequired {
		status.Notes = append(status.Notes, "Для применения уже установленного обновления системы требуется перезагрузка.")
	}
	if !status.RunningAsRoot {
		status.Notes = append(status.Notes, "Для установки обновлений панель должна работать с root-правами.")
	}
	return status, nil
}

func Refresh(ctx context.Context) (Status, error) {
	info := detectDistribution()
	if info.manager == "" {
		return Status{}, fmt.Errorf("unsupported Linux distribution or package manager")
	}
	if err := refreshPackageDatabase(ctx, info.manager); err != nil {
		return Status{}, err
	}
	return GetStatus(ctx)
}

func newUpdateContext(_ context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), commandTimeout)
}

func Apply(ctx context.Context) (UpdateResult, error) {
	updateMu.Lock()
	defer updateMu.Unlock()

	if os.Geteuid() != 0 {
		return UpdateResult{}, fmt.Errorf("system updates require root privileges")
	}

	info := detectDistribution()
	if info.manager == "" {
		return UpdateResult{}, fmt.Errorf("unsupported Linux distribution or package manager")
	}

	updateCtx, cancel := newUpdateContext(ctx)
	defer cancel()

	if err := refreshPackageDatabase(updateCtx, info.manager); err != nil {
		return UpdateResult{}, err
	}
	status, err := GetStatus(updateCtx)
	if err != nil {
		return UpdateResult{}, err
	}

	packageNames := make([]string, 0)
	seen := map[string]bool{}
	for _, item := range status.Packages {
		if item.Required && !seen[item.Name] && (!item.Installed || item.UpdateAvailable) {
			seen[item.Name] = true
			packageNames = append(packageNames, item.Name)
		}
		if item.Kernel && item.UpdateAvailable && !seen[item.Name] {
			seen[item.Name] = true
			packageNames = append(packageNames, item.Name)
		}
	}
	sort.Strings(packageNames)

	var args []string
	switch info.manager {
	case "apt-get":
		if len(packageNames) == 0 {
			return UpdateResult{Updated: false, RebootRequired: status.Kernel.RebootRequired}, nil
		}
		args = append([]string{"apt-get", "install", "-y", "--no-install-recommends"}, packageNames...)
	case "dnf":
		if len(packageNames) == 0 {
			return UpdateResult{Updated: false, RebootRequired: status.Kernel.RebootRequired}, nil
		}
		args = append([]string{"dnf", "install", "-y"}, packageNames...)
	case "yum":
		if len(packageNames) == 0 {
			return UpdateResult{Updated: false, RebootRequired: status.Kernel.RebootRequired}, nil
		}
		args = append([]string{"yum", "install", "-y"}, packageNames...)
	case "zypper":
		if len(packageNames) == 0 {
			return UpdateResult{Updated: false, RebootRequired: status.Kernel.RebootRequired}, nil
		}
		args = append([]string{"zypper", "--non-interactive", "install"}, packageNames...)
	case "apk":
		if len(packageNames) == 0 {
			return UpdateResult{Updated: false, RebootRequired: status.Kernel.RebootRequired}, nil
		}
		args = append([]string{"apk", "add", "--no-cache", "--upgrade"}, packageNames...)
	case "pacman":
		args = append([]string{"pacman", "-Syu", "--noconfirm", "--needed"}, packageNames...)
	default:
		return UpdateResult{}, fmt.Errorf("unsupported package manager: %s", info.manager)
	}

	output, err := runCommand(updateCtx, args[0], args[1:]...)
	result := UpdateResult{
		Updated:        err == nil,
		Output:         truncateOutput(output, 20000),
		RebootRequired: status.Kernel.UpdateAvailable || status.Kernel.RebootRequired,
	}
	if err != nil {
		result.Error = truncateOutput(err.Error(), 4000)
		return result, fmt.Errorf("system update failed: %s", result.Error)
	}
	return result, nil
}

func detectDistribution() distroInfo {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return distroInfo{}
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
	like := strings.ToLower(values["ID_LIKE"])
	switch {
	case id == "ubuntu" || id == "debian" || strings.Contains(like, "debian"):
		return distroInfo{id: id, version: values["VERSION_ID"], manager: "apt-get"}
	case id == "fedora":
		return distroInfo{id: id, version: values["VERSION_ID"], manager: "dnf"}
	case id == "amzn" || id == "rhel" || id == "almalinux" || id == "rocky" || id == "ol" || strings.Contains(like, "rhel") || strings.Contains(like, "fedora"):
		if commandExists("dnf") {
			return distroInfo{id: id, version: values["VERSION_ID"], manager: "dnf"}
		}
		return distroInfo{id: id, version: values["VERSION_ID"], manager: "yum"}
	case id == "centos":
		if commandExists("dnf") {
			return distroInfo{id: id, version: values["VERSION_ID"], manager: "dnf"}
		}
		return distroInfo{id: id, version: values["VERSION_ID"], manager: "yum"}
	case id == "arch" || strings.Contains(like, "arch") || id == "manjaro" || id == "parch":
		return distroInfo{id: id, version: values["VERSION_ID"], manager: "pacman"}
	case id == "opensuse-tumbleweed" || id == "opensuse-leap" || strings.Contains(id, "opensuse") || strings.Contains(like, "suse"):
		return distroInfo{id: id, version: values["VERSION_ID"], manager: "zypper"}
	case id == "alpine":
		return distroInfo{id: id, version: values["VERSION_ID"], manager: "apk"}
	default:
		if commandExists("apt-get") {
			return distroInfo{id: id, version: values["VERSION_ID"], manager: "apt-get"}
		}
		return distroInfo{}
	}
}

func requiredPackages(distribution string) []string {
	var packages []string
	switch distribution {
	case "ubuntu", "debian", "armbian":
		packages = []string{"cron", "curl", "tar", "tzdata", "socat", "ca-certificates", "openssl", "util-linux"}
	case "fedora", "amzn", "rhel", "almalinux", "rocky", "ol", "centos":
		packages = []string{"cronie", "curl", "tar", "tzdata", "socat", "ca-certificates", "openssl", "util-linux"}
	case "arch", "manjaro", "parch":
		packages = []string{"cronie", "curl", "tar", "tzdata", "socat", "ca-certificates", "openssl", "util-linux"}
	case "opensuse-tumbleweed", "opensuse-leap":
		packages = []string{"cron", "curl", "tar", "timezone", "socat", "ca-certificates", "openssl", "util-linux"}
	case "alpine":
		packages = []string{"dcron", "curl", "tar", "tzdata", "socat", "ca-certificates", "openssl", "util-linux"}
	default:
		packages = []string{"cron", "curl", "tar", "tzdata", "socat", "ca-certificates", "openssl", "util-linux"}
	}

	addPackage := func(name string) {
		if name == "" {
			return
		}
		for _, existing := range packages {
			if existing == name {
				return
			}
		}
		packages = append(packages, name)
	}

	// The panel can use pg_dump/pg_restore when PostgreSQL is selected. Mirror
	// the package names used by install.sh/update.sh so the system update page
	// can repair a missing client as well.
	if config.GetDBKind() == "postgres" {
		switch distribution {
		case "ubuntu", "debian", "armbian", "alpine":
			addPackage("postgresql-client")
		case "fedora", "amzn", "rhel", "almalinux", "rocky", "ol", "centos",
			"arch", "manjaro", "parch", "opensuse-tumbleweed", "opensuse-leap":
			addPackage("postgresql")
		}
	}

	// Fail2ban is optional. When the module is installed, nftables is needed on
	// minimal images because recent fail2ban defaults use its nftables action.
	if commandExists("fail2ban-client") {
		addPackage("fail2ban")
		addPackage("nftables")
	}

	return packages
}
func packageUpdateAvailable(installed bool, installedVersion, availableVersion string) bool {
	return installed && availableVersion != "" && installedVersion != "" && installedVersion != availableVersion
}

func refreshPackageDatabase(ctx context.Context, manager string) error {
	switch manager {
	case "apt-get":
		_, err := runCommand(ctx, "apt-get", "update")
		return err
	case "dnf":
		_, err := runCommand(ctx, "dnf", "makecache", "-y")
		return err
	case "yum":
		_, err := runCommand(ctx, "yum", "makecache", "-y")
		return err
	case "zypper":
		_, err := runCommand(ctx, "zypper", "refresh")
		return err
	case "pacman":
		return nil
	case "apk":
		_, err := runCommand(ctx, "apk", "update")
		return err
	default:
		return fmt.Errorf("unsupported package manager: %s", manager)
	}
}

func listAvailableUpdates(ctx context.Context, manager string) (map[string]string, error) {
	switch manager {
	case "apt-get":
		output, err := runCommand(ctx, "apt", "list", "--upgradable")
		return parseAptUpdates(output), err
	case "dnf", "yum":
		output, err := runCommand(ctx, manager, "-q", "check-update")
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 100 {
				if strings.TrimSpace(output) == "" {
					return nil, err
				}
			}
		}
		return parseDnfUpdates(output), nil
	case "zypper":
		output, err := runCommand(ctx, "zypper", "--non-interactive", "list-updates")
		if err != nil && strings.TrimSpace(output) == "" {
			return nil, err
		}
		return parseZypperUpdates(output), nil
	case "pacman":
		if commandExists("checkupdates") {
			output, err := runCommand(ctx, "checkupdates")
			if err != nil && strings.TrimSpace(output) == "" {
				return nil, err
			}
			return parsePacmanUpdates(output), nil
		}
		output, err := runCommand(ctx, "pacman", "-Qu")
		if err != nil && strings.TrimSpace(output) == "" {
			return nil, err
		}
		return parsePacmanQueryUpdates(output), nil
	case "apk":
		output, err := runCommand(ctx, "apk", "version", "-l", "<")
		if err != nil {
			return nil, err
		}
		return parseApkUpdates(output), nil
	default:
		return nil, fmt.Errorf("unsupported package manager: %s", manager)
	}
}

func installedPackageVersion(manager, name string) (string, bool) {
	switch manager {
	case "apt-get":
		output, err := exec.CommandContext(context.Background(), "dpkg-query", "-W", "-f=${Status}\t${Version}\n", name).Output()
		if err != nil {
			return "", false
		}
		fields := strings.Split(strings.TrimSpace(string(output)), "\t")
		if len(fields) < 2 || fields[0] != "install ok installed" {
			return "", false
		}
		return fields[1], true
	case "dnf", "yum", "zypper":
		output, err := exec.CommandContext(context.Background(), "rpm", "-q", "--qf", "%{VERSION}-%{RELEASE}", name).Output()
		if err != nil {
			return "", false
		}
		version := strings.TrimSpace(string(output))
		return version, version != ""
	case "pacman":
		output, err := exec.CommandContext(context.Background(), "pacman", "-Q", name).Output()
		if err != nil {
			return "", false
		}
		fields := strings.Fields(string(output))
		if len(fields) < 2 {
			return "", false
		}
		return fields[1], true
	case "apk":
		output, err := exec.CommandContext(context.Background(), "apk", "info", "-e", name).Output()
		if err != nil {
			return "", false
		}
		line := strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0])
		if line == "" {
			return "", false
		}
		return strings.TrimPrefix(line, name+"-"), true
	default:
		return "", false
	}
}

func parseAptUpdates(output string) map[string]string {
	result := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, "[upgradable from:") || !strings.Contains(line, "/") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.SplitN(fields[0], "/", 2)[0]
		if name != "Listing..." {
			result[name] = fields[1]
		}
	}
	return result
}

func parseDnfUpdates(output string) map[string]string {
	result := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || strings.HasPrefix(fields[0], "Last") || strings.HasPrefix(fields[0], "Obsoleting") {
			continue
		}
		name := fields[0]
		if idx := strings.LastIndex(name, "."); idx > 0 && isRPMArch(name[idx+1:]) {
			name = name[:idx]
		}
		result[name] = fields[1]
	}
	return result
}

func parseZypperUpdates(output string) map[string]string {
	result := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) < 5 {
			continue
		}
		name := strings.TrimSpace(parts[2])
		version := strings.TrimSpace(parts[4])
		if name != "" && version != "" && name != "Name" {
			result[name] = version
		}
	}
	return result
}

func parsePacmanUpdates(output string) map[string]string {
	result := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		result[fields[0]] = fields[3]
	}
	return result
}

func parsePacmanQueryUpdates(output string) map[string]string {
	result := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 && fields[2] == "->" {
			result[fields[0]] = fields[3]
			continue
		}
		if len(fields) >= 3 {
			result[fields[0]] = fields[2]
		}
	}
	return result
}

func parseApkUpdates(output string) map[string]string {
	result := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		match := packageVersionPattern.FindStringSubmatch(strings.TrimSpace(scanner.Text()))
		if len(match) != 4 {
			continue
		}
		result[match[1]] = match[3]
	}
	return result
}

func isRPMArch(value string) bool {
	switch value {
	case "x86_64", "aarch64", "noarch", "ppc64le", "s390x", "arm64", "armv7hl", "i686":
		return true
	default:
		return false
	}
}

func isKernelPackage(name string) bool {
	lower := strings.ToLower(name)
	for _, exact := range []string{"linux", "linux-hardened", "linux-zen", "linux-rt"} {
		if lower == exact {
			return true
		}
	}
	for _, prefix := range []string{
		"linux-image", "linux-generic", "linux-virtual", "linux-azure",
		"linux-oem", "linux-lowlatency", "linux-kvm", "linux-lts", "kernel",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func runtimeKernelVersion() string {
	output, err := exec.CommandContext(context.Background(), "uname", "-r").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

var startAsyncCommand = func(_ context.Context, command string, args ...string) error {
	return exec.Command(command, args...).Start()
}

// Reboot schedules a system reboot without waiting for the shutdown to finish.
// This is intentionally asynchronous because the HTTP request will be terminated
// by the reboot itself. The spawned process must not inherit the HTTP request
// context: that context is canceled as soon as the response is returned.
func Reboot(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("system reboot requires root privileges")
	}

	commands := [][]string{
		{"systemctl", "reboot"},
		{"reboot"},
		{"shutdown", "-r", "now"},
	}
	for _, args := range commands {
		if !commandExists(args[0]) {
			continue
		}
		if err := startAsyncCommand(ctx, args[0], args[1:]...); err == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to start system reboot command")
}

func rebootRequired(manager string) bool {
	for _, path := range []string{"/var/run/reboot-required", "/run/reboot-required"} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	if manager == "dnf" || manager == "yum" {
		if commandExists("needs-restarting") {
			return exec.CommandContext(context.Background(), "needs-restarting", "-r").Run() != nil
		}
	}
	return false
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runCommand(ctx context.Context, command string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func truncateOutput(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n…"
}
