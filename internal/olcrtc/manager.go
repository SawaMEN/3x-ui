// Package olcrtc manages the independent olcRTC server and its upstream manager.
// It never modifies the panel's sing-box core or inbound configuration.
package olcrtc

import (
	"bytes"
	"context"
	"crypto/rand"
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
	configPath   = "/etc/olcrtc-manager/config.json"
	envPath      = "/etc/olcrtc-manager/panel.env"
	revisionPath = "/etc/olcrtc-manager/.3x-ui-revisions"
	servicePath  = "/etc/systemd/system/olcrtc-manager.service"
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
type Location struct {
	Name     string `json:"name"`
	Endpoint struct {
		RoomID string `json:"room_id"`
		Key    string `json:"key"`
	} `json:"endpoint"`
	Carrier   string `json:"carrier"`
	Transport struct {
		Type string `json:"type"`
		Payload map[string]string `json:"payload,omitempty"`
	} `json:"transport"`
	DNS string `json:"dns"`
}
type Client struct {
	ID        string          `json:"client-id"`
	Refresh   string          `json:"refresh,omitempty"`
	Quota     json.RawMessage `json:"quota,omitempty"`
	Locations []Location      `json:"locations"`
}
type clientConfig struct {
	Clients []Client `json:"clients"`
	Port    int      `json:"port"`
}
type ClientInput struct {
	ClientID  string          `json:"client_id"`
	RoomID    string          `json:"room_id"`
	Key       string          `json:"key"`
	Carrier   string          `json:"carrier"`
	Transport string          `json:"transport"`
	DNS       string          `json:"dns"`
	Name      string          `json:"name"`
	Refresh   string          `json:"refresh,omitempty"`
	Quota     json.RawMessage `json:"quota,omitempty"`
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
	if _, err := exec.LookPath("apt-get"); err != nil {
		return false, "Автоматическая установка поддерживает Debian/Ubuntu (apt)"
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false, "Для установки необходим systemd"
	}
	return true, ""
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
func remoteRevision(ctx context.Context) string {
	updateCache.Lock()
	defer updateCache.Unlock()
	if time.Since(updateCache.when) < 5*time.Minute {
		return updateCache.sha
	}
	revisions := make([]string, 0, 2)
	for _, item := range []struct{ repo, ref string }{{managerRepo, "refs/heads/main"}, {serverRepo, "refs/heads/master"}} {
		short, cancel := context.WithTimeout(ctx, 7*time.Second)
		output, err := run(short, "git", "ls-remote", item.repo, item.ref)
		cancel()
		if err != nil {
			revisions = nil
			break
		}
		fields := strings.Fields(output)
		if len(fields) == 0 || len(fields[0]) != 40 {
			revisions = nil
			break
		}
		revisions = append(revisions, fields[0])
	}
	if len(revisions) == 2 {
		updateCache.sha = strings.Join(revisions, "|")
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
	if result.Installed {
		check, cancel := context.WithTimeout(ctx, 3*time.Second)
		output, err := run(check, "systemctl", "is-active", "olcrtc-manager")
		cancel()
		result.Running = err == nil && output == "active"
		current := readRevisions()
		latest := ""
		if _, e := exec.LookPath("git"); e == nil {
			latest = remoteRevision(ctx)
		}
		result.InstalledVersion = revisionLabel(current)
		result.AvailableVersion = revisionLabel(latest)
		result.UpdateAvailable = latest != "" && current != latest
	} else {
		result.AvailableVersion = "upstream"
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
	if operation != "install" && operation != "update" && operation != "remove" {
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
		if operation == "remove" {
			err = remove(ctx)
		} else {
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
func install(ctx context.Context) error {
	if installed() {
		if _, err := os.Stat(revisionPath); err != nil {
			return errors.New("an existing OlcRTC installation is not managed by 3x-ui")
		}
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
	before := remoteRevision(ctx)
	cmd := exec.CommandContext(ctx, "bash", name)
	// Never publish the independent admin interface. Native settings go through 3x-ui.
	cmd.Env = append(os.Environ(),
		"PANEL_ADDR=127.0.0.1", "PANEL_TLS=0",
		"PANEL_ADMIN_PATH=/admin", "AUTO_SWAP=0")
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
		return fmt.Errorf("OlcRTC installation failed; check system packages, free RAM and Go toolchain (%w)", err)
	}
	if _, err := run(ctx, "systemctl", "restart", "olcrtc-manager"); err != nil {
		return fmt.Errorf("installed but unable to restart olcrtc-manager: %w", err)
	}
	if before == "" {
		before = "unknown|unknown"
	}
	if err := os.WriteFile(revisionPath, []byte(before+"\n"), 0600); err != nil {
		return err
	}
	return nil
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
func api(ctx context.Context, method, path string, payload any) error {
	if !installed() {
		return errors.New("OlcRTC is not installed")
	}
	output, activeErr := run(ctx, "systemctl", "is-active", "olcrtc-manager")
	if activeErr != nil || output != "active" {
		return errors.New("OlcRTC manager service is not running")
	}
	cfg, err := readConfig()
	if err != nil {
		return err
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return errors.New("invalid OlcRTC port")
	}
	username, password, err := credentials()
	if err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	// The target is fixed to loopback; no user URL reaches this HTTP client.
	url := "http://127.0.0.1:" + strconv.Itoa(cfg.Port) + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(username, password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("OlcRTC manager is unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	msg, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	return fmt.Errorf("OlcRTC manager: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(msg)))
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
		// Retain quota accounting and subscription refresh settings while editing
		// the single configured location.
		input.Refresh = existing.Refresh
		input.Quota = existing.Quota
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
