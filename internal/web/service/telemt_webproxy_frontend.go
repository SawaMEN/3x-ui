package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const telemtWebFrontUnit = "3x-ui-telemt-web-front.service"
const telemtWebFrontDir = "/etc/x-ui/telemt-web-front"

var telemtWebOperationMu sync.Mutex

type TelemtWebProxyOptions struct {
	Frontend string   `json:"frontend"`
	Carrier  string   `json:"carrier"`
	Carriers []string `json:"carriers"`
}

func normalizeTelemtWebOptions(o TelemtWebProxyOptions) (TelemtWebProxyOptions, error) {
	if o.Frontend == "" {
		o.Frontend = "nginx"
	}
	if o.Carrier == "" {
		o.Carrier = "https"
	}
	if !slices.Contains([]string{"nginx", "caddy", "haproxy"}, o.Frontend) {
		return o, errors.New("WEB Proxy frontend must be nginx, caddy or haproxy")
	}
	allowed := []string{"https", "https-lanes", "websocket", "websocket-lanes"}
	if !slices.Contains(allowed, o.Carrier) {
		return o, errors.New("invalid WEB Proxy fallback carrier")
	}
	seen := map[string]bool{}
	for _, c := range o.Carriers {
		if !slices.Contains(allowed, c) || seen[c] {
			return o, errors.New("WEB Proxy carriers must be unique supported transports")
		}
		seen[c] = true
	}
	return o, nil
}

func telemtWebFrontend(s TelemtWebProxyState) string {
	if s.Frontend == "" {
		return "nginx"
	}
	return s.Frontend
}
func telemtWebCarrier(s TelemtWebProxyState) string {
	if s.Carrier == "" {
		return "https"
	}
	return s.Carrier
}

func (s TelemtService) EnableWebProxy(ctx context.Context, domain, cert, key string, options ...TelemtWebProxyOptions) (TelemtWebProxyStatus, error) {
	telemtWebOperationMu.Lock()
	defer telemtWebOperationMu.Unlock()
	domain = normalizeTelemtWebDomain(domain)
	if !telemtWebDomainPattern.MatchString(domain) || len(domain) > 253 {
		return TelemtWebProxyStatus{}, errors.New("WEB Proxy requires a public FQDN")
	}
	if os.Geteuid() != 0 {
		return TelemtWebProxyStatus{}, errors.New("WEB Proxy setup requires root privileges")
	}
	if !telemtWebVersionAtLeast(telemtVersion(), telemtWebMinEngine) {
		return TelemtWebProxyStatus{}, errors.New("Telemt does not support WEB Proxy")
	}
	old, err := readTelemtWebState()
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	o := TelemtWebProxyOptions{Frontend: old.Frontend, Carrier: old.Carrier, Carriers: old.Carriers}
	if len(options) > 0 {
		if options[0].Frontend != "" {
			o.Frontend = options[0].Frontend
		}
		if options[0].Carrier != "" {
			o.Carrier = options[0].Carrier
		}
		if options[0].Carriers != nil {
			o.Carriers = options[0].Carriers
		}
	}
	o, err = normalizeTelemtWebOptions(o)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	if (len(o.Carriers) > 0 || o.Carrier != "https") && !telemtWebVersionAtLeast(telemtVersion(), "3.5.4") {
		return TelemtWebProxyStatus{}, errors.New("WEB carrier selection requires Telemt 3.5.4 or newer")
	}
	// Fail before changing packages, certificates, state or listeners.
	preflight, err := s.PreflightWebProxy(ctx, domain, o, cert, key)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	if !preflight.Ready {
		for _, check := range preflight.Checks {
			if !check.OK {
				return TelemtWebProxyStatus{}, fmt.Errorf("WEB Proxy preflight (%s): %s", check.ID, check.Message)
			}
		}
		return TelemtWebProxyStatus{}, errors.New("WEB Proxy prerequisites are not met")
	}
	snapshot, err := s.CaptureWebProxyEnableSnapshot(domain)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	if old.Enabled && telemtWebFrontend(old) != o.Frontend {
		if err := disableTelemtWebProxy(); err != nil {
			return TelemtWebProxyStatus{}, err
		}
		old.Enabled = false
		if err := writeTelemtWebState(old); err != nil {
			return TelemtWebProxyStatus{}, withTelemtRollbackError(err, s.RestoreWebProxyEnableSnapshot(snapshot))
		}
	}
	result, err := enableTelemtWebProxy(ctx, domain, cert, key, o)
	if err == nil {
		err = s.EnsureWebProxyBackend()
		if err == nil {
			current, readErr := readTelemtWebState()
			if readErr != nil {
				err = readErr
			} else {
				err = telemtWebWaitFrontendReady(ctx, current)
			}
		}
	}
	if err != nil {
		return result, withTelemtRollbackError(err, s.RestoreWebProxyEnableSnapshot(snapshot))
	}
	return result, nil
}

func (TelemtService) DisableWebProxy() error {
	telemtWebOperationMu.Lock()
	defer telemtWebOperationMu.Unlock()
	if err := disableTelemtWebProxy(); err != nil {
		return err
	}
	return cleanupWebProxyBootUnit()
}

func telemtWebFrontendActive(s TelemtWebProxyState) bool {
	if telemtWebFrontend(s) == "nginx" {
		return telemtWebNginxActive()
	}
	return systemctl("is-active", "--quiet", telemtWebFrontUnit) == nil
}
func telemtWebFrontendEnabled(s TelemtWebProxyState) bool {
	if telemtWebFrontend(s) == "nginx" {
		return telemtWebNginxEnabled()
	}
	return systemctl("is-enabled", "--quiet", telemtWebFrontUnit) == nil
}
func telemtWebFrontendCanOwn443(s TelemtWebProxyState) bool {
	if telemtWebFrontend(s) == "nginx" {
		return telemtWebNginxCanOwn443()
	}
	return telemtWebPortAvailable(443) || telemtWebFrontendActive(s)
}

func telemtWebEnsureFrontendPackage(ctx context.Context, s TelemtWebProxyState) error {
	front := telemtWebFrontend(s)
	if front == "nginx" {
		return telemtWebEnsureNginxPackage(ctx)
	}
	if !telemtWebCommandExists("systemctl") {
		return errors.New("Caddy/HAProxy WEB frontend requires systemd")
	}
	if telemtWebCommandExists(front) {
		return nil
	}
	manager := telemtWebPackageManager()
	var args []string
	switch manager {
	case "apt-get":
		if err := runTelemtWebCommand(ctx, "apt-get", "update"); err != nil {
			return err
		}
		args = []string{manager, "install", "-y", front}
	case "dnf", "yum":
		args = []string{manager, "install", "-y", front}
	case "apk":
		args = []string{manager, "add", "--no-cache", front}
	case "zypper":
		args = []string{manager, "--non-interactive", "install", front}
	case "pacman":
		args = []string{manager, "-S", "--noconfirm", "--needed", front}
	default:
		return fmt.Errorf("cannot install %s automatically: unsupported package manager", front)
	}
	if err := runTelemtWebCommand(ctx, args...); err != nil {
		return err
	}
	// Only a newly installed package service is stopped; existing services are untouched.
	return systemctl("disable", "--now", front+".service")
}

func telemtWebFrontendCertificate(ctx context.Context, s TelemtWebProxyState, domain, cert, key string) (string, string, error) {
	if telemtWebFrontend(s) == "nginx" {
		return telemtWebEnsureCertificate(ctx, domain, cert, key)
	}
	if telemtWebCertificateCoversDomain(cert, domain) && telemtWebFileReadable(key) {
		return cert, key, nil
	}
	cert = "/etc/letsencrypt/live/" + domain + "/fullchain.pem"
	key = "/etc/letsencrypt/live/" + domain + "/privkey.pem"
	if telemtWebCertificateCoversDomain(cert, domain) && telemtWebFileReadable(key) {
		return cert, key, nil
	}
	if !telemtWebPortAvailable(80) {
		return "", "", errors.New("automatic certificate issuance requires free port 80 or an existing certificate for this domain")
	}
	if !telemtWebCommandExists("certbot") {
		if err := telemtWebInstallCertbot(ctx, telemtWebPackageManager()); err != nil {
			return "", "", err
		}
	}
	if err := runTelemtWebCommand(ctx, "certbot", "certonly", "--standalone", "--non-interactive", "--agree-tos", "--register-unsafely-without-email", "--preferred-challenges", "http", "--cert-name", domain, "-d", domain); err != nil {
		return "", "", err
	}
	if !telemtWebCertificateCoversDomain(cert, domain) || !telemtWebFileReadable(key) {
		return "", "", errors.New("issued WEB certificate is missing or invalid")
	}
	return cert, key, nil
}

type telemtWebSubscriptionTarget struct {
	Paths         []string
	Upstream      string
	TLSCAFile     string
	TLSServerName string
}

func telemtWebFrontendConfig(s TelemtWebProxyState, sub telemtWebSubscriptionTarget) ([]byte, error) {
	if !telemtWebDomainPattern.MatchString(s.Domain) || s.ListenPort < 1 || s.ListenPort > 65535 {
		return nil, errors.New("invalid WEB frontend domain or port")
	}
	if len(sub.Paths) > 0 {
		u, err := url.Parse(sub.Upstream)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, errors.New("invalid panel subscription upstream")
		}
		if u.Scheme == "https" && (sub.TLSCAFile == "" || sub.TLSServerName == "") {
			return nil, errors.New("panel HTTPS subscription routes require a trusted certificate and server name")
		}
	}
	backend := net.JoinHostPort(telemtWebListenIP, strconv.Itoa(s.ListenPort))
	switch telemtWebFrontend(s) {
	case "caddy":
		proxy := func(address string) map[string]any {
			target, _ := url.Parse(address)
			transport := map[string]any{"protocol": "http", "versions": []string{"1.1"}}
			if target.Scheme == "https" {
				transport["tls"] = map[string]any{"server_name": sub.TLSServerName, "root_ca_pem_files": []string{sub.TLSCAFile}}
			}
			return map[string]any{"handler": "reverse_proxy", "upstreams": []any{map[string]any{"dial": target.Host}}, "flush_interval": -1, "headers": map[string]any{"request": map[string]any{"set": map[string][]string{"X-Forwarded-For": {"{http.request.remote.host}"}, "X-Forwarded-Proto": {"https"}, "Host": {"{http.request.host}"}}}}, "transport": transport}
		}
		routes := []any{}
		if len(sub.Paths) > 0 {
			routes = append(routes, map[string]any{"match": []any{map[string]any{"path": sub.Paths}}, "handle": []any{proxy(sub.Upstream)}, "terminal": true})
		}
		routes = append(routes, map[string]any{"handle": []any{proxy("http://" + backend)}})
		server := map[string]any{"listen": []string{":443"}, "automatic_https": map[string]any{"disable": true}, "tls_connection_policies": []any{map[string]any{}}, "routes": []any{map[string]any{"match": []any{map[string]any{"host": []string{s.Domain}}}, "handle": []any{map[string]any{"handler": "subroute", "routes": routes}}, "terminal": true}, map[string]any{"handle": []any{map[string]any{"handler": "static_response", "status_code": 404}}}}}
		config := map[string]any{"admin": map[string]any{"listen": "unix//run/3x-ui-telemt-front/caddy.sock", "config": map[string]any{"persist": false}}, "apps": map[string]any{"tls": map[string]any{"certificates": map[string]any{"load_files": []any{map[string]any{"certificate": s.CertFile, "key": s.KeyFile}}}}, "http": map[string]any{"servers": map[string]any{"web": server}}}}
		return json.MarshalIndent(config, "", "  ")
	case "haproxy":
		bind := ":443"
		if host, _, err := net.SplitHostPort(s.PublicAddr); err == nil && net.ParseIP(host) != nil && net.ParseIP(host).To4() == nil {
			bind = "[::]:443 v4v6"
		}
		var b strings.Builder
		b.WriteString("global\n    maxconn 4096\n\ndefaults\n    mode http\n    timeout connect 5s\n    timeout client 65s\n    timeout server 65s\n    timeout tunnel 1h\n\nfrontend web\n    bind " + bind + " ssl crt " + telemtWebFrontDir + "/tls.pem alpn h2,http/1.1\n")
		fmt.Fprintf(&b, "    http-request deny deny_status 404 unless { hdr(host),field(1,:) -i %s }\n", s.Domain)
		b.WriteString("    http-request set-header X-Forwarded-For %[src]\n    http-request set-header X-Forwarded-Proto https\n")
		if len(sub.Paths) > 0 {
			for _, path := range sub.Paths {
				fmt.Fprintf(&b, "    acl subscription path_beg %s\n", strings.TrimSuffix(path, "*"))
			}
			b.WriteString("    use_backend panel if subscription\n")
		}
		b.WriteString("    default_backend telemt\n\nbackend telemt\n    http-reuse safe\n    server telemt " + backend + "\n")
		if len(sub.Paths) > 0 {
			u, err := url.Parse(sub.Upstream)
			if err != nil || u.Host == "" {
				return nil, errors.New("invalid panel subscription upstream")
			}
			fmt.Fprintf(&b, "\nbackend panel\n    server panel %s", u.Host)
			if u.Scheme == "https" {
				fmt.Fprintf(&b, " ssl verify required ca-file %s verifyhost %s sni str(%s)", strconv.Quote(sub.TLSCAFile), sub.TLSServerName, sub.TLSServerName)
			}
			b.WriteByte('\n')
		}
		return []byte(b.String()), nil
	default:
		return nil, errors.New("alternative WEB frontend must be caddy or haproxy")
	}
}

func writeTelemtWebFrontendConfig(s TelemtWebProxyState) error {
	if telemtWebFrontend(s) == "nginx" {
		return writeTelemtWebNginxConfig(s)
	}
	if !telemtWebCertificateCoversDomain(s.CertFile, s.Domain) {
		return errors.New("WEB certificate does not cover the domain")
	}
	aliases, upstream, err := telemtWebSubscriptionTargets(&SettingService{})
	if err != nil {
		return err
	}
	sub := telemtWebSubscriptionTarget{Upstream: upstream}
	for _, a := range aliases {
		if path, err := normalizeHiddifyLegacySubPath(a.Path); err == nil && path != "" {
			sub.Paths = append(sub.Paths, "/"+path+"/*")
		}
	}
	if strings.HasPrefix(upstream, "https://") {
		sub.TLSCAFile, err = (&SettingService{}).GetCertFile()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(sub.TLSCAFile)
		if err != nil {
			return err
		}
		block, _ := pem.Decode(data)
		if block == nil {
			return errors.New("invalid panel TLS certificate")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return err
		}
		if len(cert.DNSNames) > 0 {
			sub.TLSServerName = cert.DNSNames[0]
			if strings.HasPrefix(sub.TLSServerName, "*.") {
				sub.TLSServerName = "x-ui." + strings.TrimPrefix(sub.TLSServerName, "*.")
			}
		} else if len(cert.IPAddresses) > 0 {
			sub.TLSServerName = cert.IPAddresses[0].String()
		} else {
			return errors.New("panel TLS certificate requires a DNS name or IP SAN")
		}
	}
	data, err := telemtWebFrontendConfig(s, sub)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(telemtWebFrontDir, 0o700); err != nil {
		return err
	}
	if telemtWebFrontend(s) == "haproxy" {
		cert, err := os.ReadFile(s.CertFile)
		if err != nil {
			return err
		}
		key, err := os.ReadFile(s.KeyFile)
		if err != nil {
			return err
		}
		if err := os.WriteFile(telemtWebFrontDir+"/tls.pem", append(append(cert, '\n'), key...), 0o600); err != nil {
			return err
		}
	}
	return os.WriteFile(telemtWebFrontDir+"/config", data, 0o600)
}

func telemtWebFrontUnitContent(front, binary string) string {
	command := strconv.Quote(binary)
	start := command + " run --config " + telemtWebFrontDir + "/config"
	reload := command + " reload --force --address unix//run/3x-ui-telemt-front/caddy.sock --config " + telemtWebFrontDir + "/config"
	if front == "haproxy" {
		start = command + " -W -db -f " + telemtWebFrontDir + "/config"
		reload = "/bin/kill -USR2 $MAINPID"
	}
	return "[Unit]\nDescription=3X-UI Telemt WEB frontend (" + front + ")\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nRuntimeDirectory=3x-ui-telemt-front\nRuntimeDirectoryMode=0700\nExecStart=" + start + "\nExecReload=" + reload + "\nRestart=on-failure\nRestartSec=3\nLimitNOFILE=65536\n\n[Install]\nWantedBy=multi-user.target\n"
}

func telemtWebValidateFrontend(s TelemtWebProxyState) error {
	front := telemtWebFrontend(s)
	args := []string{"validate", "--config", telemtWebFrontDir + "/config"}
	if front == "haproxy" {
		args = []string{"-c", "-f", telemtWebFrontDir + "/config"}
	}
	out, err := exec.CommandContext(context.Background(), front, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s WEB config validation failed: %w: %s", front, err, strings.TrimSpace(string(out)))
	}
	return nil
}
func telemtWebEnsureFrontendRunning(s TelemtWebProxyState) error {
	if telemtWebFrontend(s) == "nginx" {
		return telemtWebEnsureNginxRunning()
	}
	if err := telemtWebValidateFrontend(s); err != nil {
		return err
	}
	binary, err := exec.LookPath(telemtWebFrontend(s))
	if err != nil {
		return err
	}
	unit := telemtWebFrontUnitContent(telemtWebFrontend(s), binary)
	if err := os.WriteFile(filepath.Join("/etc/systemd/system", telemtWebFrontUnit), []byte(unit), 0o644); err != nil {
		return err
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	return systemctl("enable", "--now", telemtWebFrontUnit)
}
func telemtWebReloadFrontend(s TelemtWebProxyState) error {
	if telemtWebFrontend(s) == "nginx" {
		return telemtWebReloadNginx()
	}
	if !telemtWebFrontendActive(s) {
		return nil
	}
	if err := telemtWebValidateFrontend(s); err != nil {
		return err
	}
	return systemctl("reload", telemtWebFrontUnit)
}
func telemtWebStopFrontend(s TelemtWebProxyState, disable bool) error {
	if telemtWebFrontend(s) == "nginx" {
		return telemtWebStopNginx(disable)
	}
	if err := systemctl("stop", telemtWebFrontUnit); err != nil {
		return err
	}
	if disable {
		return systemctl("disable", telemtWebFrontUnit)
	}
	return nil
}
func telemtWebRestoreFrontend(s TelemtWebProxyState) error {
	if telemtWebFrontend(s) == "nginx" {
		return telemtWebRestoreNginx(s)
	}
	return telemtWebStopFrontend(s, true)
}
func removeTelemtWebFrontendConfig(s TelemtWebProxyState) error {
	if telemtWebFrontend(s) == "nginx" {
		return removeTelemtWebNginxConfig()
	}
	for _, name := range []string{"config", "tls.pem"} {
		if err := os.Remove(filepath.Join(telemtWebFrontDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func telemtWebWaitFrontendReady(ctx context.Context, state TelemtWebProxyState) error {
	data, err := os.ReadFile(state.CertFile)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		return errors.New("invalid WEB certificate bundle")
	}
	host := "127.0.0.1"
	if ip, _, err := net.SplitHostPort(state.PublicAddr); err == nil && net.ParseIP(ip) != nil && net.ParseIP(ip).To4() == nil {
		host = "::1"
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: state.Domain, MinVersion: tls.VersionTLS12}}}
	defer client.CloseIdleConnections()
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(deadline, http.MethodHead, "https://"+net.JoinHostPort(host, "443")+"/", nil)
		if err != nil {
			return err
		}
		req.Host = state.Domain
		response, err := client.Do(req)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("WEB HTTPS endpoint returned HTTP %d", response.StatusCode)
		}
		lastErr = err
		select {
		case <-deadline.Done():
			return fmt.Errorf("WEB HTTPS frontend is not ready: %w", lastErr)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
