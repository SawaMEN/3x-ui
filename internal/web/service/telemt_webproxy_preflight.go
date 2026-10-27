package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// TelemtWebPreflight is a read-only deployment plan. The enable endpoint runs
// these same checks again, so direct API callers cannot bypass validation.
type TelemtWebPreflight struct {
	Ready    bool                      `json:"ready"`
	Domain   string                    `json:"domain"`
	Frontend string                    `json:"frontend"`
	Checks   []TelemtWebPreflightCheck `json:"checks"`
	Warnings []string                  `json:"warnings"`
}

type TelemtWebPreflightCheck struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func (s TelemtService) PreflightWebProxy(ctx context.Context, domain string, options TelemtWebProxyOptions, panelCert, panelKey string) (TelemtWebPreflight, error) {
	state, err := readTelemtWebState()
	if err != nil {
		return TelemtWebPreflight{}, err
	}
	options, err = normalizeTelemtWebOptions(options)
	if err != nil {
		return TelemtWebPreflight{}, err
	}
	domain = normalizeTelemtWebDomain(domain)
	result := TelemtWebPreflight{Ready: true, Domain: domain, Frontend: options.Frontend, Checks: []TelemtWebPreflightCheck{}, Warnings: []string{}}
	check := func(id string, ok bool, message string) {
		result.Checks = append(result.Checks, TelemtWebPreflightCheck{ID: id, OK: ok, Message: message})
		if !ok {
			result.Ready = false
		}
	}

	validDomain := len(domain) <= 253 && telemtWebDomainPattern.MatchString(domain)
	if validDomain {
		check("domain", true, "Публичный домен: "+domain)
	} else {
		check("domain", false, "Требуется публичный домен (FQDN), например web.example.com; IP и localhost не подходят.")
	}
	if validDomain {
		if addr, err := resolveTelemtWebPublicAddr(domain); err != nil {
			check("dns", false, err.Error())
		} else {
			check("dns", true, "DNS разрешается в публичный адрес "+addr)
			// A remote firewall, DNS proxy or NAT port forwarding cannot be
			// established by listening on local sockets alone.
			result.Warnings = append(result.Warnings, "Проверьте, что DNS указывает непосредственно на этот сервер (без CDN), TCP 443 доступен извне, а при NAT настроена переадресация.")
		}
	}
	version := telemtVersion()
	engineReady := s.IsInstalled() && telemtWebVersionAtLeast(version, telemtWebMinEngine)
	if engineReady {
		check("engine", true, "Telemt установлен, версия "+version+" поддерживает WEB Proxy.")
	} else {
		check("engine", false, fmt.Sprintf("Telemt WEB требует установленного Telemt %s+ (текущая версия: %s).", telemtWebMinEngine, version))
	}
	if (len(options.Carriers) > 0 || options.Carrier != "https") && !telemtWebVersionAtLeast(version, "3.5.4") {
		check("carriers", false, "Выбранные WEB-транспорты требуют Telemt 3.5.4+.")
	}
	if options.Carrier != "https" {
		result.Warnings = append(result.Warnings, "Для совместимости с Telegram iOS выберите HTTPS как основной транспорт. WebSocket можно оставить в списке автоматического выбора для поддерживающих его клиентов.")
	}
	if os.Geteuid() == 0 {
		check("privileges", true, "Права root доступны.")
	} else {
		check("privileges", false, "Для автоматической настройки нужны права root.")
	}

	nativePort, configErr := telemtWebNativePort()
	if configErr != nil {
		check("config", false, "Не удаётся прочитать конфигурацию Telemt: "+configErr.Error())
	} else {
		// Conflicts on 443 are reported once by the shared https-port check.
		// This includes the native Telemt listener, even if Telemt is stopped.
		if !state.Enabled {
			backendPort, portErr := pickTelemtWebListenPort(nativePort)
			if portErr != nil {
				check("backend", false, "Нужен свободный порт локального Telemt в диапазоне 15080–15179.")
			} else {
				check("backend", true, fmt.Sprintf("Свободен порт локального Telemt %d.", backendPort))
			}
		}
	}

	// One authoritative check reports both system processes and saved panel
	// listeners, including the admin UI and enabled inbound configurations.
	port443, inspectionErr := inspectTelemtWebPort443(state, options.Frontend)
	if inspectionErr != nil {
		check("https-port", false, "Не удалось проверить TCP 443: "+inspectionErr.Error())
	} else if port443.Available {
		check("https-port", true, "TCP 443 доступен для выбранного HTTPS frontend.")
	} else if port443.Owner != "" {
		if port443.Free {
			check("https-port", false, "TCP 443 зарезервирован настройками панели: "+port443.Owner+".")
		} else {
			check("https-port", false, "TCP 443 занят: "+port443.Owner+".")
		}
	} else {
		check("https-port", false, "TCP 443 занят другим сервисом.")
	}
	if options.Frontend == "nginx" && telemtWebCommandExists("nginx") {
		conflict, inspectErr := telemtWebNginxHostConflict(ctx, domain)
		if inspectErr == nil && !conflict {
			check("nginx-vhost", true, "Нет конкурирующего HTTPS vhost Nginx для домена.")
		} else {
			check("nginx-vhost", false, "Nginx: домен уже обслуживается другим HTTPS vhost либо нельзя проверить конфигурацию.")
		}
	}
	if options.Frontend != "nginx" {
		if telemtWebCommandExists("systemctl") {
			check("systemd", true, "Systemd доступен для управления HTTPS-сервером WEB Proxy.")
		} else {
			check("systemd", false, "Caddy и HAProxy требуют systemd.")
		}
	}
	if !telemtWebCommandExists(options.Frontend) {
		if manager := telemtWebPackageManager(); manager != "" {
			check("package-manager", true, "Автоматическая установка через "+manager+".")
		} else {
			check("package-manager", false, "Для автоматической установки HTTPS-сервера WEB Proxy необходим поддерживаемый менеджер пакетов.")
		}
	}
	if validDomain {
		acmeCert := "/etc/letsencrypt/live/" + domain + "/fullchain.pem"
		acmeKey := "/etc/letsencrypt/live/" + domain + "/privkey.pem"
		certReady := telemtWebCertificatePairReady(panelCert, panelKey, domain) ||
			telemtWebCertificatePairReady(acmeCert, acmeKey, domain)
		if state.Enabled && state.Domain == domain {
			certReady = certReady || telemtWebCertificatePairReady(state.CertFile, state.KeyFile, domain)
		}
		if certReady {
			check("tls", true, "Подходящий действующий TLS-сертификат будет использован повторно.")
		} else {
			port80Ready := telemtWebPortAvailable(80) || (options.Frontend == "nginx" && telemtWebNginxOwnsPort(80))
			if port80Ready {
				check("acme-port", true, "Порт TCP 80 подходит для автоматического получения сертификата.")
			} else {
				check("acme-port", false, "Для автоматического выпуска TLS-сертификата HTTP-01 требуется свободный TCP 80 или Nginx на TCP 80.")
			}
			if options.Frontend == "nginx" && telemtWebNginxOwnsPort(80) {
				conflict, inspectErr := telemtWebNginxPortHostConflict(ctx, domain, 80)
				if inspectErr == nil && !conflict {
					check("nginx-acme-vhost", true, "HTTP-01 не конфликтует с существующими Nginx vhost.")
				} else {
					check("nginx-acme-vhost", false, "Уже есть Nginx HTTP vhost на этом домене: ACME HTTP-01 будет перехвачен. Подключите готовый сертификат или перенастройте HTTP vhost.")
				}
			}
			if !telemtWebCommandExists("certbot") {
				if manager := telemtWebPackageManager(); manager != "" {
					check("certbot", true, "Certbot будет установлен автоматически через "+manager+".")
				} else {
					check("certbot", false, "Для автоматической установки Certbot необходим поддерживаемый менеджер пакетов.")
				}
			}
			result.Warnings = append(result.Warnings, "Выпуск сертификата HTTP-01 требует внешнего доступа к TCP 80; локальная проверка не подтверждает правила облачного firewall.")
		}
	}
	return result, nil
}

// Do not call GetConfig here: it creates or upgrades the real Telemt config
// on disk. A preflight must not change configuration merely by being opened.
func telemtWebNativePort() (int, error) {
	data, err := os.ReadFile(telemtConfigPath)
	if os.IsNotExist(err) {
		return defaultTelemtConfig().Port, nil
	}
	if err != nil {
		return 0, err
	}
	var config struct {
		Server struct {
			Port int `toml:"port"`
		} `toml:"server"`
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		return 0, err
	}
	if config.Server.Port < 1 || config.Server.Port > 65535 {
		return 0, fmt.Errorf("invalid Telemt native port: %d", config.Server.Port)
	}
	return config.Server.Port, nil
}

// ss can report several owners for separate IPv4/IPv6 listeners. Any
// non-frontend owner makes a wildcard bind unsafe.
func telemtWebPortOwnedOnlyBy(owners, frontend string) bool {
	if owners == "" {
		return false
	}
	for _, owner := range strings.Split(owners, ",") {
		if strings.TrimSpace(owner) != frontend {
			return false
		}
	}
	return true
}

// Verify the private key actually belongs to the certificate, in addition
// to checking hostname and validity. A readable but mismatched key is not TLS.
func telemtWebCertificatePairReady(certFile, keyFile, domain string) bool {
	if certFile == "" || keyFile == "" || domain == "" {
		return false
	}
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil || len(pair.Certificate) == 0 {
		return false
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return false
	}
	now := time.Now()
	return !now.Before(cert.NotBefore) && now.Before(cert.NotAfter) && cert.VerifyHostname(domain) == nil
}

// Parse nginx -T by server blocks, rather than simply searching for the
// hostname (which would incorrectly reject HTTP-only ACME virtual hosts).
func telemtWebNginxHostConflict(ctx context.Context, domain string) (bool, error) {
	return telemtWebNginxPortHostConflict(ctx, domain, 443)
}

func telemtWebNginxPortHostConflict(ctx context.Context, domain string, port int) (bool, error) {
	if domain == "" {
		return false, nil
	}
	timeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(timeout, "nginx", "-T").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("nginx -T failed: %w", err)
	}
	return telemtWebNginxConfigHasHostPort(string(output), domain, port), nil
}

var telemtWebServerStart = regexp.MustCompile(`\bserver\s*\{`)

func telemtWebNginxConfigHasHost(config, domain string) bool {
	return telemtWebNginxConfigHasHostPort(config, domain, 443)
}

func telemtWebNginxConfigHasHostPort(config, domain string, port int) bool {
	currentFile := ""
	var block strings.Builder
	inServer, depth, skip := false, 0, false
	for _, raw := range strings.Split(config, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "# configuration file ") {
			currentFile = strings.TrimSuffix(strings.TrimPrefix(line, "# configuration file "), ":")
			continue
		}
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if !inServer {
			if !telemtWebServerStart.MatchString(line) {
				continue
			}
			inServer, skip = true, currentFile == telemtWebNginxConf
			block.Reset()
			line = line[telemtWebServerStart.FindStringIndex(line)[0]:]
		}
		block.WriteString(line)
		block.WriteByte('\n')
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if depth != 0 {
			continue
		}
		if !skip && telemtWebNginxServerMatchesPort(block.String(), domain, port) {
			return true
		}
		inServer = false
	}
	return false
}

var (
	telemtWebNginxListen = regexp.MustCompile(`(?m)^\s*listen\s+([^;]+);`)
	telemtWebNginxNames  = regexp.MustCompile(`(?m)^\s*server_name\s+([^;]+);`)
)

func telemtWebNginxServerMatchesPort(block, domain string, port int) bool {
	hasPort := false
	for _, match := range telemtWebNginxListen.FindAllStringSubmatch(block, -1) {
		for _, field := range strings.Fields(match[1]) {
			portStr := fmt.Sprint(port)
			if field == portStr || field == ":"+portStr || strings.HasSuffix(field, "]:"+portStr) || strings.HasSuffix(field, ":"+portStr) {
				hasPort = true
			}
		}
	}
	if !hasPort {
		return false
	}
	for _, match := range telemtWebNginxNames.FindAllStringSubmatch(block, -1) {
		for _, name := range strings.Fields(match[1]) {
			if strings.EqualFold(name, domain) || (strings.HasPrefix(name, "*.") && strings.HasSuffix(domain, name[1:])) {
				return true
			}
		}
	}
	return false
}
