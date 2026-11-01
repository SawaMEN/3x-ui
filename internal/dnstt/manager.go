// Package dnstt manages a standalone DNS transport tunnel. It deliberately
// does not alter host resolver settings, OxiDNS, or the sing-box DNS overlay.
package dnstt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	Root        = "/etc/x-ui/dnstt"
	binaryPath  = Root + "/bin/dnstt-server"
	privatePath = Root + "/server.key"
	publicPath  = Root + "/server.pub"
	statePath   = Root + "/settings.json"
	versionPath = Root + "/version"
	unitPath    = "/etc/systemd/system/x-ui-dnstt.service"
	unitName    = "x-ui-dnstt.service"
	Version     = "2026-05-01-0c5c52a"
	releaseBase = "https://github.com/NNdroid/dnstt/releases/download/" + Version + "/dnstt-server-linux-"
)

var mu sync.Mutex

type Config struct {
	Domain     string `json:"domain"`
	ServerIP   string `json:"serverIP"`
	ListenPort int    `json:"listenPort"`
	TargetPort int    `json:"targetPort"`
	Enabled    bool   `json:"enabled"`
}
type Status struct {
	Config
	Installed         bool   `json:"installed"`
	Running           bool   `json:"running"`
	Supported         bool   `json:"supported"`
	UnsupportedReason string `json:"unsupportedReason,omitempty"`
	InstalledVersion  string `json:"installedVersion,omitempty"`
	LatestVersion     string `json:"latestVersion"`
	UpdateAvailable   bool   `json:"updateAvailable"`
	PublicKey         string `json:"publicKey,omitempty"`
	NameServer        string `json:"nameServer,omitempty"`
}

// binaryReady rejects empty downloads and files that systemd cannot execute.
func binaryReady(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0 && info.Mode().Perm()&0o111 != 0
}

func Defaults() Config { return Config{ListenPort: 53, TargetPort: 22} }
func normalize(c *Config) {
	if c.ListenPort == 0 {
		c.ListenPort = 53
	}
	if c.TargetPort == 0 {
		c.TargetPort = 22
	}
	c.Domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(c.Domain)), ".")
	c.ServerIP = strings.TrimSpace(c.ServerIP)
}

var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func Validate(c Config) error {
	normalize(&c)
	labels := strings.Split(c.Domain, ".")
	if len(c.Domain) > 253 || len(labels) < 2 {
		return errors.New("укажите домен выделенной зоны DNSTT, например t.example.com")
	}
	for _, label := range labels {
		if !dnsLabel.MatchString(label) {
			return fmt.Errorf("некорректная DNS-метка %q", label)
		}
	}
	if c.ListenPort < 1 || c.ListenPort > 65535 {
		return errors.New("неверный входящий UDP-порт")
	}
	if c.TargetPort < 1 || c.TargetPort > 65535 {
		return errors.New("неверный локальный TCP-порт")
	}
	// The suggested authoritative NS hostname must be outside the
	// delegated tunnel zone or recursive resolution can loop.
	if strings.EqualFold(labels[0], "ns") {
		return errors.New("имя ns.<родительский домен> зарезервировано для NS-сервера; используйте t.example.com")
	}
	if c.ServerIP != "" {
		ip := net.ParseIP(c.ServerIP)
		if ip == nil || ip.To4() == nil {
			return errors.New("укажите публичный IPv4 сервера")
		}
	}
	return nil
}

func supported() (bool, string) {
	if runtime.GOOS != "linux" {
		return false, "DNSTT требует Linux"
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return false, "нет проверенного бинарника DNSTT для этой архитектуры"
	}
	if os.Geteuid() != 0 {
		return false, "управление systemd требует права root"
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false, "не найден systemctl"
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return false, "systemd не запущен"
	}
	return true, ""
}

func load() (Config, error) {
	c := Defaults()
	data, err := os.ReadFile(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	normalize(&c)
	return c, nil
}

func StatusNow(ctx context.Context) (Status, error) {
	c, err := load()
	if err != nil {
		return Status{}, err
	}
	ok, reason := supported()
	s := Status{Config: c, Supported: ok, UnsupportedReason: reason, LatestVersion: Version}
	if binaryReady(binaryPath) {
		s.Installed = true
	}
	if !s.Installed {
		s.Enabled = false
		return s, nil
	}
	if version, err := os.ReadFile(versionPath); err == nil {
		s.InstalledVersion = strings.TrimSpace(string(version))
	}
	s.UpdateAvailable = s.InstalledVersion != Version
	if key, err := os.ReadFile(publicPath); err == nil {
		k := strings.TrimSpace(string(key))
		if len(k) == 64 {
			if _, e := hex.DecodeString(k); e == nil {
				s.PublicKey = k
			}
		}
	}
	if c.Domain != "" {
		s.NameServer = "ns." + strings.Join(strings.Split(c.Domain, ".")[1:], ".")
	}
	if ok {
		s.Running = exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", unitName).Run() == nil
	}
	return s, nil
}

func fileWrite(path string, b []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".dnstt-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func systemctl(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func asset() (string, string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return releaseBase + "amd64", "def817a91ea0fce57c37c3d82c97799d7caa38618b8a73e94c05c14a1300560c", nil
	case "linux/arm64":
		return releaseBase + "arm64", "da748d99f4b25743878dac21806c1c97a4ddad63ee2c4bbb1eb7361497b35ac0", nil
	default:
		return "", "", errors.New("DNSTT не поддерживается на этой архитектуре")
	}
}

func download(ctx context.Context) ([]byte, error) {
	url, digest, err := asset()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 95 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("скачивание DNSTT: HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (32<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 32<<20 {
		return nil, errors.New("слишком большой бинарник DNSTT")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return nil, errors.New("контрольная сумма DNSTT не совпадает с проверенным релизом")
	}
	return data, nil
}

func Install(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()
	if ok, reason := supported(); !ok {
		return errors.New(reason)
	}
	if binaryReady(binaryPath) {
		return nil
	}
	data, err := download(ctx)
	if err != nil {
		return err
	}
	if err = fileWrite(binaryPath, data, 0o755); err != nil {
		return err
	}
	return fileWrite(versionPath, []byte(Version+"\n"), 0o600)
}

func serviceUnit(c Config) string {
	return "[Unit]\nDescription=3x-ui DNSTT DNS transport tunnel\nAfter=network-online.target\nWants=network-online.target\n[Service]\nType=simple\nExecStart=" + binaryPath + " -udp :" + strconv.Itoa(c.ListenPort) + " -privkey-file " + privatePath + " " + c.Domain + " 127.0.0.1:" + strconv.Itoa(c.TargetPort) + "\nRestart=on-failure\nRestartSec=3\nNoNewPrivileges=true\nProtectSystem=full\nProtectHome=true\nCapabilityBoundingSet=CAP_NET_BIND_SERVICE\nAmbientCapabilities=CAP_NET_BIND_SERVICE\n[Install]\nWantedBy=multi-user.target\n"
}

func keypair(ctx context.Context) error {
	private, err := os.Stat(privatePath)
	pub, pubErr := os.Stat(publicPath)
	if err == nil && pubErr == nil && private.Mode().IsRegular() && pub.Mode().IsRegular() {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) || !errors.Is(pubErr, os.ErrNotExist) {
		return errors.New("набор ключей DNSTT неполный: восстановите оба файла ключей или переустановите DNSTT")
	}
	if err := os.MkdirAll(Root, 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, binaryPath, "-gen-key", "-privkey-file", privatePath, "-pubkey-file", publicPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("генерация ключей DNSTT: %w (%s)", err, out)
	}
	if err := os.Chmod(privatePath, 0o600); err != nil {
		return err
	}
	return os.Chmod(publicPath, 0o644)
}

func serviceActive(ctx context.Context) bool {
	return exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", unitName).Run() == nil
}

func Setup(ctx context.Context, c Config) error {
	mu.Lock()
	defer mu.Unlock()
	normalize(&c)
	if err := Validate(c); err != nil {
		return err
	}
	if !binaryReady(binaryPath) {
		return errors.New("сначала установите DNSTT в разделе «Обновления пакетов и ядер Linux»")
	}
	if ok, reason := supported(); !ok {
		return errors.New(reason)
	}
	if err := keypair(ctx); err != nil {
		return err
	}
	old, err := load()
	if err != nil {
		return err
	}
	oldUnit, unitErr := os.ReadFile(unitPath)
	if unitErr != nil && !errors.Is(unitErr, os.ErrNotExist) {
		return unitErr
	}
	oldState, stateErr := os.ReadFile(statePath)
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return stateErr
	}
	// Validate local TCP service before exposing a tunnel that cannot connect.
	dialer := net.Dialer{Timeout: 900 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.TargetPort)))
	if err != nil {
		return fmt.Errorf("нет локального TCP-сервиса 127.0.0.1:%d: %w", c.TargetPort, err)
	}
	conn.Close()
	if err := fileWrite(unitPath, []byte(serviceUnit(c)), 0o644); err != nil {
		return err
	}
	rollback := func(original error) error {
		rec, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var problems error
		if len(oldUnit) > 0 {
			problems = errors.Join(problems, fileWrite(unitPath, oldUnit, 0o644))
		} else {
			if e := os.Remove(unitPath); e != nil && !errors.Is(e, os.ErrNotExist) {
				problems = errors.Join(problems, e)
			}
		}
		if len(oldState) > 0 {
			problems = errors.Join(problems, fileWrite(statePath, oldState, 0o600))
		} else {
			if e := os.Remove(statePath); e != nil && !errors.Is(e, os.ErrNotExist) {
				problems = errors.Join(problems, e)
			}
		}
		if len(oldUnit) > 0 {
			// Stop the failed candidate before replacing its unit.
			if serviceActive(rec) {
				problems = errors.Join(problems, systemctl(rec, "stop", unitName))
			}
			problems = errors.Join(problems, systemctl(rec, "daemon-reload"))
			if old.Enabled {
				problems = errors.Join(problems, systemctl(rec, "enable", unitName), systemctl(rec, "restart", unitName))
			} else {
				problems = errors.Join(problems, systemctl(rec, "disable", unitName))
			}
		} else {
			if serviceActive(rec) {
				problems = errors.Join(problems, systemctl(rec, "stop", unitName))
			}
			problems = errors.Join(problems, systemctl(rec, "disable", unitName), systemctl(rec, "daemon-reload"))
		}
		return errors.Join(original, problems)
	}
	if err := systemctl(ctx, "daemon-reload"); err != nil {
		return rollback(err)
	}
	if err := systemctl(ctx, "enable", unitName); err != nil {
		return rollback(err)
	}
	if err := systemctl(ctx, "restart", unitName); err != nil {
		return rollback(err)
	}
	for n := 0; n < 9; n++ {
		if serviceActive(ctx) {
			break
		}
		select {
		case <-ctx.Done():
			return rollback(ctx.Err())
		case <-time.After(350 * time.Millisecond):
		}
	}
	if !serviceActive(ctx) {
		return rollback(errors.New("DNSTT не запустился; проверьте UDP-порт и журнал systemd"))
	}
	c.Enabled = true
	data, err := json.Marshal(c)
	if err != nil {
		return rollback(err)
	}
	if err := fileWrite(statePath, data, 0o600); err != nil {
		return rollback(err)
	}
	return nil
}

func Disable(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()
	old, err := load()
	if err != nil {
		return err
	}
	if _, err := os.Stat(unitPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if serviceActive(ctx) {
		if err := systemctl(ctx, "stop", unitName); err != nil {
			return err
		}
	}
	if err := systemctl(ctx, "disable", unitName); err != nil {
		return err
	}
	old.Enabled = false
	data, err := json.Marshal(old)
	if err != nil {
		return err
	}
	return fileWrite(statePath, data, 0o600)
}

func Upgrade(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()
	if _, err := os.Stat(binaryPath); err != nil {
		return errors.New("DNSTT не установлен")
	}
	current, err := os.ReadFile(versionPath)
	if err == nil && strings.TrimSpace(string(current)) == Version {
		return nil
	}
	fresh, err := download(ctx)
	if err != nil {
		return err
	}
	prev, err := os.ReadFile(binaryPath)
	if err != nil {
		return err
	}
	cfg, err := load()
	if err != nil {
		return err
	}
	if err := fileWrite(binaryPath, fresh, 0o755); err != nil {
		return err
	}
	if cfg.Enabled {
		restartErr := systemctl(ctx, "restart", unitName)
		if restartErr == nil && !serviceActive(ctx) {
			restartErr = errors.New("обновлённый DNSTT не запускается")
		}
		if restartErr != nil {
			rec, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			restoreErr := fileWrite(binaryPath, prev, 0o755)
			if restoreErr == nil {
				restoreErr = systemctl(rec, "restart", unitName)
			}
			return errors.Join(restartErr, restoreErr)
		}
	}
	return fileWrite(versionPath, []byte(Version+"\n"), 0o600)
}

func Remove(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()
	if ok, reason := supported(); !ok {
		return errors.New(reason)
	}
	if _, err := os.Stat(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// systemd may still hold a unit in memory after an interrupted removal.
	if serviceActive(ctx) {
		if err := systemctl(ctx, "stop", unitName); err != nil {
			return err
		}
	}
	if _, err := os.Stat(unitPath); err == nil ||
		exec.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", unitName).Run() == nil {
		if err := systemctl(ctx, "disable", unitName); err != nil {
			return err
		}
	}
	if err := os.Remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	return os.RemoveAll(Root)
}
