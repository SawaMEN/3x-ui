// Package olcrtc manages the independent olcRTC server and its upstream manager.
// It never modifies the panel's sing-box core or inbound configuration.
package olcrtc

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	configPath     = "/etc/olcrtc-manager/config.json"
	envPath        = "/etc/olcrtc-manager/panel.env"
	revisionPath   = "/etc/olcrtc-manager/.3x-ui-revisions"
	servicePath    = "/etc/systemd/system/olcrtc-manager.service"
	installLogPath = "/var/log/3x-ui/olcrtc/install.log"
	// The installer is pinned to the reviewed upstream revision. Source branches
	// used by upstream's installer are deliberately updated by Update().
	installerURL = "https://raw.githubusercontent.com/BigDaddy3334/olcrtc-manager-panel/df626030d21d00286304bc8ef1e990e28c247a55/scripts/install.sh"
	managerRepo  = "https://github.com/BigDaddy3334/olcrtc-manager-panel.git"
	serverRepo   = "https://github.com/openlibrecommunity/olcrtc.git"
)

type Status struct {
	Installed         bool   `json:"installed"`
	Running           bool   `json:"running"`
	InstalledVersion  string `json:"installedVersion"`
	AvailableVersion  string `json:"availableVersion"`
	UpdateAvailable   bool   `json:"updateAvailable"`
	Supported         bool   `json:"supported"`
	UnsupportedReason string `json:"unsupportedReason,omitempty"`
	Busy              bool   `json:"busy"`
	Operation         string `json:"operation,omitempty"`
	Error             string `json:"error,omitempty"`
}
type SocksProxy struct {
	Addr string `json:"addr,omitempty"`
	Port int    `json:"port,omitempty"`
	User string `json:"user,omitempty"`
	Pass string `json:"pass,omitempty"`
}
type Location struct {
	Name     string `json:"name"`
	Endpoint struct {
		RoomID string `json:"room_id"`
		Key    string `json:"key"`
	} `json:"endpoint"`
	Carrier   string `json:"carrier"`
	Transport struct {
		Type    string            `json:"type"`
		Payload map[string]string `json:"payload,omitempty"`
	} `json:"transport"`
	DNS   string     `json:"dns"`
	Link  string     `json:"link,omitempty"`
	Data  string     `json:"data,omitempty"`
	Proxy SocksProxy `json:"proxy,omitempty"`
}

func SimpleLocation(loc Location) bool {
	return len(loc.Transport.Payload) == 0 &&
		loc.Proxy == (SocksProxy{}) &&
		(loc.Link == "" || loc.Link == "direct") &&
		(loc.Data == "" || loc.Data == "data")
}

type Quota struct {
	SpeedMbps int    `json:"speed_mbps,omitempty"`
	TrafficGB int    `json:"traffic_gb,omitempty"`
	UsedGB    int    `json:"used_gb,omitempty"`
	UsedBytes uint64 `json:"used_bytes,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}
type Client struct {
	ID        string     `json:"client-id"`
	Refresh   string     `json:"refresh,omitempty"`
	Quota     Quota      `json:"quota,omitempty"`
	Locations []Location `json:"locations"`
}
type clientConfig struct {
	Clients []Client `json:"clients"`
	Port    int      `json:"port"`
}
type ClientInput struct {
	ClientID  string `json:"client_id"`
	RoomID    string `json:"room_id"`
	Key       string `json:"key"`
	Carrier   string `json:"carrier"`
	Transport string `json:"transport"`
	DNS       string `json:"dns"`
	Name      string `json:"name"`
	Refresh   string `json:"refresh,omitempty"`
	Quota     *Quota `json:"quota,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
var state = struct {
	sync.Mutex
	busy      bool
	operation string
	lastError string
}{}
var updateCache = struct {
	sync.Mutex
	when time.Time
	sha  string
}{}

func Supported() (bool, string) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return false, "OlcRTC поддерживается здесь на Linux amd64/arm64"
	}
	if os.Geteuid() != 0 {
		return false, "Для установки нужны права root"
	}
	if installerPackageManager() == "" {
		return false, "Автоматическая установка доступна для Debian/Ubuntu (apt) и Arch Linux (pacman)"
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false, "Для установки необходим systemd"
	}
	return true, ""
}

// Prefer the distribution's own package manager. Do not run pacman -Sy
// from the panel: a partial Arch upgrade can leave the host inconsistent.
func installerPackageManager() string {
	data, _ := os.ReadFile("/etc/os-release")
	id := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ID=") {
			id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"'")
			break
		}
	}
	if id == "arch" {
		if _, err := exec.LookPath("pacman"); err == nil {
			return "pacman"
		}
		return ""
	}
	if _, err := exec.LookPath("apt-get"); err == nil {
		return "apt"
	}
	return ""
}

// The upstream installer is pinned but currently accepts apt only. Add the
// Arch-specific dependency installation before the apt branch without forking
// its build/service setup. Fail closed if upstream changes this anchor.
func addArchPackagesToInstaller(original []byte) ([]byte, error) {
	const marker = "install_packages() {\n"
	const aptOnly = "unsupported OS: this installer currently supports apt-based Linux distributions"
	if bytes.Count(original, []byte(marker)) != 1 || !bytes.Contains(original, []byte(aptOnly)) {
		return nil, errors.New("upstream OlcRTC installer changed; refusing unsafe Arch patch")
	}
	const arch = "	if command -v pacman >/dev/null 2>&1; then\n" +
		"		pacman -S --noconfirm --needed ca-certificates curl git tar xz iproute2 iptables openssl\n" +
		"		return\n" +
		"	fi\n"
	return bytes.Replace(original, []byte(marker), []byte(marker+arch), 1), nil
}

func installed() bool {
	a, e := os.Stat("/usr/local/bin/olcrtc")
	if e != nil || a.IsDir() {
		return false
	}
	b, e := os.Stat("/usr/local/bin/olcrtc-manager")
	return e == nil && !b.IsDir()
}
func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
func readRevisions() string {
	data, _ := os.ReadFile(revisionPath)
	return strings.TrimSpace(string(data))
}

var revisionSHA = regexp.MustCompile("^[a-fA-F0-9]{40}$")

// The GitHub HTTPS API works before git is installed on a minimal server.
func fetchGitHubRevision(ctx context.Context, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "3x-ui-olcrtc")
	response, err := (&http.Client{Timeout: 4 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub revisions: HTTP %d", response.StatusCode)
	}
	var data struct{ SHA string }
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&data); err != nil {
		return "", err
	}
	if !revisionSHA.MatchString(data.SHA) {
		return "", errors.New("invalid upstream revision")
	}
	return strings.ToLower(data.SHA), nil
}

func resolveRemoteRevision(ctx context.Context, endpoint, repo, ref string) string {
	lookup, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	if sha, err := fetchGitHubRevision(lookup, endpoint); err == nil {
		return sha
	}
	// Fallback to git if GitHub's API is blocked or rate-limited.
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}
	output, err := run(lookup, "git", "ls-remote", repo, ref)
	if err != nil {
		return ""
	}
	fields := strings.Fields(output)
	if len(fields) > 0 && revisionSHA.MatchString(fields[0]) {
		return strings.ToLower(fields[0])
	}
	return ""
}

func remoteRevision(ctx context.Context) string {
	updateCache.Lock()
	defer updateCache.Unlock()
	if time.Since(updateCache.when) < 5*time.Minute {
		return updateCache.sha
	}
	sources := [...]struct{ endpoint, repo, ref string }{
		{"https://api.github.com/repos/BigDaddy3334/olcrtc-manager-panel/commits/main", managerRepo, "refs/heads/main"},
		{"https://api.github.com/repos/openlibrecommunity/olcrtc/commits/master", serverRepo, "refs/heads/master"},
	}
	var revisions [2]string
	var lookups sync.WaitGroup
	for i := range sources {
		lookups.Add(1)
		go func(index int) {
			defer lookups.Done()
			item := sources[index]
			revisions[index] = resolveRemoteRevision(ctx, item.endpoint, item.repo, item.ref)
		}(i)
	}
	lookups.Wait()
	if revisions[0] != "" && revisions[1] != "" {
		updateCache.sha = strings.Join(revisions[:], "|")
	} else {
		// A failed lookup must not advertise a stale upstream revision.
		updateCache.sha = ""
	}
	updateCache.when = time.Now()
	return updateCache.sha
}
func revisionLabel(value string) string {
	parts := strings.Split(value, "|")
	if len(parts) != 2 {
		return value
	}
	shorten := func(sha string) string {
		if len(sha) > 8 {
			return sha[:8]
		}
		return sha
	}
	return "manager " + shorten(parts[0]) + " / olcrtc " + shorten(parts[1])
}
func GetStatus(ctx context.Context) Status {
	yes, reason := Supported()
	result := Status{Installed: installed(), Supported: yes, UnsupportedReason: reason}
	// Query upstream via HTTPS even when git has not been installed.
	latest := remoteRevision(ctx)
	result.AvailableVersion = revisionLabel(latest)
	if result.Installed {
		check, cancel := context.WithTimeout(ctx, 3*time.Second)
		output, err := run(check, "systemctl", "is-active", "olcrtc-manager")
		cancel()
		result.Running = err == nil && output == "active"
		current := readRevisions()
		result.InstalledVersion = revisionLabel(current)
		result.UpdateAvailable = latest != "" && current != latest
	}
	state.Lock()
	result.Busy = state.busy
	result.Operation = state.operation
	result.Error = state.lastError
	state.Unlock()
	return result
}

// Start launches a single long-running installation operation; the HTTP request
// returns immediately and the UI polls status. No user-controlled commands.
func Start(operation string) error {
	if operation != "install" && operation != "update" && operation != "remove" && operation != "restart" {
		return errors.New("unknown OlcRTC operation")
	}
	if operation != "remove" {
		if yes, reason := Supported(); !yes {
			return errors.New(reason)
		}
	}
	state.Lock()
	if state.busy {
		state.Unlock()
		return errors.New("OlcRTC operation is already running")
	}
	state.busy = true
	state.operation = operation
	state.lastError = ""
	state.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
		defer cancel()
		var err error
		switch operation {
		case "remove":
			err = remove(ctx)
		case "restart":
			if !installed() {
				err = errors.New("OlcRTC is not installed")
			} else {
				_, err = run(ctx, "systemctl", "restart", "olcrtc-manager")
				if err == nil {
					err = waitForManager(ctx)
				}
			}
		default:
			err = install(ctx)
		}
		state.Lock()
		state.busy = false
		if err != nil {
			state.lastError = err.Error()
		} else {
			state.lastError = ""
		}
		state.Unlock()
	}()
	return nil
}

// Upstream logs may include freshly generated admin credentials. Never expose
// raw installer logs through the API: only allow-list executable names.
var installerStage = regexp.MustCompile(`(?m)^3XUI_OLCRTC_STAGE=(packages|toolchain|memory|sources|build-server|build-manager|deploy|config|service|complete)$`)
var installerMissingCommand = regexp.MustCompile(`(?m)line [0-9]+: ([A-Za-z0-9_.+-]{1,64}): command not found$`)

func installerFailure(err error, logPath string) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 127 {
		if output, readErr := os.ReadFile(logPath); readErr == nil {
			matches := installerMissingCommand.FindAllSubmatch(output, -1)
			for i := len(matches) - 1; i >= 0; i-- {
				command := string(matches[i][1])
				switch command {
				case "go", "git", "curl", "tar", "apt-get", "pacman", "systemctl",
					"bash", "hostname", "awk", "sed", "sort", "head", "grep", "install",
					"ln", "rm", "mkdir", "mktemp", "ss", "seq", "od", "tr", "id", "tail", "cut", "openssl",
					"chmod", "fallocate", "mkswap", "swapon", "dd", "cat", "ip", "iptables", "tc":
					return fmt.Errorf("OlcRTC installer: command %q not found (exit status 127); check PATH and packages", command)
				}
			}
		}
		return errors.New("OlcRTC installer: required command not found (exit status 127); check PATH and packages")
	}
	if output, readErr := os.ReadFile(logPath); readErr == nil {
		if matches := installerStage.FindAllSubmatch(output, -1); len(matches) > 0 {
			return fmt.Errorf("OlcRTC installation failed at stage %s (%w)", matches[len(matches)-1][1], err)
		}
	}
	return fmt.Errorf("OlcRTC installation failed before build initialization (%w)", err)
}

// These commands are used by the pinned install.sh global initializers
// before install_packages runs. Minimal server images may be missing them.
var initialInstallerCommands = []string{"bash", "id", "od", "tr", "seq", "ss", "grep", "awk"}

func missingInstallerCommands() []string {
	var missing []string
	for _, name := range initialInstallerCommands {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	return missing
}

func installerPrerequisitePackages(pm string) []string {
	switch pm {
	case "apt":
		return []string{"bash", "coreutils", "gawk", "grep", "iproute2"}
	case "pacman":
		return []string{"bash", "coreutils", "gawk", "grep", "iproute2"}
	default:
		return nil
	}
}

func prepareInstallerHost(ctx context.Context) error {
	missing := missingInstallerCommands()
	if len(missing) == 0 {
		return nil
	}
	pm := installerPackageManager()
	packages := installerPrerequisitePackages(pm)
	if len(packages) == 0 {
		return fmt.Errorf("OlcRTC installer: missing commands: %s (unsupported package manager)", strings.Join(missing, ", "))
	}
	var commands [][]string
	switch pm {
	case "apt":
		commands = append(commands, []string{"apt-get", "update"})
		commands = append(commands, append([]string{"apt-get", "install", "-y", "--no-install-recommends"}, packages...))
	case "pacman":
		// Do not run pacman -Sy: partial upgrades are unsafe on Arch.
		commands = append(commands, append([]string{"pacman", "-S", "--noconfirm", "--needed"}, packages...))
	}
	for _, args := range commands {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("OlcRTC prerequisites: %s failed (missing: %s): %w", args[0], strings.Join(missing, ", "), err)
		}
	}
	if remaining := missingInstallerCommands(); len(remaining) > 0 {
		return fmt.Errorf("OlcRTC installer: required commands still missing: %s", strings.Join(remaining, ", "))
	}
	return nil
}

func install(ctx context.Context) error {
	if installed() {
		if _, err := os.Stat(revisionPath); err != nil {
			return errors.New("an existing OlcRTC installation is not managed by 3x-ui")
		}
	}
	if err := prepareInstallerHost(ctx); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, installerURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("fetch upstream installer: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream installer: HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 256<<10))
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(data, []byte("#!/usr/bin/env bash")) {
		return errors.New("unexpected upstream installer format")
	}
	data, err = managedInstaller(data, installerPackageManager())
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "olcrtc-installer-*.sh")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	revisions, err := os.CreateTemp("", "olcrtc-built-revisions-*")
	if err != nil {
		return err
	}
	builtPath := revisions.Name()
	_ = revisions.Close()
	defer os.Remove(builtPath)
	cmd := exec.CommandContext(ctx, "bash", name)
	// Never publish the independent admin interface. Native settings go through 3x-ui.
	cmd.Env = installerEnvironment(builtPath)
	// Installer logs include a generated password: never return them to browsers.
	logFile, err := os.CreateTemp("", "olcrtc-install-log-*.log")
	if err != nil {
		return err
	}
	defer os.Remove(logFile.Name())
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Run(); err != nil {
		failure := installerFailure(err, logFile.Name())
		// Keep a root-only diagnostic after the temporary file is removed.
		// It can contain credentials, so never return its contents via API.
		if output, readErr := os.ReadFile(logFile.Name()); readErr == nil {
			if os.MkdirAll("/var/log/3x-ui/olcrtc", 0700) == nil {
				if os.WriteFile(installLogPath, output, 0600) == nil {
					return fmt.Errorf("%w; root-only log: %s", failure, installLogPath)
				}
			}
		}
		return failure
	}
	if _, err := run(ctx, "systemctl", "restart", "olcrtc-manager"); err != nil {
		return fmt.Errorf("installed but unable to restart olcrtc-manager: %w", err)
	}
	// Record the commits actually checked out, never remote branch tips which
	// may have advanced during a long build.
	built, err := os.ReadFile(builtPath)
	if err != nil {
		return err
	}
	if !validRevisions(strings.TrimSpace(string(built))) {
		return errors.New("installer did not report valid built revisions")
	}
	if err := os.WriteFile(revisionPath, built, 0600); err != nil {
		return err
	}
	return waitForManager(ctx)
}
func remove(ctx context.Context) error {
	// Refuse to delete unrelated services or components.
	if _, err := os.Stat(revisionPath); err != nil {
		return errors.New("this OlcRTC installation is not managed by 3x-ui")
	}
	data, err := os.ReadFile(servicePath)
	if err != nil {
		return fmt.Errorf("cannot verify managed service: %w", err)
	}
	if !bytes.Contains(data, []byte("ExecStart=/usr/local/bin/olcrtc-manager")) {
		return errors.New("OlcRTC service was not created by the supported installer")
	}
	if _, err = run(ctx, "systemctl", "disable", "--now", "olcrtc-manager"); err != nil {
		return fmt.Errorf("stopping olcrtc-manager: %w", err)
	}
	for _, name := range []string{servicePath, "/usr/local/bin/olcrtc-manager", "/usr/local/bin/olcrtc", revisionPath} {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	_, err = run(ctx, "systemctl", "daemon-reload")
	// Preserve client config and secrets for a future reinstall.
	return err
}
func readConfig() (clientConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return clientConfig{}, err
	}
	if len(data) > 2<<20 {
		return clientConfig{}, errors.New("OlcRTC config is too large")
	}
	var cfg clientConfig
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return cfg, err
	}
	if cfg.Clients == nil {
		cfg.Clients = []Client{}
	}
	return cfg, nil
}
func Clients() ([]Client, error) {
	if !installed() {
		return nil, errors.New("OlcRTC is not installed")
	}
	cfg, err := readConfig()
	if err != nil {
		return nil, err
	}
	return cfg.Clients, nil
}
func randomHex(n int) (string, error) {
	value := make([]byte, n)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
func normalize(input *ClientInput) error {
	input.ClientID = strings.TrimSpace(input.ClientID)
	input.RoomID = strings.TrimSpace(input.RoomID)
	input.Carrier = strings.TrimSpace(input.Carrier)
	input.Transport = strings.TrimSpace(input.Transport)
	input.DNS = strings.TrimSpace(input.DNS)
	if !idPattern.MatchString(input.ClientID) {
		return errors.New("client ID must contain 1-64 letters, numbers, dots, underscores or hyphens")
	}
	if input.Carrier == "" {
		input.Carrier = "jitsi"
	}
	allowed := map[string][]string{"jitsi": {"datachannel", "vp8channel", "seichannel", "videochannel"}, "telemost": {"vp8channel", "videochannel"}, "wbstream": {"vp8channel", "seichannel", "videochannel"}}
	variants, ok := allowed[input.Carrier]
	if !ok {
		return errors.New("unsupported provider")
	}
	if input.Transport == "" {
		if input.Carrier == "jitsi" {
			input.Transport = "datachannel"
		} else {
			input.Transport = "vp8channel"
		}
	}
	valid := false
	for _, item := range variants {
		if item == input.Transport {
			valid = true
		}
	}
	if !valid {
		return errors.New("unsupported provider and transport combination")
	}
	if input.RoomID == "" && input.Carrier == "jitsi" {
		suffix, err := randomHex(12)
		if err != nil {
			return err
		}
		input.RoomID = "https://meet.handyweb.org/olcrtc-" + suffix
	}
	if input.RoomID == "" || input.RoomID == "any" || len(input.RoomID) > 512 {
		return errors.New("enter an existing room ID or URL for the selected provider")
	}
	if input.Carrier == "jitsi" {
		parsed, err := http.NewRequest(http.MethodGet, input.RoomID, nil)
		if err != nil || parsed.URL.Scheme != "https" || parsed.URL.Host == "" || parsed.URL.User != nil {
			return errors.New("Jitsi room must be an HTTPS URL")
		}
	}
	if input.Key == "" {
		var err error
		input.Key, err = randomHex(32)
		if err != nil {
			return err
		}
	}
	if len(input.Key) != 64 {
		return errors.New("key must be 64 hex characters")
	}
	if _, err := hex.DecodeString(input.Key); err != nil {
		return errors.New("invalid key")
	}
	if input.DNS == "" {
		input.DNS = "1.1.1.1:53"
	}
	host, port, err := net.SplitHostPort(input.DNS)
	if err != nil || net.ParseIP(host) == nil {
		return errors.New("DNS must be an IP:port")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return errors.New("invalid DNS port")
	}
	if len(input.Name) > 128 {
		return errors.New("location name is too long")
	}
	if input.Name == "" {
		input.Name = "Main"
	}
	if input.Quota != nil {
		if input.Quota.SpeedMbps < 0 || input.Quota.TrafficGB < 0 {
			return errors.New("speed and traffic quotas cannot be negative")
		}
		if input.Quota.ExpiresAt != "" {
			if _, err := time.Parse("2006-01-02", input.Quota.ExpiresAt); err != nil {
				return errors.New("expiration date must be YYYY-MM-DD")
			}
		}
	}
	return nil
}
func credentials() (string, string, error) {
	data, err := os.ReadFile(envPath)
	if err != nil {
		return "", "", err
	}
	result := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		if key == "OLCRTC_MANAGER_USER" || key == "OLCRTC_MANAGER_PASS" {
			result[key] = strings.Trim(strings.TrimSpace(value), "'\"")
		}
	}
	if result["OLCRTC_MANAGER_USER"] == "" || result["OLCRTC_MANAGER_PASS"] == "" {
		return "", "", errors.New("OlcRTC manager credentials not initialized")
	}
	return result["OLCRTC_MANAGER_USER"], result["OLCRTC_MANAGER_PASS"], nil
}
func apiResponse(ctx context.Context, method, path string, payload any) ([]byte, error) {
	if !installed() {
		return nil, errors.New("OlcRTC is not installed")
	}
	output, activeErr := run(ctx, "systemctl", "is-active", "olcrtc-manager")
	if activeErr != nil || output != "active" {
		return nil, errors.New("OlcRTC manager service is not running")
	}
	cfg, err := readConfig()
	if err != nil {
		return nil, err
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, errors.New("invalid OlcRTC port")
	}
	username, password, err := credentials()
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	// The target is fixed to loopback; no user URL reaches this HTTP client.
	url := "http://127.0.0.1:" + strconv.Itoa(cfg.Port) + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(username, password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := managerHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("OlcRTC manager is unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return io.ReadAll(io.LimitReader(response.Body, 1<<20))
	}
	msg, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	return nil, fmt.Errorf("OlcRTC manager: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(msg)))
}
func api(ctx context.Context, method, path string, payload any) error {
	_, err := apiResponse(ctx, method, path, payload)
	return err
}

// LiveStatus only exposes aggregate metrics and connection health. Upstream
// /api/state includes sensitive room keys and proxy credentials, which must
// never be forwarded to the browser as raw JSON.
type LiveStatus struct {
	RunningCount int          `json:"runningCount"`
	PeerCount    int          `json:"peerCount"`
	Clients      []LiveClient `json:"clients"`
}
type LiveClient struct {
	ID            string `json:"id"`
	Running       bool   `json:"running"`
	PeerCount     int    `json:"peerCount"`
	LocationCount int    `json:"locationCount"`
	FailedCount   int    `json:"failedCount"`
}

func Live(ctx context.Context) (LiveStatus, error) {
	body, err := apiResponse(ctx, http.MethodGet, "/api/state", nil)
	if err != nil {
		return LiveStatus{}, err
	}
	return decodeLive(body)
}
func decodeLive(body []byte) (LiveStatus, error) {
	var result LiveStatus
	var upstream struct {
		Clients []struct {
			ClientID  string `json:"client_id"`
			Locations []struct {
				Running bool `json:"running"`
				Runtime struct {
					PeerCount *int   `json:"peer_count"`
					Status    string `json:"status"`
				} `json:"runtime"`
			} `json:"locations"`
		} `json:"clients"`
	}
	if err := json.Unmarshal(body, &upstream); err != nil {
		return result, fmt.Errorf("decode OlcRTC status: %w", err)
	}
	result.Clients = make([]LiveClient, 0, len(upstream.Clients))
	for _, client := range upstream.Clients {
		row := LiveClient{ID: client.ClientID, LocationCount: len(client.Locations)}
		for _, location := range client.Locations {
			if location.Running {
				row.Running = true
				result.RunningCount++
			} else {
				row.FailedCount++
			}
			if location.Runtime.PeerCount != nil && *location.Runtime.PeerCount > 0 {
				row.PeerCount += *location.Runtime.PeerCount
				result.PeerCount += *location.Runtime.PeerCount
			}
		}
		result.Clients = append(result.Clients, row)
	}
	return result, nil
}

func CreateClient(ctx context.Context, input ClientInput) (ClientInput, error) {
	if input.ClientID == "" {
		suffix, err := randomHex(8)
		if err != nil {
			return input, err
		}
		input.ClientID = "user-" + suffix
	}
	if err := normalize(&input); err != nil {
		return input, err
	}
	if err := api(ctx, http.MethodPost, "/api/clients", input); err != nil {
		return ClientInput{}, err
	}
	return input, nil
}
func UpdateClient(ctx context.Context, id string, input ClientInput) (ClientInput, error) {
	if !idPattern.MatchString(id) {
		return input, errors.New("invalid client ID")
	}
	cfg, err := readConfig()
	if err != nil {
		return input, err
	}
	found := false
	for _, existing := range cfg.Clients {
		if existing.ID != id {
			continue
		}
		found = true
		if len(existing.Locations) != 1 {
			return input, errors.New("editing multiple-location clients is not supported in the simple editor")
		}
		if !SimpleLocation(existing.Locations[0]) {
			return input, errors.New("advanced proxy, transport or link settings cannot be edited in the simple editor")
		}
		// Preserve usage accounting when adjusting speed, traffic and expiry
		// limits, and retain settings which this simple editor doesn't expose.
		input.Refresh = existing.Refresh
		if input.Quota == nil {
			quota := existing.Quota
			input.Quota = &quota
		} else {
			input.Quota.UsedBytes = existing.Quota.UsedBytes
			input.Quota.UsedGB = existing.Quota.UsedGB
		}
		if input.ClientID == "" {
			input.ClientID = id
		}
		if input.ClientID != id {
			return input, errors.New("changing client IDs is not supported")
		}
		break
	}
	if !found {
		return input, errors.New("client not found")
	}
	if err := normalize(&input); err != nil {
		return input, err
	}
	if err := api(ctx, http.MethodPut, "/api/clients/"+id, input); err != nil {
		return ClientInput{}, err
	}
	return input, nil
}
func DeleteClient(ctx context.Context, id string) error {
	if !idPattern.MatchString(id) {
		return errors.New("invalid client ID")
	}
	return api(ctx, http.MethodDelete, "/api/clients/"+id, nil)
}
func RotateKey(ctx context.Context, id string) error {
	if !idPattern.MatchString(id) {
		return errors.New("invalid client ID")
	}
	return api(ctx, http.MethodPost, "/api/actions/rotate-key", map[string]string{"client_id": id})
}

// An exported connection link is a client-side convention, not a sing-box
// inbound or a protocol recognized by sing-box itself.
func ClientLink(client Client) string {
	if len(client.Locations) == 0 {
		return ""
	}
	loc := client.Locations[0]
	payload := ""
	if len(loc.Transport.Payload) > 0 {
		keys := make([]string, 0, len(loc.Transport.Payload))
		for key := range loc.Transport.Payload {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, key+"="+loc.Transport.Payload[key])
		}
		payload = "<" + strings.Join(parts, "&") + ">"
	}
	return "olcrtc://" + loc.Carrier + "?" + loc.Transport.Type + payload + "@" + loc.Endpoint.RoomID + "#" + loc.Endpoint.Key + "$" + loc.Name
}

//go:embed installer_adapter.sh
var installerAdapter []byte

func managedInstaller(original []byte, pm string) ([]byte, error) {
	var err error
	if pm == "pacman" {
		original, err = addArchPackagesToInstaller(original)
		if err != nil {
			return nil, err
		}
	}
	const start = `main "$@"`
	const execStart = "ExecStart=/usr/local/bin/olcrtc-manager -config $CONFIG_PATH"
	if bytes.Count(original, []byte(start)) != 1 || bytes.Count(original, []byte(execStart)) != 1 {
		return nil, errors.New("upstream OlcRTC installer changed; refusing unsafe integration patch")
	}
	// A preserved panel.env may contain TLS or bind settings from an earlier
	// standalone installation. Enforce the local HTTP API in systemd itself.
	original = bytes.Replace(original, []byte(execStart), []byte(execStart+" -addr 127.0.0.1\nUnsetEnvironment=OLCRTC_MANAGER_TLS_CERT OLCRTC_MANAGER_TLS_KEY"), 1)
	return bytes.Replace(original, []byte(start), installerAdapter, 1), nil
}

func validRevisions(value string) bool {
	parts := strings.Split(value, "|")
	return len(parts) == 2 && revisionSHA.MatchString(parts[0]) && revisionSHA.MatchString(parts[1])
}

func managerHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       25 * time.Second,
		Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func waitForManager(ctx context.Context) error {
	check, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var last error
	for {
		if _, last = apiResponse(check, http.MethodGet, "/api/settings", nil); last == nil {
			return nil
		}
		select {
		case <-check.Done():
			return fmt.Errorf("OlcRTC manager did not become ready: %w", last)
		case <-time.After(time.Second):
		}
	}
}

func installerEnvironment(builtPath string) []string {
	return append(os.Environ(),
		"PATH=/usr/local/go/bin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"PANEL_ADDR=127.0.0.1", "PANEL_TLS=0", "PANEL_ADMIN_PATH=/admin", "AUTO_SWAP=0",
		// Prevent hostname -I (EX_USAGE=64 with inetutils) in the global
		// initializer. A local HTTP service does not need a public IP.
		"PANEL_CERT_IP=127.0.0.1", "PANEL_PUBLIC_HOST=127.0.0.1",
		"PANEL_REPO="+managerRepo, "OLCRTC_REPO="+serverRepo,
		"PANEL_REF=main", "OLCRTC_REF=master",
		"CONFIG_DIR=/etc/olcrtc-manager", "CONFIG_PATH="+configPath, "PANEL_ENV_PATH="+envPath,
		"BUILT_REVISIONS_PATH="+builtPath)
}
