package oxidns

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	Version     = "v1.6.1"
	StateDir    = "/etc/x-ui/oxidns"
	binaryPath  = StateDir + "/bin/oxidns"
	configPath  = StateDir + "/config.yaml"
	statePath   = StateDir + "/settings.json"
	servicePath = "/etc/systemd/system/x-ui-oxidns.service"
	localPort   = 15353
)

var mu sync.Mutex

type Settings struct {
	Enabled    bool   `json:"enabled"`
	Domain     string `json:"domain"`
	DoH        bool   `json:"doh"`
	HTTP3      bool   `json:"http3"`
	Port       int    `json:"port"`
	DoT        bool   `json:"dot"`
	DoTPort    int    `json:"dotPort"`
	DoQ        bool   `json:"doq"`
	DoQPort    int    `json:"doqPort"`
	Upstream   string `json:"upstream"`
	UseSingBox bool   `json:"useSingBox"`
}

type Status struct {
	Settings
	Installed         bool   `json:"installed"`
	Running           bool   `json:"running"`
	DoHURL            string `json:"dohUrl,omitempty"`
	DoTAddress        string `json:"dotAddress,omitempty"`
	DoQAddress        string `json:"doqAddress,omitempty"`
	LocalAddress      string `json:"localAddress"`
	Version           string `json:"version"`
	Error             string `json:"error,omitempty"`
	InstalledVersion  string `json:"installedVersion"`
	LatestVersion     string `json:"latestVersion"`
	UpdateAvailable   bool   `json:"updateAvailable"`
	Supported         bool   `json:"supported"`
	UnsupportedReason string `json:"unsupportedReason,omitempty"`
}

func Defaults() Settings {
	return Settings{Port: 8443, DoTPort: 853, DoQPort: 853, Upstream: "quad9", UseSingBox: true}
}

func normalizePorts(c *Settings) {
	if c.Port == 0 {
		c.Port = 8443
	}
	if c.DoTPort == 0 {
		c.DoTPort = 853
	}
	if c.DoQPort == 0 {
		c.DoQPort = 853
	}
}

func Load() (Settings, error) {
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
	normalizePorts(&c)
	// Never route sing-box to a removed OxiDNS binary, even if an older
	// installation left behind an enabled settings.json file.
	if c.Enabled {
		if _, err := os.Stat(binaryPath); errors.Is(err, os.ErrNotExist) {
			c.Enabled = false
		}
	}
	return c, nil
}

func CurrentStatus(ctx context.Context) (Status, error) {
	cfg, err := Load()
	if err != nil {
		return Status{}, err
	}
	s := Status{Settings: cfg, LocalAddress: net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort)), Version: Version}
	if info, err := os.Stat(binaryPath); err == nil && info.Mode().IsRegular() {
		s.Installed = true
	}
	if !s.Installed {
		// Hide configuration for an uninstalled component, including data
		// left by an older installation.
		s.Settings = Defaults()
	}
	s.InstalledVersion = installedVersion(ctx)
	_, err = releaseAssetName()
	s.Supported = err == nil
	if err != nil {
		s.UnsupportedReason = err.Error()
	}
	if s.Supported && os.Geteuid() != 0 {
		s.Supported = false
		s.UnsupportedReason = "для установки требуется запуск панели от root"
	}
	if s.Supported {
		if _, systemErr := exec.LookPath("systemctl"); systemErr != nil {
			s.Supported = false
			s.UnsupportedReason = "не найден systemd (Docker не поддерживается)"
		} else if _, systemErr := os.Stat("/run/systemd/system"); systemErr != nil {
			s.Supported = false
			s.UnsupportedReason = "systemd не запущен: автоматическая установка в Docker пока недоступна"
		}
	}
	if s.InstalledVersion != "" {
		s.Version = s.InstalledVersion
	}
	if s.Installed {
		cmd := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "x-ui-oxidns.service")
		s.Running = cmd.Run() == nil
	}
	// A saved DoH domain is not a usable endpoint when the service is
	// disabled, removed, or its HTTPS listener has stopped.
	if s.Installed && s.Enabled && s.Running && s.DoH && s.Domain != "" {
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port))
		if conn, dialErr := net.DialTimeout("tcp", address, 250*time.Millisecond); dialErr == nil {
			_ = conn.Close()
			port := ""
			if s.Port != 443 {
				port = ":" + strconv.Itoa(s.Port)
			}
			s.DoHURL = "https://" + s.Domain + port + "/dns-query"
		}
	}
	if s.Installed && s.Enabled && s.Running && s.Domain != "" {
		if s.DoT {
			address := net.JoinHostPort("127.0.0.1", strconv.Itoa(s.DoTPort))
			if conn, dialErr := net.DialTimeout("tcp", address, 250*time.Millisecond); dialErr == nil {
				_ = conn.Close()
				s.DoTAddress = net.JoinHostPort(s.Domain, strconv.Itoa(s.DoTPort))
			}
		}
		if s.DoQ {
			// UDP is connectionless: this is the configured endpoint of a
			// running OxiDNS service, not a claim that a client probe passed.
			s.DoQAddress = net.JoinHostPort(s.Domain, strconv.Itoa(s.DoQPort))
		}
	}
	return s, nil
}

func validate(c Settings, certFile, keyFile string) error {
	if _, _, valid := upstream(c); !valid {
		return errors.New("выберите DNS-провайдера")
	}
	normalizePorts(&c)
	validPort := func(port int) bool { return port > 0 && port <= 65535 && port != localPort }
	if c.DoH && !validPort(c.Port) {
		return errors.New("неверный HTTPS-порт DoH")
	}
	if c.DoT && !validPort(c.DoTPort) {
		return errors.New("неверный TCP-порт DoT")
	}
	if c.DoQ && !validPort(c.DoQPort) {
		return errors.New("неверный UDP-порт DoQ")
	}
	if c.HTTP3 && !c.DoH {
		return errors.New("HTTP/3 требует включённого DoH")
	}
	if c.DoH && c.DoT && c.Port == c.DoTPort {
		return errors.New("DoH и DoT не могут занимать один TCP-порт")
	}
	if c.DoH && c.HTTP3 && c.DoQ && c.Port == c.DoQPort {
		return errors.New("DoH3 и DoQ не могут занимать один UDP-порт")
	}
	if c.DoH || c.DoT || c.DoQ {
		if len(c.Domain) < 4 || len(c.Domain) > 253 || !strings.Contains(c.Domain, ".") || net.ParseIP(c.Domain) != nil {
			return errors.New("укажите доменное имя для DoH")
		}
		for _, ch := range c.Domain {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '-') {
				return errors.New("недопустимые символы в домене DoH")
			}
		}
		if certFile == "" || keyFile == "" {
			return errors.New("сначала настройте TLS-сертификат панели")
		}
		pair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return fmt.Errorf("не удалось прочитать TLS-сертификат: %w", err)
		}
		leaf, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return err
		}
		if err := leaf.VerifyHostname(c.Domain); err != nil {
			return fmt.Errorf("сертификат не подходит домену: %w", err)
		}
		if time.Now().After(leaf.NotAfter) || time.Now().Before(leaf.NotBefore) {
			return errors.New("TLS-сертификат просрочен или ещё не действует")
		}
	}
	return nil
}

// DNS upstreams use authenticated TLS and official bootstrap addresses.
func upstream(c Settings) (address, bootstrap string, valid bool) {
	switch c.Upstream {
	case "quad9":
		return "tls://dns.quad9.net:853", "9.9.9.9:53", true
	case "cloudflare":
		return "tls://one.one.one.one:853", "1.1.1.1:53", true
	case "adguard":
		return "tls://dns.adguard-dns.com:853", "94.140.14.14:53", true
	case "google":
		return "tls://dns.google:853", "8.8.8.8:53", true
	case "yandex-basic":
		return "tls://common.dot.dns.yandex.net:853", "77.88.8.8:53", true
	case "yandex-safe":
		return "tls://safe.dot.dns.yandex.net:853", "77.88.8.88:53", true
	case "yandex-family":
		return "tls://family.dot.dns.yandex.net:853", "77.88.8.7:53", true
	default:
		return "", "", false
	}
}

// JSON-quoted scalar strings are valid YAML strings and cannot inject plugin fields.
func Generate(c Settings, certFile, keyFile string) (string, error) {
	normalizePorts(&c)
	if err := validate(c, certFile, keyFile); err != nil {
		return "", err
	}
	addr, bootstrap, _ := upstream(c)
	var b strings.Builder
	lines := []string{"log:", "  level: info", "plugins:", "  - tag: dns_cache", "    type: cache", "    args:", "      size: 4096", "      short_circuit: true", "      max_positive_ttl: 600", "  - tag: forward_upstream", "    type: forward", "    args:", "      upstreams:"}
	for _, line := range lines {
		fmt.Fprintln(&b, line)
	}
	fmt.Fprintln(&b, "        - addr: "+strconv.Quote(addr))
	fmt.Fprintln(&b, "          bootstrap: "+strconv.Quote(bootstrap))
	for _, line := range []string{"  - tag: main", "    type: sequence", "    args:", `      - exec: "$dns_cache"`, `      - exec: "$forward_upstream"`, "      - exec: accept"} {
		fmt.Fprintln(&b, line)
	}
	for _, scheme := range []string{"udp", "tcp"} {
		fmt.Fprintln(&b, "  - tag: "+scheme+"_local")
		fmt.Fprintln(&b, "    type: "+scheme+"_server")
		fmt.Fprintln(&b, "    args:")
		fmt.Fprintln(&b, "      entry: main")
		fmt.Fprintln(&b, "      listen: "+strconv.Quote(net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort))))
	}
	if c.DoH {
		fmt.Fprintln(&b, "  - tag: doh_public")
		fmt.Fprintln(&b, "    type: http_server")
		fmt.Fprintln(&b, "    args:")
		fmt.Fprintln(&b, "      listen: "+strconv.Quote(":"+strconv.Itoa(c.Port)))
		fmt.Fprintln(&b, "      cert: "+strconv.Quote(certFile))
		fmt.Fprintln(&b, "      key: "+strconv.Quote(keyFile))
		if c.HTTP3 {
			fmt.Fprintln(&b, "      enable_http3: true")
		}
		fmt.Fprintln(&b, "      entries:")
		fmt.Fprintln(&b, `        - path: "/dns-query"`)
		fmt.Fprintln(&b, "          exec: main")
	}
	if c.DoT {
		fmt.Fprintln(&b, "  - tag: dot_public")
		fmt.Fprintln(&b, "    type: tcp_server")
		fmt.Fprintln(&b, "    args:")
		fmt.Fprintln(&b, "      entry: main")
		fmt.Fprintln(&b, "      listen: "+strconv.Quote(":"+strconv.Itoa(c.DoTPort)))
		fmt.Fprintln(&b, "      cert: "+strconv.Quote(certFile))
		fmt.Fprintln(&b, "      key: "+strconv.Quote(keyFile))
	}
	if c.DoQ {
		fmt.Fprintln(&b, "  - tag: doq_public")
		fmt.Fprintln(&b, "    type: quic_server")
		fmt.Fprintln(&b, "    args:")
		fmt.Fprintln(&b, "      entry: main")
		fmt.Fprintln(&b, "      listen: "+strconv.Quote(":"+strconv.Itoa(c.DoQPort)))
		fmt.Fprintln(&b, "      cert: "+strconv.Quote(certFile))
		fmt.Fprintln(&b, "      key: "+strconv.Quote(keyFile))
	}
	return b.String(), nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".oxidns-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func systemctl(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

var unit = strings.Join([]string{"[Unit]", "Description=3x-ui OxiDNS", "After=network-online.target", "Wants=network-online.target", "[Service]", "Type=simple", "ExecStart=/etc/x-ui/oxidns/bin/oxidns start -c /etc/x-ui/oxidns/config.yaml -d /etc/x-ui/oxidns/data", "Restart=on-failure", "RestartSec=3", "NoNewPrivileges=true", "[Install]", "WantedBy=multi-user.target"}, string([]byte{10})) + string([]byte{10})

type releaseAsset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
}

type upstreamRelease struct {
	Tag    string         `json:"tag_name"`
	Assets []releaseAsset `json:"assets"`
}

func releaseAssetName() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "oxidns-x86_64-unknown-linux-musl.tar.gz", nil
	case "linux/arm64":
		return "oxidns-aarch64-unknown-linux-musl.tar.gz", nil
	default:
		return "", fmt.Errorf("OxiDNS поддерживается только на Linux amd64 и arm64 (%s/%s)", runtime.GOOS, runtime.GOARCH)
	}
}

func Latest(ctx context.Context) (string, *releaseAsset, error) {
	name, err := releaseAssetName()
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/svenshi/oxidns/releases/latest", nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", nil, fmt.Errorf("GitHub API: HTTP %d", resp.StatusCode)
	}
	var rel upstreamRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return "", nil, err
	}
	if len(rel.Tag) < 6 || !strings.HasPrefix(rel.Tag, "v") || len(rel.Tag) > 64 || strings.ContainsAny(rel.Tag, "/ ?#") {
		return "", nil, errors.New("неверная версия OxiDNS из GitHub")
	}
	for _, asset := range rel.Assets {
		expectedURL := "https://github.com/svenshi/oxidns/releases/download/" + rel.Tag + "/" + name
		if asset.Name == name && asset.URL == expectedURL && strings.HasPrefix(asset.Digest, "sha256:") && len(asset.Digest) == 71 {
			if _, err := hex.DecodeString(strings.TrimPrefix(asset.Digest, "sha256:")); err == nil {
				return rel.Tag, &asset, nil
			}
		}
	}
	return "", nil, errors.New("официальный архив OxiDNS или SHA-256 не найдены")
}

func installedVersion(ctx context.Context) string {
	if _, err := os.Stat(binaryPath); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, binaryPath, "--version").Output()
	if err != nil {
		return ""
	}
	for _, field := range strings.Fields(string(data)) {
		part := strings.TrimPrefix(field, "v")
		if len(part) > 0 && part[0] >= '0' && part[0] <= '9' && strings.Count(part, ".") >= 2 {
			return "v" + part
		}
	}
	return ""
}

func StatusWithUpdates(ctx context.Context) (Status, error) {
	s, err := CurrentStatus(ctx)
	if err != nil {
		return s, err
	}
	latest, _, err := Latest(ctx)
	if err != nil {
		s.Error = "Не удалось проверить GitHub: " + err.Error()
		return s, nil
	}
	s.LatestVersion = latest
	// An installed binary with an unreadable version should be replaceable by an official release.
	s.UpdateAvailable = s.Installed && s.InstalledVersion != latest
	return s, nil
}

func installRelease(ctx context.Context, asset releaseAsset) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 120 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub вернул HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if len(data) >= 32<<20 {
		return nil, errors.New("архив превышает ограничение 32 MiB")
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != strings.TrimPrefix(asset.Digest, "sha256:") {
		return nil, errors.New("контрольная сумма официального архива не совпадает")
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	for {
		hdr, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		if filepath.Base(hdr.Name) != "oxidns" {
			continue
		}
		if hdr.Size < 1 || hdr.Size > 32<<20 {
			return nil, errors.New("неверный размер бинарника")
		}
		data, err := io.ReadAll(io.LimitReader(archive, hdr.Size))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) != hdr.Size {
			return nil, errors.New("неполный бинарник OxiDNS")
		}
		return data, nil
	}
	return nil, errors.New("не найден исполняемый файл oxidns")
}

func installOnly(ctx context.Context) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("для OxiDNS требуется Linux с systemd")
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return errors.New("на сервере не запущен systemd. Установка внутри Docker пока не поддерживается")
	}
	if _, err := releaseAssetName(); err != nil {
		return err
	}
	// The systemd unit starts OxiDNS with -d, so its writable working
	// directory must exist before the first service start.
	if err := os.MkdirAll(filepath.Join(StateDir, "data"), 0700); err != nil {
		return err
	}
	if _, err := os.Stat(binaryPath); errors.Is(err, os.ErrNotExist) {
		_, asset, err := Latest(ctx)
		if err != nil {
			return err
		}
		executable, err := installRelease(ctx, *asset)
		if err != nil {
			return err
		}
		if err := atomicWrite(binaryPath, executable, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := atomicWrite(servicePath, []byte(unit), 0644); err != nil {
		return err
	}
	return systemctl(ctx, "daemon-reload")
}

func InstallOnly(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()
	return installOnly(ctx)
}

// CheckDNS tests a real upstream DNS resolution, not just a listening TCP socket.
// It is safe to invoke from the panel: the resolver can only dial the loopback
// listener managed by OxiDNS, regardless of the supplied hostname.
func CheckDNS(ctx context.Context) error {
	if _, err := os.Stat(binaryPath); err != nil {
		return errors.New("сначала установите OxiDNS")
	}
	return checkDNSAt(ctx, net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort)))
}

// checkDNSAt is used by the live DNS probe and the official-binary integration test.
func checkDNSAt(ctx context.Context, address string) error {
	lookupCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: 2 * time.Second}
			return dialer.DialContext(ctx, network, address)
		},
	}
	ips, err := resolver.LookupIP(lookupCtx, "ip4", "example.com")
	if err != nil {
		return fmt.Errorf("OxiDNS не разрешает домены: %w. Проверьте доступ к выбранному DNS-провайдеру (TCP 853) и настройки сервера", err)
	}
	if len(ips) == 0 {
		return errors.New("OxiDNS не возвращает IP-адреса. Проверьте выбранного DNS-провайдера")
	}
	return nil
}

func waitHealthy(ctx context.Context) error {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort))
	for attempt := 0; attempt < 12; attempt++ {
		conn, err := net.DialTimeout("tcp", address, 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return CheckDNS(ctx)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return errors.New("OxiDNS не открыл локальный DNS-порт после запуска")
}

// waitDoHReady verifies that the configured public HTTPS listener actually
// starts. Local DNS success alone does not prove that DoH was activated.
func waitDoHReady(ctx context.Context, port int) error {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	var lastErr error
	for attempt := 0; attempt < 12; attempt++ {
		dialCtx, cancel := context.WithTimeout(ctx, time.Second)
		conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", address)
		cancel()
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("HTTPS-порт DoH %d не отвечает после запуска: %w", port, lastErr)
}

func Upgrade(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()
	if _, err := os.Stat(binaryPath); err != nil {
		return errors.New("сначала установите OxiDNS")
	}
	latest, asset, err := Latest(ctx)
	if err != nil {
		return err
	}
	if installedVersion(ctx) == latest {
		return nil
	}
	executable, err := installRelease(ctx, *asset)
	if err != nil {
		return err
	}
	oldBinary, err := os.ReadFile(binaryPath)
	if err != nil {
		return err
	}
	c, err := Load()
	if err != nil {
		return err
	}
	if err := atomicWrite(binaryPath, executable, 0755); err != nil {
		return err
	}
	if c.Enabled {
		restartErr := systemctl(ctx, "restart", "x-ui-oxidns.service")
		if restartErr == nil {
			restartErr = waitHealthy(ctx)
		}
		if restartErr == nil && c.DoH {
			restartErr = waitDoHReady(ctx, c.Port)
		}
		if restartErr == nil && c.DoT {
			restartErr = waitDoHReady(ctx, c.DoTPort)
		}
		if restartErr != nil {
			// The original request may have timed out during a failed upgrade.
			// Always attempt to restore and recheck the previous executable.
			recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			restoreErr := atomicWrite(binaryPath, oldBinary, 0755)
			if restoreErr == nil {
				restoreErr = systemctl(recovery, "restart", "x-ui-oxidns.service")
			}
			if restoreErr == nil {
				restoreErr = waitHealthy(recovery)
			}
			if restoreErr == nil && c.DoH {
				restoreErr = waitDoHReady(recovery, c.Port)
			}
			if restoreErr == nil && c.DoT {
				restoreErr = waitDoHReady(recovery, c.DoTPort)
			}
			return errors.Join(restartErr, restoreErr)
		}
	}
	return nil
}

// Uninstall is idempotent: stop the dedicated service, remove only managed
// OxiDNS files, and discard DNS preferences once the removal succeeds.
// No sing-box templates, panel certificates, or host DNS settings are changed.
func Uninstall(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()
	_, statErr := os.Stat(servicePath)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	// A previous interrupted removal can leave the service loaded by
	// systemd even after its unit file has disappeared.
	active := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "x-ui-oxidns.service").Run() == nil
	if active || statErr == nil {
		if err := systemctl(ctx, "stop", "x-ui-oxidns.service"); err != nil {
			if !strings.Contains(err.Error(), "not loaded") && !strings.Contains(err.Error(), "not found") {
				return err
			}
		}
	}
	// A removed unit file may still have an enabled systemd symlink.
	enabled := exec.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", "x-ui-oxidns.service").Run() == nil
	if statErr == nil || enabled {
		if err := systemctl(ctx, "disable", "x-ui-oxidns.service"); err != nil {
			if !strings.Contains(err.Error(), "does not exist") && !strings.Contains(err.Error(), "not loaded") {
				return err
			}
		}
	}

	// Ensure any interrupted deletion is safe: no enabled DNS overlay can
	// remain while its service files and binary are being removed.
	cfg, err := Load()
	if err != nil {
		return err
	}
	cfg.Enabled = false
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := atomicWrite(statePath, data, 0600); err != nil {
		return err
	}
	for _, path := range []string{binaryPath, configPath, servicePath} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	// OxiDNS owns this directory; it contains only its runtime cache/data.
	if err := os.RemoveAll(filepath.Join(StateDir, "data")); err != nil {
		return err
	}
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Remove empty directories, but leave foreign/unexpected files alone.
	_ = os.Remove(filepath.Join(StateDir, "bin"))
	_ = os.Remove(StateDir)
	return nil
}

func Setup(ctx context.Context, c Settings, certFile, keyFile string) error {
	mu.Lock()
	defer mu.Unlock()
	normalizePorts(&c)
	c.Enabled = true
	if err := validate(c, certFile, keyFile); err != nil {
		return err
	}
	yaml, err := Generate(c, certFile, keyFile)
	if err != nil {
		return err
	}
	previous, err := Load()
	if err != nil {
		return err
	}
	// Read existing config before changing anything. Permission and I/O
	// errors must not erase a previously working DNS configuration.
	oldYAML, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("предыдущая конфигурация OxiDNS: %w", err)
	}
	oldState, err := os.ReadFile(statePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("предыдущие настройки OxiDNS: %w", err)
	}
	if err := installOnly(ctx); err != nil {
		return err
	}
	if err := atomicWrite(configPath, []byte(yaml), 0600); err != nil {
		return err
	}
	rollback := func() error {
		// The incoming HTTP request may already have timed out. A separate
		// bounded context is needed to restore the old systemd state.
		recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var restoreErr error
		if len(oldYAML) > 0 {
			restoreErr = errors.Join(restoreErr, atomicWrite(configPath, oldYAML, 0600))
		} else if err := os.Remove(configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			restoreErr = errors.Join(restoreErr, err)
		}
		if len(oldState) > 0 {
			restoreErr = errors.Join(restoreErr, atomicWrite(statePath, oldState, 0600))
		} else if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			restoreErr = errors.Join(restoreErr, err)
		}
		// An old configuration file can remain after Disable. Do not restart
		// it unless OxiDNS was actually enabled before this failed Setup.
		if previous.Enabled && len(oldYAML) > 0 {
			restoreErr = errors.Join(restoreErr, systemctl(recovery, "enable", "x-ui-oxidns.service"))
			restoreErr = errors.Join(restoreErr, systemctl(recovery, "restart", "x-ui-oxidns.service"))
		} else {
			restoreErr = errors.Join(restoreErr, systemctl(recovery, "stop", "x-ui-oxidns.service"))
			restoreErr = errors.Join(restoreErr, systemctl(recovery, "disable", "x-ui-oxidns.service"))
		}
		return restoreErr
	}
	if out, err := exec.CommandContext(ctx, binaryPath, "check", "-c", configPath).CombinedOutput(); err != nil {
		return errors.Join(fmt.Errorf("OxiDNS отклонил конфигурацию: %w (%s)", err, strings.TrimSpace(string(out))), rollback())
	}
	if err := systemctl(ctx, "enable", "x-ui-oxidns.service"); err != nil {
		return errors.Join(err, rollback())
	}
	if err := systemctl(ctx, "restart", "x-ui-oxidns.service"); err != nil {
		return errors.Join(err, rollback())
	}
	if err := waitHealthy(ctx); err != nil {
		return errors.Join(err, rollback())
	}
	if c.DoH {
		if err := waitDoHReady(ctx, c.Port); err != nil {
			return errors.Join(err, rollback())
		}
	}
	if c.DoT {
		if err := waitDoHReady(ctx, c.DoTPort); err != nil {
			return errors.Join(err, rollback())
		}
	}
	data, err := json.Marshal(c)
	if err != nil {
		return errors.Join(err, rollback())
	}
	if err := atomicWrite(statePath, data, 0600); err != nil {
		return errors.Join(err, rollback())
	}
	return nil
}

// Disable stops OxiDNS first and persists the disabled state only after
// systemd confirms both operations. Failures restore the running service.
func Disable(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	c, err := Load()
	if err != nil {
		return err
	}
	if !c.Enabled {
		// An interrupted setup or manual systemd start can leave the service
		// running despite disabled settings. Ensure "disable" is effective.
		if _, err := os.Stat(servicePath); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		if exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "x-ui-oxidns.service").Run() == nil {
			if err := systemctl(ctx, "stop", "x-ui-oxidns.service"); err != nil {
				return err
			}
		}
		if exec.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", "x-ui-oxidns.service").Run() == nil {
			return systemctl(ctx, "disable", "x-ui-oxidns.service")
		}
		return nil
	}
	if err := systemctl(ctx, "stop", "x-ui-oxidns.service"); err != nil {
		return err
	}
	if err := systemctl(ctx, "disable", "x-ui-oxidns.service"); err != nil {
		recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return errors.Join(err, systemctl(recovery, "enable", "x-ui-oxidns.service"),
			systemctl(recovery, "start", "x-ui-oxidns.service"))
	}
	c.Enabled = false
	data, err := json.Marshal(c)
	if err == nil {
		err = atomicWrite(statePath, data, 0600)
	}
	if err != nil {
		recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return errors.Join(err,
			systemctl(recovery, "enable", "x-ui-oxidns.service"),
			systemctl(recovery, "start", "x-ui-oxidns.service"))
	}
	return nil
}

const Tag = "oxidns-local"

// HasManagedServer detects a runtime-only DNS overlay in a JSON-decoded
// sing-box configuration, including after the OxiDNS package was removed.
func HasManagedServer(dns map[string]any) bool {
	if dns == nil {
		return false
	}
	switch servers := dns["servers"].(type) {
	case []any:
		for _, item := range servers {
			if row, ok := item.(map[string]any); ok && row["tag"] == Tag {
				return true
			}
		}
	case []map[string]any:
		for _, row := range servers {
			if row["tag"] == Tag {
				return true
			}
		}
	}
	return false
}

// WithSingBoxDNS leaves user DNS rules and upstreams intact, changing only the default.
func WithSingBoxDNS(src map[string]any) (map[string]any, error) {
	next := make(map[string]any, len(src)+2)
	for k, v := range src {
		next[k] = v
	}
	servers := make([]map[string]any, 0)
	switch rows := src["servers"].(type) {
	case []map[string]any:
		servers = append(servers, rows...)
	case []any:
		for _, v := range rows {
			m, ok := v.(map[string]any)
			if !ok {
				return nil, errors.New("неверный DNS-сервер sing-box")
			}
			servers = append(servers, m)
		}
	case nil:
	default:
		return nil, errors.New("неверный список DNS-серверов sing-box")
	}
	for _, row := range servers {
		if row["tag"] == Tag {
			return nil, fmt.Errorf("DNS-тег %q уже занят", Tag)
		}
	}
	servers = append(servers, map[string]any{"type": "udp", "tag": Tag, "server": "127.0.0.1", "server_port": localPort})
	next["servers"] = servers
	next["final"] = Tag
	return next, nil
}
