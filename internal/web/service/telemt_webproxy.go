package service

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	telemtWebStatePath  = "/etc/x-ui/telemt-web.json"
	telemtWebNginxConf  = "/etc/nginx/conf.d/3x-ui-telemt-web.conf"
	telemtWebAcmeConf   = "/etc/nginx/conf.d/3x-ui-telemt-web-acme.conf"
	telemtWebDecoyDir   = "/var/lib/x-ui/telemt-web"
	telemtWebListenIP   = "127.0.0.1"
	telemtWebListenPort = 15080
	telemtWebUser       = "webproxy"
	telemtWebMinEngine  = "3.5.1"
)

var telemtWebDomainPattern = regexp.MustCompile(
	`^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}package service

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	telemtWebStatePath  = "/etc/x-ui/telemt-web.json"
	telemtWebNginxConf  = "/etc/nginx/conf.d/3x-ui-telemt-web.conf"
	telemtWebAcmeConf   = "/etc/nginx/conf.d/3x-ui-telemt-web-acme.conf"
	telemtWebDecoyDir   = "/var/lib/x-ui/telemt-web"
	telemtWebListenIP   = "127.0.0.1"
	telemtWebListenPort = 15080
	telemtWebUser       = "webproxy"
	telemtWebMinEngine  = "3.5.1"
)

,
)
var telemtWebVersionPattern = regexp.MustCompile(`(?i)v?([0-9]+)\.([0-9]+)\.([0-9]+)`)
var telemtWebPortOwnerPattern = regexp.MustCompile(`users:\(\("([^"]+)"`)

type TelemtWebProxyState struct {
	Enabled    bool   `json:"enabled"`
	Domain     string `json:"domain"`
	Secret     string `json:"secret"`
	DecoyDir   string `json:"decoyDir"`
	ListenPort int    `json:"listenPort"`
	CertFile   string `json:"certFile"`
	KeyFile    string `json:"keyFile"`
}

type TelemtWebProxyStatus struct {
	Enabled          bool   `json:"enabled"`
	Supported        bool   `json:"supported"`
	Domain           string `json:"domain"`
	DefaultDomain    string `json:"defaultDomain"`
	Link             string `json:"link"`
	NginxInstalled   bool   `json:"nginxInstalled"`
	NginxActive      bool   `json:"nginxActive"`
	CertificateReady bool   `json:"certificateReady"`
	CertificateFile  string `json:"certificateFile"`
	ListenPort       int    `json:"listenPort"`
	Port443Available bool   `json:"port443Available"`
	Port443Owner     string `json:"port443Owner"`
	Error            string `json:"error"`
}

func readTelemtWebState() (TelemtWebProxyState, error) {
	data, err := os.ReadFile(telemtWebStatePath)
	if errors.Is(err, os.ErrNotExist) {
		return TelemtWebProxyState{DecoyDir: telemtWebDecoyDir, ListenPort: telemtWebListenPort}, nil
	}
	if err != nil { return TelemtWebProxyState{}, err }
	var state TelemtWebProxyState
	if err := json.Unmarshal(data, &state); err != nil { return TelemtWebProxyState{}, fmt.Errorf("telemt web proxy: parse state: %w", err) }
	if state.DecoyDir == "" { state.DecoyDir = telemtWebDecoyDir }
	if state.ListenPort == 0 { state.ListenPort = telemtWebListenPort }
	return state, nil
}

func writeTelemtWebState(state TelemtWebProxyState) error {
	if err := os.MkdirAll(filepath.Dir(telemtWebStatePath), 0o700); err != nil { return err }
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil { return err }
	tmp := telemtWebStatePath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil { return err }
	if err := os.Rename(tmp, telemtWebStatePath); err != nil { _ = os.Remove(tmp); return err }
	return nil
}

func clearTelemtWebState() error {
	err := os.Remove(telemtWebStatePath)
	if errors.Is(err, os.ErrNotExist) { return nil }
	return err
}

func appendTelemtWebProxyConfig(base string, state TelemtWebProxyState) (string, error) {
	if !state.Enabled { return base, nil }
	if !telemtWebDomainPattern.MatchString(state.Domain) || len(state.Domain) > 253 { return "", errors.New("telemt web proxy: invalid domain") }
	if !regexp.MustCompile("^[0-9a-fA-F]{32}$").MatchString(state.Secret) { return "", errors.New("telemt web proxy: invalid secret") }
	base = removeTelemtWebUser(base)
	section := "[access.users]"
	idx := strings.Index(base, section)
	if idx < 0 { return "", errors.New("telemt web proxy: access.users section is missing") }
	insertAt := len(base)
	if next := strings.Index(base[idx+len(section):], "\n["); next >= 0 { insertAt = idx + len(section) + next + 1 }
base = base[:insertAt] + fmt.Sprintf("%s = \"%s\"\n", telemtWebUser, state.Secret) + base[insertAt:]
	decoyDir := state.DecoyDir; if decoyDir == "" { decoyDir = telemtWebDecoyDir }
	listenPort := state.ListenPort; if listenPort == 0 { listenPort = telemtWebListenPort }
	webConfig := fmt.Sprintf("\n[[server.listeners]]\nip = \"%s\"\nport = %d\ntransport = \"web\"\nproxy_protocol = false\nreuse_allow = false\nweb_client_ip_source = \"x_forwarded_for\"\nweb_trusted_proxy_cidrs = [\"127.0.0.1/32\"]\n\n[web]\nenabled = true\ncarrier = \"websocket\"\n\n[[web.vhosts]]\nhost = \"%s\"\npublic_addr = \"%s:443\"\n\n[web.vhosts.decoy]\nmode = \"static_directory\"\ndirectory = \"%s\"\nindex = \"index.html\"\n\n[[web.vhosts.profiles]]\nuser = \"%s\"\nsecret_mode = \"dd\"\n", telemtWebListenIP, listenPort, state.Domain, state.Domain, decoyDir, telemtWebUser)
	return strings.TrimRight(base, "\n") + "\n" + webConfig, nil
}

func removeTelemtWebUser(base string) string {
	lines := strings.Split(base, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines { if !strings.HasPrefix(strings.TrimSpace(line), telemtWebUser+" = ") { out = append(out, line) } }
	return strings.Join(out, "\n")
}

func (TelemtService) WebProxyStatus(defaultDomain, panelCert, panelKey string) (TelemtWebProxyStatus, error) {
	state, err := readTelemtWebState(); if err != nil { return TelemtWebProxyStatus{}, err }
	version := strings.TrimPrefix(telemtVersion(), "v")
	nginxActive := systemctl("is-active", "--quiet", "nginx") == nil
	nginxOwns443 := telemtWebNginxOwnsPort443()
	portOwner := telemtWebPortOwner(443)
	status := TelemtWebProxyStatus{Enabled: state.Enabled, Supported: telemtWebVersionAtLeast(version, telemtWebMinEngine), Domain: state.Domain, DefaultDomain: defaultDomain, NginxInstalled: telemtWebCommandExists("nginx"), NginxActive: nginxActive, Port443Available: telemtWebNginxCanOwn443(), Port443Owner: portOwner, ListenPort: state.ListenPort, CertificateFile: state.CertFile}
	if state.Enabled && state.Domain != "" && state.Secret != "" { status.Link = telemtWebLink(state.Domain, state.Secret) }
	if state.CertFile != "" { status.CertificateReady = telemtWebCertificateCoversDomain(state.CertFile, state.Domain) && telemtWebFileReadable(state.KeyFile) } else if panelCert != "" { status.CertificateReady = telemtWebCertificateCoversDomain(panelCert, state.Domain) && telemtWebFileReadable(panelKey); status.CertificateFile = panelCert }
	if !status.Supported { status.Error = fmt.Sprintf("Telemt %s does not support WEB Proxy; requires %s or newer", version, telemtWebMinEngine) }
	if !status.Port443Available && !nginxOwns443 {
		if portOwner != "" {
			status.Error = fmt.Sprintf("Порт 443 занят процессом «%s». Остановите этот сервис или освободите порт 443 для Nginx.", portOwner)
		} else {
			status.Error = "Порт 443 занят другим сервисом. Освободите порт 443 для Nginx."
		}
	}
	return status, nil
}

func (TelemtService) EnableWebProxy(ctx context.Context, domain, panelCert, panelKey string) (TelemtWebProxyStatus, error) {
	domain = normalizeTelemtWebDomain(domain)
	if !telemtWebDomainPattern.MatchString(domain) || len(domain) > 253 { return TelemtWebProxyStatus{}, errors.New("WEB Proxy requires a public FQDN, for example web.example.com") }
	if os.Geteuid() != 0 { return TelemtWebProxyStatus{}, errors.New("WEB Proxy setup requires root privileges") }
	version := strings.TrimPrefix(telemtVersion(), "v")
	if !telemtWebVersionAtLeast(version, telemtWebMinEngine) { return TelemtWebProxyStatus{}, fmt.Errorf("Telemt %s does not support WEB Proxy; requires %s or newer", version, telemtWebMinEngine) }
	if !telemtWebNginxCanOwn443() {
		owner := telemtWebPortOwner(443)
		if owner != "" {
			return TelemtWebProxyStatus{}, fmt.Errorf("Порт 443 занят процессом «%s». Освободите порт 443 для Nginx.", owner)
		}
		return TelemtWebProxyStatus{}, errors.New("Порт 443 занят другим сервисом. Освободите порт 443 для Nginx.")
	}
	if err := telemtWebEnsureNginxPackage(ctx); err != nil { return TelemtWebProxyStatus{}, err }
	if err := writeTelemtWebDecoy(domain, telemtWebDecoyDir); err != nil { return TelemtWebProxyStatus{}, err }
	state, err := readTelemtWebState(); if err != nil { return TelemtWebProxyStatus{}, err }
	oldState := state
	certFile, keyFile, err := telemtWebEnsureCertificate(ctx, domain, panelCert, panelKey); if err != nil { return TelemtWebProxyStatus{}, err }
	if err := installTelemtWebRenewalHook(); err != nil { return TelemtWebProxyStatus{}, err }
	if state.Secret == "" { buf := make([]byte, 16); if _, err := rand.Read(buf); err != nil { return TelemtWebProxyStatus{}, err }; state.Secret = hex.EncodeToString(buf) }
	state.Enabled = true; state.Domain = domain; state.DecoyDir = telemtWebDecoyDir; state.ListenPort = telemtWebListenPort; state.CertFile = certFile; state.KeyFile = keyFile
	if err := writeTelemtWebState(state); err != nil { return TelemtWebProxyStatus{}, err }
	cfg, err := (TelemtService{}).GetConfig(); if err != nil { _ = writeTelemtWebState(oldState); return TelemtWebProxyStatus{}, err }
	if err := (TelemtService{}).SaveConfig(cfg); err != nil { _ = writeTelemtWebState(oldState); return TelemtWebProxyStatus{}, err }
	if err := writeTelemtWebNginxConfig(state); err != nil { _ = writeTelemtWebState(oldState); _ = (TelemtService{}).SaveConfig(cfg); return TelemtWebProxyStatus{}, err }
	if err := telemtWebEnsureNginxRunning(); err != nil { _ = removeTelemtWebNginxConfig(); _ = writeTelemtWebState(oldState); _ = (TelemtService{}).SaveConfig(cfg); return TelemtWebProxyStatus{}, err }
	return (TelemtService{}).WebProxyStatus(domain, panelCert, panelKey)
}

func (TelemtService) DisableWebProxy() error {
	state, err := readTelemtWebState(); if err != nil { return err }
	if !state.Enabled { _ = removeTelemtWebNginxConfig(); return nil }
	oldState := state; state.Enabled = false
	if err := writeTelemtWebState(state); err != nil { return err }
	cfg, err := (TelemtService{}).GetConfig(); if err != nil { _ = writeTelemtWebState(oldState); return err }
	if err := (TelemtService{}).SaveConfig(cfg); err != nil { _ = writeTelemtWebState(oldState); return err }
	if err := removeTelemtWebNginxConfig(); err != nil { _ = writeTelemtWebState(oldState); _ = (TelemtService{}).SaveConfig(cfg); return err }
	if err := telemtWebReloadNginx(); err != nil { _ = writeTelemtWebState(oldState); _ = (TelemtService{}).SaveConfig(cfg); _ = writeTelemtWebNginxConfig(oldState); _ = telemtWebReloadNginx(); return err }
	return clearTelemtWebState()
}

func normalizeTelemtWebDomain(domain string) string { domain = strings.TrimSpace(domain); domain = strings.TrimPrefix(domain, "https://"); domain = strings.TrimPrefix(domain, "http://"); return strings.TrimRight(domain, "/") }

func telemtWebLink(domain, secret string) string { return fmt.Sprintf("tg://webproxy?server=%s&port=443&secret=dd%s", domain, secret) }

func telemtWebCertificateCoversDomain(path, domain string) bool {
	if path == "" || domain == "" { return false }
	data, err := os.ReadFile(path); if err != nil { return false }
	block, _ := pem.Decode(data); if block == nil { return false }
	cert, err := x509.ParseCertificate(block.Bytes); if err != nil { return false }
	return cert.NotBefore.Before(time.Now()) && cert.NotAfter.After(time.Now()) && cert.VerifyHostname(domain) == nil
}

func writeTelemtWebDecoy(domain, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil { return err }
	content := fmt.Sprintf("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>%s</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;font-family:system-ui,-apple-system,sans-serif;background:#f5f7fa;color:#222}main{max-width:720px;padding:48px;text-align:center}h1{font-size:48px;font-weight:600}p{font-size:18px;color:#667085}</style></head><body><main><h1>%s</h1><p>Welcome. This website is protected by HTTPS.</p></main></body></html>", domain, domain)
	return os.WriteFile(filepath.Join(dir, "index.html"), []byte(content), 0o644)
}

func writeTelemtWebNginxConfig(state TelemtWebProxyState) error {
	if state.CertFile == "" || state.KeyFile == "" { return errors.New("WEB Proxy certificate is not configured") }
	if !telemtWebCertificateCoversDomain(state.CertFile, state.Domain) { return fmt.Errorf("certificate does not cover domain %s", state.Domain) }
	config := fmt.Sprintf("server {\n    listen 443 ssl;\n    server_name %s;\n\n    ssl_certificate %s;\n    ssl_certificate_key %s;\n    ssl_protocols TLSv1.2 TLSv1.3;\n\n    location / {\n        proxy_pass http://%s:%d;\n        proxy_http_version 1.1;\n        proxy_set_header Host $host;\n        proxy_set_header X-Forwarded-For $remote_addr;\n        proxy_set_header Upgrade $http_upgrade;\n        proxy_set_header Connection \"upgrade\";\n        proxy_read_timeout 120s;\n        proxy_send_timeout 120s;\n        proxy_buffering off;\n    }\n}\n", state.Domain, state.CertFile, state.KeyFile, telemtWebListenIP, state.ListenPort)
	if err := os.MkdirAll(filepath.Dir(telemtWebNginxConf), 0o755); err != nil { return err }
	return os.WriteFile(telemtWebNginxConf, []byte(config), 0o644)
}

func removeTelemtWebNginxConfig() error { err := os.Remove(telemtWebNginxConf); if errors.Is(err, os.ErrNotExist) { return nil }; return err }

func telemtWebEnsureNginxRunning() error {
	if err := exec.Command("nginx", "-t").Run(); err != nil { return errors.New("nginx configuration test failed") }
	if systemctl("enable", "--now", "nginx") == nil { return nil }
	if _, err := exec.LookPath("rc-service"); err == nil { if err := exec.CommandContext(context.Background(), "rc-service", "nginx", "restart").Run(); err != nil { return fmt.Errorf("failed to start nginx: %w", err) }; _, _ = exec.CommandContext(context.Background(), "rc-update", "add", "nginx", "default").Output(); return nil }
	return errors.New("failed to start nginx")
}

func telemtWebReloadNginx() error {
	if _, err := exec.LookPath("systemctl"); err == nil { if err := systemctl("reload", "nginx"); err == nil { return nil } }
	if _, err := exec.LookPath("rc-service"); err == nil { if err := exec.CommandContext(context.Background(), "rc-service", "nginx", "reload").Run(); err == nil { return nil } }
	return errors.New("failed to reload nginx")
}

func telemtWebEnsureNginxPackage(ctx context.Context) error {
	if telemtWebCommandExists("nginx") { return nil }
	manager := telemtWebPackageManager(); if manager == "" { return errors.New("cannot install nginx automatically: unsupported Linux package manager") }
	var args []string
	switch manager {
	case "apt-get": args = []string{"apt-get", "update"}; if err := runTelemtWebCommand(ctx, args...); err != nil { return err }; args = []string{"apt-get", "install", "-y", "nginx"}
	case "dnf": args = []string{"dnf", "install", "-y", "nginx"}
	case "yum": args = []string{"yum", "install", "-y", "nginx"}
	case "pacman": args = []string{"pacman", "-Syu", "--noconfirm", "--needed", "nginx"}
	case "zypper": args = []string{"zypper", "--non-interactive", "install", "nginx"}
	case "apk": args = []string{"apk", "add", "--no-cache", "nginx"}
	default: return errors.New("unsupported package manager")
	}
	return runTelemtWebCommand(ctx, args...)
}

func telemtWebEnsureCertificate(ctx context.Context, domain, panelCert, panelKey string) (string, string, error) {
	if telemtWebCertificateCoversDomain(panelCert, domain) && telemtWebFileReadable(panelKey) { return panelCert, panelKey, nil }
	acmeCert := "/etc/letsencrypt/live/" + domain + "/fullchain.pem"; acmeKey := "/etc/letsencrypt/live/" + domain + "/privkey.pem"
	if telemtWebCertificateCoversDomain(acmeCert, domain) && telemtWebFileReadable(acmeKey) { return acmeCert, acmeKey, nil }
	if !telemtWebPortAvailable(80) && !telemtWebNginxOwnsPort(80) { return "", "", errors.New("automatic certificate issuance needs port 80 to be available or owned by nginx") }
	if err := telemtWebWriteAcmeNginxConfig(domain); err != nil { return "", "", err }
	defer removeTelemtWebAcmeConfig()
	if err := telemtWebEnsureNginxRunning(); err != nil { return "", "", err }
	if !telemtWebCommandExists("certbot") { if err := telemtWebInstallCertbot(ctx, telemtWebPackageManager()); err != nil { return "", "", err } }
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute); defer cancel()
	cmd := exec.CommandContext(timeoutCtx, "certbot", "certonly", "--webroot", "-w", telemtWebDecoyDir, "--non-interactive", "--agree-tos", "--register-unsafely-without-email", "-d", domain, "--preferred-challenges", "http")
	if out, err := cmd.CombinedOutput(); err != nil { return "", "", fmt.Errorf("certbot failed: %s", strings.TrimSpace(string(out))) }
	if !telemtWebCertificateCoversDomain(acmeCert, domain) || !telemtWebFileReadable(acmeKey) { return "", "", errors.New("certbot completed but certificate files are missing or invalid") }
	if err := installTelemtWebRenewalHook(); err != nil { return "", "", err }
	return acmeCert, acmeKey, nil
}

func telemtWebWriteAcmeNginxConfig(domain string) error {
	if err := os.MkdirAll(filepath.Join(telemtWebDecoyDir, ".well-known", "acme-challenge"), 0o755); err != nil { return err }
	conf := fmt.Sprintf("server {\n    listen 80;\n    server_name %s;\n    root %s;\n    location /.well-known/acme-challenge/ {\n        try_files $uri =404;\n    }\n}\n", domain, telemtWebDecoyDir)
	return os.WriteFile(telemtWebAcmeConf, []byte(conf), 0o644)
}

func removeTelemtWebAcmeConfig() { _ = os.Remove(telemtWebAcmeConf); _ = telemtWebReloadNginx() }


func installTelemtWebRenewalHook() error {
	dir := "/etc/letsencrypt/renewal-hooks/deploy"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "3x-ui-telemt-web.sh")
	script := "#!/bin/sh\n" +
		"systemctl reload nginx >/dev/null 2>&1 || rc-service nginx reload >/dev/null 2>&1 || true\n"
	return os.WriteFile(path, []byte(script), 0o755)
}

func telemtWebInstallCertbot(ctx context.Context, manager string) error {
	var args []string
	switch manager {
	case "apt-get": args = []string{"apt-get", "update"}; if err := runTelemtWebCommand(ctx, args...); err != nil { return err }; args = []string{"apt-get", "install", "-y", "certbot"}
	case "dnf": args = []string{"dnf", "install", "-y", "certbot"}
	case "yum": args = []string{"yum", "install", "-y", "certbot"}
	case "pacman": args = []string{"pacman", "-Syu", "--noconfirm", "--needed", "certbot"}
	case "zypper": args = []string{"zypper", "--non-interactive", "install", "certbot"}
	case "apk": args = []string{"apk", "add", "--no-cache", "certbot"}
	default: return errors.New("unsupported package manager")
	}
	return runTelemtWebCommand(ctx, args...)
}

func telemtWebPackageManager() string {
	data, err := os.ReadFile("/etc/os-release"); if err != nil { return "" }
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") { parts := strings.SplitN(line, "=", 2); if len(parts) == 2 { values[parts[0]] = strings.Trim(strings.TrimSpace(parts[1]), "\"") } }
	id := strings.ToLower(values["ID"]); like := strings.ToLower(values["ID_LIKE"])
	switch {
	case id == "ubuntu" || id == "debian" || strings.Contains(like, "debian"): return "apt-get"
	case id == "fedora": return "dnf"
	case id == "rhel" || id == "almalinux" || id == "rocky" || id == "ol" || id == "amzn" || strings.Contains(like, "rhel") || strings.Contains(like, "fedora"): if telemtWebCommandExists("dnf") { return "dnf" }; return "yum"
	case id == "centos": if telemtWebCommandExists("dnf") { return "dnf" }; return "yum"
	case id == "arch" || id == "manjaro" || id == "parch" || strings.Contains(like, "arch"): return "pacman"
	case strings.Contains(id, "opensuse") || strings.Contains(like, "suse"): return "zypper"
	case id == "alpine": return "apk"
	default: return ""
	}
}

func telemtWebPortAvailable(port int) bool {
	if telemtWebCommandExists("ss") {
		return exec.CommandContext(
			context.Background(),
			"ss",
			"-lnt",
			fmt.Sprintf("sport = :%d", port),
		).Run() != nil
	}
	listenConfig := net.ListenConfig{}; listener, err := listenConfig.Listen(context.Background(), "tcp", fmt.Sprintf(":%d", port)); if err != nil { return false }; _ = listener.Close(); return true
}

func telemtWebPortOwner(port int) string {
	if !telemtWebCommandExists("ss") {
		return ""
	}
	output, err := exec.CommandContext(
		context.Background(),
		"ss",
		"-lntpH",
		fmt.Sprintf("sport = :%d", port),
	).CombinedOutput()
	if err != nil {
		return ""
	}
	seen := map[string]struct{}{}
	owners := make([]string, 0, 2)
	for _, match := range telemtWebPortOwnerPattern.FindAllStringSubmatch(string(output), -1) {
		if len(match) < 2 || match[1] == "" {
			continue
		}
		if _, ok := seen[match[1]]; ok {
			continue
		}
		seen[match[1]] = struct{}{}
		owners = append(owners, match[1])
	}
	return strings.Join(owners, ", ")
}

func telemtWebNginxCanOwn443() bool { if systemctl("is-active", "--quiet", "nginx") == nil { return telemtWebNginxOwnsPort443() || telemtWebPortAvailable(443) }; return telemtWebPortAvailable(443) }

func telemtWebNginxOwnsPort(port int) bool {
	if !telemtWebCommandExists("ss") { return false }
	output, err := exec.CommandContext(
		context.Background(),
		"ss",
		"-lntp",
		fmt.Sprintf("sport = :%d", port),
	).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "nginx")
}

func telemtWebNginxOwnsPort443() bool { return telemtWebNginxOwnsPort(443) }

func telemtWebVersionAtLeast(current, required string) bool {
	currentMatch := telemtWebVersionPattern.FindStringSubmatch(current); requiredMatch := telemtWebVersionPattern.FindStringSubmatch(required)
	if len(currentMatch) != 4 || len(requiredMatch) != 4 { return false }
	for i := 1; i <= 3; i++ { cv := atoiTelemtWeb(currentMatch[i]); rv := atoiTelemtWeb(requiredMatch[i]); if cv != rv { return cv > rv } }
	return true
}

func atoiTelemtWeb(value string) int { n := 0; for _, r := range value { n = n*10 + int(r-'0') }; return n }

func telemtWebCommandExists(name string) bool { _, err := exec.LookPath(name); return err == nil }
func telemtWebFileReadable(path string) bool { if path == "" { return false }; _, err := os.Stat(path); return err == nil }

func runTelemtWebCommand(ctx context.Context, args ...string) error {
	if len(args) == 0 { return errors.New("empty package command") }
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Minute); defer cancel()
	output, err := exec.CommandContext(timeoutCtx, args[0], args[1:]...).CombinedOutput(); if err != nil { return fmt.Errorf("%s failed: %s", args[0], strings.TrimSpace(string(output))) }; return nil
}