package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
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

const telemtWebDomainPatternSource = "^(?:[A-Za-z0-9]" +
	"(?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?" +
	"\\.)+[A-Za-z]{2,63}$"

var (
	telemtWebDomainPattern    = regexp.MustCompile(telemtWebDomainPatternSource)
	telemtWebVersionPattern   = regexp.MustCompile(`(?i)v?([0-9]+)\.([0-9]+)\.([0-9]+)`)
	telemtWebPortOwnerPattern = regexp.MustCompile(`\("([^"]+)",pid=\d+`)
)

type TelemtWebProxyState struct {
	Site               *TelemtWebSiteOptions `json:"site,omitempty"`
	ManagedInboundID   int                   `json:"managedInboundId,omitempty"`
	BackendPort        int                   `json:"backendPort,omitempty"`
	UserFingerprint    string                `json:"userFingerprint,omitempty"`
	Frontend           string                `json:"frontend,omitempty"`
	Carrier            string                `json:"carrier,omitempty"`
	Carriers           []string              `json:"carriers,omitempty"`
	Enabled            bool                  `json:"enabled"`
	Domain             string                `json:"domain"`
	BasePath           string                `json:"basePath,omitempty"`
	Aliases            []string              `json:"aliases,omitempty"`
	Secret             string                `json:"secret"`
	DecoyDir           string                `json:"decoyDir"`
	ListenPort         int                   `json:"listenPort"`
	CertFile           string                `json:"certFile"`
	KeyFile            string                `json:"keyFile"`
	PublicAddr         string                `json:"publicAddr,omitempty"`
	NginxManaged       bool                  `json:"nginxManaged,omitempty"`
	NginxWasEnabled    bool                  `json:"nginxWasEnabled,omitempty"`
	NginxStateCaptured bool                  `json:"nginxStateCaptured,omitempty"`
}

type TelemtWebProxyStatus struct {
	Site                        *TelemtWebSiteOptions `json:"site,omitempty"`
	Frontend                    string                `json:"frontend"`
	FrontendActive              bool                  `json:"frontendActive"`
	Carrier                     string                `json:"carrier"`
	Carriers                    []string              `json:"carriers"`
	CarrierNegotiationSupported bool                  `json:"carrierNegotiationSupported"`
	Enabled                     bool                  `json:"enabled"`
	Supported                   bool                  `json:"supported"`
	Domain                      string                `json:"domain"`
	BasePath                    string                `json:"basePath"`
	Aliases                     []string              `json:"aliases,omitempty"`
	DefaultDomain               string                `json:"defaultDomain"`
	Link                        string                `json:"link"`
	Links                       []string              `json:"links,omitempty"`
	NginxInstalled              bool                  `json:"nginxInstalled"`
	NginxActive                 bool                  `json:"nginxActive"`
	CertificateReady            bool                  `json:"certificateReady"`
	CertificateFile             string                `json:"certificateFile"`
	ListenPort                  int                   `json:"listenPort"`
	Port443Free                 bool                  `json:"port443Free"`
	Port443Available            bool                  `json:"port443Available"`
	Port443Owner                string                `json:"port443Owner"`
	Error                       string                `json:"error"`
}

func readTelemtWebState() (TelemtWebProxyState, error) {
	data, err := os.ReadFile(telemtWebStatePath)
	if errors.Is(err, os.ErrNotExist) {
		return TelemtWebProxyState{DecoyDir: telemtWebDecoyDir, ListenPort: telemtWebListenPort}, nil
	}
	if err != nil {
		return TelemtWebProxyState{}, err
	}
	var state TelemtWebProxyState
	if err := json.Unmarshal(data, &state); err != nil {
		return TelemtWebProxyState{}, fmt.Errorf("telemt web proxy: parse state: %w", err)
	}
	if state.DecoyDir == "" {
		state.DecoyDir = telemtWebDecoyDir
	}
	if state.ListenPort == 0 {
		state.ListenPort = telemtWebListenPort
	}
	return state, nil
}

func writeTelemtWebState(state TelemtWebProxyState) error {
	if err := os.MkdirAll(filepath.Dir(telemtWebStatePath), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := telemtWebStatePath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, telemtWebStatePath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func pickTelemtWebListenPort(primaryPort int) (int, error) {
	for port := telemtWebListenPort; port < telemtWebListenPort+100; port++ {
		if port == primaryPort {
			continue
		}
		listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", net.JoinHostPort(telemtWebListenIP, fmt.Sprint(port)))
		if err == nil {
			_ = listener.Close()
			return port, nil
		}
	}
	return 0, errors.New("telemt: no free loopback port for WEB Proxy backend (15080-15179)")
}

func clearTelemtWebState() error {
	err := os.Remove(telemtWebStatePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func appendTelemtWebProxyConfig(base string, state TelemtWebProxyState) (string, error) {
	base = removeTelemtWebUser(base)
	if !state.Enabled {
		return base, nil
	}
	if !telemtWebDomainPattern.MatchString(state.Domain) || len(state.Domain) > 253 {
		return "", errors.New("telemt web proxy: invalid domain")
	}
	if !regexp.MustCompile("^[0-9a-fA-F]{32}$").MatchString(state.Secret) {
		return "", errors.New("telemt web proxy: invalid secret")
	}
	publicAddr := strings.TrimSpace(state.PublicAddr)
	if publicAddr == "" {
		var err error
		publicAddr, err = resolveTelemtWebPublicAddr(state.Domain)
		if err != nil {
			return "", err
		}
	}
	managedUsers := ""
	managedProfiles := ""
	managedUserCount := 0
	var managedQuotas, managedExpirations, managedIPLimits strings.Builder
	if state.ManagedInboundID != 0 {
		var inbound model.Inbound
		if err := database.GetDB().First(&inbound, state.ManagedInboundID).Error; err != nil {
			return "", fmt.Errorf("telemt web proxy: load managed inbound: %w", err)
		}
		effective, err := (&InboundService{}).buildInboundForLocalRuntime(database.GetDB(), &inbound)
		if err != nil {
			return "", fmt.Errorf("telemt web proxy: resolve active clients: %w", err)
		}
		inst, ok := mtproto.InstanceFromInbound(effective)
		if !ok {
			return "", errors.New("telemt web proxy: managed inbound has no enabled clients")
		}
		managedUserCount = len(inst.Secrets)
		for _, user := range inst.Secrets {
			// Stable identities retain per-user counters across list reordering.
			name := mtprotoWebUsername(state.ManagedInboundID, user.Name)
			managedUsers += fmt.Sprintf("%q = %q\n", name, user.Secret)
			managedProfiles += fmt.Sprintf("\n[[web.vhosts.profiles]]\nuser = %q\nsecret_mode = \"dd\"\n", name)
			if user.QuotaBytes > 0 {
				fmt.Fprintf(&managedQuotas, "%q = %d\n", name, user.QuotaBytes)
			}
			if user.ExpiresUnix > 0 {
				fmt.Fprintf(&managedExpirations, "%q = %q\n", name, time.Unix(user.ExpiresUnix, 0).UTC().Format(time.RFC3339))
			}
			if user.LimitIP > 0 {
				fmt.Fprintf(&managedIPLimits, "%q = %d\n", name, user.LimitIP)
			}
		}
	}
	section := "[access.users]"
	idx := strings.Index(base, section)
	if idx < 0 {
		return "", errors.New("telemt web proxy: access.users section is missing")
	}
	insertAt := len(base)
	if next := strings.Index(base[idx+len(section):], "\n["); next >= 0 {
		insertAt = idx + len(section) + next + 1
	}
	if state.ManagedInboundID != 0 {
		base = base[:insertAt] + managedUsers + base[insertAt:]
	} else {
		base = base[:insertAt] + fmt.Sprintf("%s = \"%s\"\n", telemtWebUser, state.Secret) + base[insertAt:]
	}
	decoyDir := state.DecoyDir
	if decoyDir == "" {
		decoyDir = telemtWebDecoyDir
	}
	listenPort := state.ListenPort
	if listenPort == 0 {
		listenPort = telemtWebListenPort
	}
	webConfig := fmt.Sprintf("\n[[server.listeners]]\nip = \"%s\"\nport = %d\ntransport = \"web\"\nproxy_protocol = false\nreuse_allow = false\nweb_client_ip_source = \"x_forwarded_for\"\nweb_trusted_proxy_cidrs = [\"127.0.0.1/32\"]\n\n[web]\nenabled = true\ncarrier = \"https\"\n\n[[web.vhosts]]\nhost = \"%s\"\npublic_addr = \"%s\"\n\n[web.vhosts.decoy]\nmode = \"static_directory\"\ndirectory = \"%s\"\nindex = \"index.html\"\n\n[[web.vhosts.profiles]]\nuser = \"%s\"\nsecret_mode = \"dd\"\n", telemtWebListenIP, listenPort, state.Domain, publicAddr, decoyDir, telemtWebUser)
	options, err := normalizeTelemtWebOptions(TelemtWebProxyOptions{Frontend: state.Frontend, Carrier: state.Carrier, Carriers: state.Carriers, BasePath: &state.BasePath, Aliases: &state.Aliases})
	if err != nil {
		return "", err
	}
	if state.BasePath != "" {
		webConfig = strings.Replace(webConfig, "public_addr = "+strconv.Quote(publicAddr), "base_path = "+strconv.Quote(state.BasePath)+"\npublic_addr = "+strconv.Quote(publicAddr), 1)
	}
	if err := validateTelemtWebSite(state.Site); err != nil {
		return "", err
	}
	if state.Site != nil && state.Site.Mode == "origin" {
		staticDecoy := fmt.Sprintf("[web.vhosts.decoy]\nmode = \"static_directory\"\ndirectory = %q\nindex = \"index.html\"", decoyDir)
		originDecoy := fmt.Sprintf("[web.vhosts.decoy]\nmode = \"http_upstream\"\nupstream = %q", telemtWebCanonicalOrigin(state.Site.Origin))
		webConfig = strings.Replace(webConfig, staticDecoy, originDecoy, 1)
	}
	webConfig = strings.Replace(webConfig, "carrier = \"https\"", "carrier = "+strconv.Quote(options.Carrier), 1)
	if len(options.Carriers) > 0 {
		values := make([]string, 0, len(options.Carriers))
		for _, carrier := range options.Carriers {
			values = append(values, strconv.Quote(carrier))
		}
		webConfig = strings.Replace(webConfig, "[web]\nenabled = true\n", "[web]\nenabled = true\ncarriers = ["+strings.Join(values, ", ")+"]\n", 1)
	}
	if state.ManagedInboundID != 0 {
		webConfig = strings.Replace(webConfig, "\n[[web.vhosts.profiles]]\nuser = \"webproxy\"\nsecret_mode = \"dd\"\n", managedProfiles, 1)
	}
	// Explicit WEB admission bounds with headroom for reconfiguration. These
	// limits apply to the WEB listener only, not ordinary MTProto sidecars.
	maxProfiles := telemtWebProfileCapacity(managedUserCount, len(state.Aliases)+1)
	webConfig = strings.Replace(webConfig, "[web]\nenabled = true\n", fmt.Sprintf("[web]\nenabled = true\nlimits.max_profiles = %d\nlimits.max_vhosts = %d\n", maxProfiles, len(state.Aliases)+1), 1)
	// Extra hostnames share the same loopback WEB listener and per-user
	// profiles. Each vhost advertises its own resolved public endpoint.
	for _, alias := range state.Aliases {
		aliasAddr, resolveErr := resolveTelemtWebPublicAddr(alias)
		if resolveErr != nil {
			return "", fmt.Errorf("WEB alias %s: %w", alias, resolveErr)
		}
		webConfig += fmt.Sprintf("\n[[web.vhosts]]\nhost = %q\n", alias)
		if state.BasePath != "" {
			webConfig += fmt.Sprintf("base_path = %q\n", state.BasePath)
		}
		webConfig += fmt.Sprintf("public_addr = %q\n", aliasAddr)
		if state.Site != nil && state.Site.Mode == "origin" {
			webConfig += fmt.Sprintf("\n[web.vhosts.decoy]\nmode = \"http_upstream\"\nupstream = %q\n", telemtWebCanonicalOrigin(state.Site.Origin))
		} else {
			webConfig += fmt.Sprintf("\n[web.vhosts.decoy]\nmode = \"static_directory\"\ndirectory = %q\nindex = \"index.html\"\n", decoyDir)
		}
		if state.ManagedInboundID != 0 {
			webConfig += managedProfiles
		} else {
			webConfig += fmt.Sprintf("\n[[web.vhosts.profiles]]\nuser = %q\nsecret_mode = \"dd\"\n", telemtWebUser)
		}
	}
	if state.ManagedInboundID != 0 {
		if managedQuotas.Len() > 0 {
			base += "\n[access.user_data_quota]\n" + managedQuotas.String()
		}
		if managedExpirations.Len() > 0 {
			base += "\n[access.user_expirations]\n" + managedExpirations.String()
		}
		if managedIPLimits.Len() > 0 {
			base += "\n[access.user_max_unique_ips]\n" + managedIPLimits.String()
		}
	}
	return strings.TrimRight(base, "\n") + "\n" + webConfig, nil
}

func removeTelemtWebUser(base string) string {
	lines := strings.Split(base, "\n")
	out := make([]string, 0, len(lines))
	inUsers := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inUsers = trimmed == "[access.users]"
		}
		if inUsers && (strings.HasPrefix(trimmed, telemtWebUser+" = ") || strings.HasPrefix(trimmed, "\"mtweb_")) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func (TelemtService) WebProxyStatus(defaultDomain, panelCert, panelKey string) (TelemtWebProxyStatus, error) {
	state, err := readTelemtWebState()
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	version := strings.TrimPrefix(telemtVersion(), "v")
	nginxActive := telemtWebNginxActive()
	frontend := telemtWebFrontend(state)
	nginxOwns443 := telemtWebNginxOwnsPort443()
	frontendActive := state.Enabled && telemtWebFrontendActive(state) && (frontend != "nginx" || nginxOwns443)
	port443, inspectionErr := inspectTelemtWebPort443(state, frontend)
	status := TelemtWebProxyStatus{Aliases: state.Aliases, Site: state.Site, Frontend: frontend, FrontendActive: frontendActive, Carrier: telemtWebCarrier(state), Carriers: state.Carriers, CarrierNegotiationSupported: telemtWebVersionAtLeast(version, "3.5.4"), Enabled: state.Enabled, Supported: telemtWebVersionAtLeast(version, telemtWebMinEngine), Domain: state.Domain, BasePath: state.BasePath, DefaultDomain: defaultDomain, NginxInstalled: telemtWebCommandExists("nginx"), NginxActive: nginxActive, Port443Free: port443.Free, Port443Available: port443.Available, Port443Owner: port443.Owner, ListenPort: state.ListenPort, CertificateFile: state.CertFile}
	if state.Enabled && state.ManagedInboundID == 0 && state.Domain != "" && state.Secret != "" {
		for _, host := range telemtWebHosts(state) {
			status.Links = append(status.Links, telemtWebLinkWithPath(host, state.Secret, state.BasePath))
		}
		status.Link = status.Links[0]
	}
	if state.CertFile != "" {
		status.CertificateReady = telemtWebCertificatePairReady(state.CertFile, state.KeyFile, state.Domain)
	} else if panelCert != "" {
		status.CertificateReady = telemtWebCertificatePairReady(panelCert, panelKey, state.Domain)
		status.CertificateFile = panelCert
	}
	if !status.Supported {
		status.Error = fmt.Sprintf("Telemt %s does not support WEB Proxy; requires %s or newer", version, telemtWebMinEngine)
	}
	if inspectionErr != nil {
		status.Error = "Не удалось проверить TCP 443: " + inspectionErr.Error()
	} else if !port443.Available {
		if port443.Owner != "" {
			if port443.Free {
				status.Error = fmt.Sprintf("TCP 443 зарезервирован настройками панели: %s.", port443.Owner)
			} else {
				status.Error = fmt.Sprintf("TCP 443 занят: %s.", port443.Owner)
			}
		} else {
			status.Error = "TCP 443 занят другим сервисом."
		}
	}
	return status, nil
}

func enableTelemtWebProxy(ctx context.Context, domain, panelCert, panelKey string, options TelemtWebProxyOptions) (TelemtWebProxyStatus, error) {
	domain = normalizeTelemtWebDomain(domain)
	if !telemtWebDomainPattern.MatchString(domain) || len(domain) > 253 {
		return TelemtWebProxyStatus{}, errors.New("WEB Proxy requires a public FQDN, for example web.example.com")
	}
	if os.Geteuid() != 0 {
		return TelemtWebProxyStatus{}, errors.New("WEB Proxy setup requires root privileges")
	}
	version := strings.TrimPrefix(telemtVersion(), "v")
	if !telemtWebVersionAtLeast(version, telemtWebMinEngine) {
		return TelemtWebProxyStatus{}, fmt.Errorf("Telemt %s does not support WEB Proxy; requires %s or newer", version, telemtWebMinEngine)
	}
	state, err := readTelemtWebState()
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	port443, err := inspectTelemtWebPort443(state, options.Frontend)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	if !port443.Available {
		if port443.Owner != "" {
			if port443.Free {
				return TelemtWebProxyStatus{}, fmt.Errorf("TCP 443 зарезервирован настройками панели: %s", port443.Owner)
			}
			return TelemtWebProxyStatus{}, fmt.Errorf("TCP 443 занят: %s", port443.Owner)
		}
		return TelemtWebProxyStatus{}, errors.New("TCP 443 занят другим сервисом")
	}
	oldState := state
	// Validate and select a certificate for the *new* primary hostname, not
	// the saved hostname of a previous WEB installation.
	state.Domain = domain
	state.Frontend, state.Carrier, state.Carriers = options.Frontend, options.Carrier, options.Carriers
	if options.BasePath != nil {
		state.BasePath = *options.BasePath
	}
	if options.Aliases != nil {
		state.Aliases = *options.Aliases
	}
	nginxWasInstalled := telemtWebCommandExists(options.Frontend)
	nginxWasActive := telemtWebFrontendActive(state)
	nginxManaged := oldState.NginxManaged
	nginxWasEnabled := oldState.NginxWasEnabled
	nginxStateCaptured := oldState.NginxStateCaptured
	if !oldState.Enabled {
		nginxManaged = !nginxWasInstalled || !nginxWasActive
		nginxWasEnabled = nginxWasInstalled && telemtWebFrontendEnabled(state)
		nginxStateCaptured = true
	}
	if err := telemtWebEnsureFrontendPackage(ctx, state); err != nil {
		return TelemtWebProxyStatus{}, err
	}
	decoyDir, err := prepareTelemtWebSite(domain, telemtWebDecoyDir, telemtWebDecoyDir+"-sites", options.Site)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	publicAddr, err := resolveTelemtWebPublicAddr(domain)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	certFile, keyFile, err := telemtWebFrontendCertificate(ctx, state, domain, panelCert, panelKey)
	if err != nil {
		if nginxManaged && !oldState.Enabled {
			_ = telemtWebStopFrontend(state, !nginxWasEnabled)
		}
		return TelemtWebProxyStatus{}, err
	}
	if err := telemtWebPrepareCertificateRenewal(ctx, state, domain, certFile); err != nil {
		if nginxManaged && !oldState.Enabled {
			_ = telemtWebStopFrontend(state, !nginxWasEnabled)
		}
		return TelemtWebProxyStatus{}, err
	}
	rollbackNginx := func() {
		if oldState.Enabled {
			return
		}
		_ = removeTelemtWebFrontendConfig(state)
		if nginxManaged {
			_ = telemtWebStopFrontend(state, !nginxWasEnabled)
		} else if telemtWebFrontendActive(state) {
			_ = telemtWebReloadFrontend(state)
		}
	}
	if !telemtWebCertificateCoversHosts(certFile, keyFile, telemtWebHosts(state)) {
		return TelemtWebProxyStatus{}, errors.New("Все WEB-домены должны покрываться одним действующим TLS-сертификатом (например, wildcard).")
	}
	state.CertFile, state.KeyFile = certFile, keyFile
	if err := installTelemtWebRenewalHook(state); err != nil {
		rollbackNginx()
		return TelemtWebProxyStatus{}, err
	}
	if state.Secret == "" {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			rollbackNginx()
			return TelemtWebProxyStatus{}, err
		}
		state.Secret = hex.EncodeToString(buf)
	}
	cfg, err := (TelemtService{}).GetConfig()
	if err != nil {
		rollbackNginx()
		return TelemtWebProxyStatus{}, err
	}
	if !oldState.Enabled {
		state.ListenPort, err = pickTelemtWebListenPort(cfg.Port)
		if err != nil {
			rollbackNginx()
			return TelemtWebProxyStatus{}, err
		}
	}
	state.Enabled = true
	state.Domain = domain
	state.DecoyDir = decoyDir
	state.Site = options.Site
	state.CertFile = certFile
	state.KeyFile = keyFile
	state.PublicAddr = publicAddr
	state.NginxManaged = nginxManaged
	state.NginxWasEnabled = nginxWasEnabled
	state.NginxStateCaptured = nginxStateCaptured
	if err := writeTelemtWebState(state); err != nil {
		rollbackNginx()
		return TelemtWebProxyStatus{}, err
	}
	if err := (TelemtService{}).SaveConfig(cfg); err != nil {
		_ = writeTelemtWebState(oldState)
		rollbackNginx()
		return TelemtWebProxyStatus{}, err
	}
	if err := writeTelemtWebFrontendConfig(state); err != nil {
		_ = writeTelemtWebState(oldState)
		_ = (TelemtService{}).SaveConfig(cfg)
		rollbackNginx()
		return TelemtWebProxyStatus{}, err
	}
	if err := telemtWebEnsureFrontendRunning(state); err != nil {
		_ = removeTelemtWebFrontendConfig(state)
		if nginxManaged {
			_ = telemtWebStopFrontend(state, !nginxWasEnabled)
		} else {
			_ = telemtWebReloadFrontend(state)
		}
		_ = writeTelemtWebState(oldState)
		_ = (TelemtService{}).SaveConfig(cfg)
		return TelemtWebProxyStatus{}, err
	}
	if err := telemtWebReloadFrontend(state); err != nil {
		_ = removeTelemtWebFrontendConfig(state)
		if nginxManaged {
			_ = telemtWebStopFrontend(state, !nginxWasEnabled)
		} else {
			_ = telemtWebReloadFrontend(state)
		}
		_ = writeTelemtWebState(oldState)
		_ = (TelemtService{}).SaveConfig(cfg)
		return TelemtWebProxyStatus{}, err
	}
	return (TelemtService{}).WebProxyStatus(domain, panelCert, panelKey)
}

func disableTelemtWebProxy() error {
	state, err := readTelemtWebState()
	if err != nil {
		return err
	}
	if !state.Enabled {
		removeTelemtWebAcmeConfig()
		_ = removeTelemtWebFrontendConfig(state)
		if state.NginxManaged {
			_ = telemtWebRestoreFrontend(state)
		} else if telemtWebFrontendActive(state) {
			_ = telemtWebReloadFrontend(state)
		}
		return clearTelemtWebState()
	}
	oldState := state
	state.Enabled = false
	if err := writeTelemtWebState(state); err != nil {
		return err
	}
	cfg, err := (TelemtService{}).GetConfig()
	if err != nil {
		_ = writeTelemtWebState(oldState)
		return err
	}
	if err := (TelemtService{}).SaveConfig(cfg); err != nil {
		_ = writeTelemtWebState(oldState)
		return err
	}
	removeTelemtWebAcmeConfig()
	if err := removeTelemtWebFrontendConfig(state); err != nil {
		_ = writeTelemtWebState(oldState)
		_ = (TelemtService{}).SaveConfig(cfg)
		return err
	}
	if oldState.NginxManaged {
		if err := telemtWebRestoreFrontend(oldState); err != nil {
			_ = writeTelemtWebState(oldState)
			_ = (TelemtService{}).SaveConfig(cfg)
			_ = writeTelemtWebFrontendConfig(oldState)
			_ = telemtWebEnsureFrontendRunning(state)
			_ = telemtWebReloadFrontend(state)
			return err
		}
	} else if telemtWebFrontendActive(state) {
		if err := telemtWebReloadFrontend(state); err != nil {
			_ = writeTelemtWebState(oldState)
			_ = (TelemtService{}).SaveConfig(cfg)
			_ = writeTelemtWebFrontendConfig(oldState)
			_ = telemtWebReloadFrontend(state)
			return err
		}
	}
	return clearTelemtWebState()
}

func normalizeTelemtWebDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "http://")
	return strings.TrimRight(domain, "/")
}

func normalizeTelemtWebBasePath(value string) (string, error) {
	path := strings.Trim(strings.TrimSpace(value), "/")
	if path == "" {
		return "", nil
	}
	if len(path) < 6 || len(path) > 96 || !regexp.MustCompile(`^[A-Za-z0-9_-]+(?:/[A-Za-z0-9_-]+)*$`).MatchString(path) {
		return "", errors.New("WEB Proxy base path must contain 6-96 URL-safe characters (letters, digits, underscores, dashes or slashes)")
	}
	return path, nil
}

// A WEB path link uses the official 0x70 0xdd prefix and base64url encoding.
// Links without paths preserve the historic dd+hex form byte for byte.
func telemtWebLinkWithPath(domain, secret, basePath string) string {
	if basePath == "" {
		return fmt.Sprintf("tg://webproxy?server=%s&secret=dd%s", domain, secret)
	}
	decoded, err := hex.DecodeString(secret)
	if err != nil || len(decoded) != 16 {
		return ""
	}
	buf := make([]byte, 0, 18)
	buf = append(buf, 0x70, 0xdd)
	buf = append(buf, decoded...)
	return "tg://webproxy?server=" + url.QueryEscape(domain+"/"+basePath) +
		"&secret=" + base64.RawURLEncoding.EncodeToString(buf)
}

func resolveTelemtWebPublicAddr(domain string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return "", fmt.Errorf("WEB Proxy domain %s cannot be resolved: %w", domain, err)
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		ips = append(ips, address.IP)
	}
	isPublic := func(ip net.IP) bool {
		return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
	}
	// Prefer an address that actually belongs to this server. A hostname can
	// have several A/AAAA records, but Telemt binds public_addr to the inner
	// relay destination; choosing an unrelated record breaks WEB sessions.
	interfaces, err := net.InterfaceAddrs()
	if err == nil {
		local := make([]net.IP, 0, len(interfaces))
		for _, address := range interfaces {
			if network, ok := address.(*net.IPNet); ok && isPublic(network.IP) {
				local = append(local, network.IP)
			}
		}
		if ip := selectTelemtWebPublicIP(ips, local); ip != nil {
			return net.JoinHostPort(ip.String(), "443"), nil
		}
		// Reject addresses pointing elsewhere on the same public IP family.
		// A host can legitimately have public IPv6 but NAT-forwarded IPv4;
		// that mixed-family case must retain the NAT fallback below.
		for _, own := range local {
			for _, candidate := range ips {
				if isPublic(candidate) && (own.To4() != nil) == (candidate.To4() != nil) {
					return "", fmt.Errorf("WEB Proxy domain %s points to a different public server: none of its A/AAAA addresses match local public interfaces", domain)
				}
			}
		}
	}
	// Public IPs behind NAT are not assigned to a local interface. Preserve
	// the DNS fallback for those installations.
	if ip := selectTelemtWebPublicIP(ips, nil); ip != nil {
		return net.JoinHostPort(ip.String(), "443"), nil
	}
	return "", fmt.Errorf("WEB Proxy domain %s does not resolve to a public IP address", domain)
}

func selectTelemtWebPublicIP(dns, local []net.IP) net.IP {
	isPublic := func(ip net.IP) bool {
		return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
	}
	for _, ip := range dns {
		if !isPublic(ip) {
			continue
		}
		for _, own := range local {
			if ip.Equal(own) {
				return ip
			}
		}
	}
	if len(local) > 0 {
		return nil
	}
	for _, ip := range dns {
		if ip4 := ip.To4(); ip4 != nil && isPublic(ip4) {
			return ip4
		}
	}
	for _, ip := range dns {
		if ip.To4() == nil && isPublic(ip) {
			return ip
		}
	}
	return nil
}

func writeTelemtWebNginxConfig(state TelemtWebProxyState) error {
	if data, err := os.ReadFile("/etc/letsencrypt/renewal/" + state.Domain + ".conf"); err == nil && telemtWebRenewalMode(data) == "managed-webroot" {
		if err := telemtWebWriteAcmeNginxConfig(state.Domain); err != nil {
			return err
		}
	}
	if state.CertFile == "" || state.KeyFile == "" {
		return errors.New("WEB Proxy certificate is not configured")
	}
	if !telemtWebCertificateCoversHosts(state.CertFile, state.KeyFile, telemtWebHosts(state)) {
		return errors.New("WEB certificate does not cover all configured domains")
	}
	listen := "listen 443 ssl;"
	if host, _, err := net.SplitHostPort(state.PublicAddr); err == nil && net.ParseIP(host) != nil && net.ParseIP(host).To4() == nil {
		listen = "listen [::]:443 ssl;"
	}
	locations, err := telemtWebSubscriptionLocations(&SettingService{})
	if err != nil {
		return err
	}
	config := fmt.Sprintf("map $http_upgrade $xui_telemt_connection_upgrade {\n    default upgrade;\n    ''      '';\n}\n\nupstream xui_telemt_web {\n    server %s:%d;\n    keepalive 64;\n}\n\nserver {\n    %s\n    server_name %s;\n    access_log off;\n\n    ssl_certificate %s;\n    ssl_certificate_key %s;\n    ssl_protocols TLSv1.2 TLSv1.3;\n    client_max_body_size 2m;\n\n%s    location / {\n        proxy_pass http://xui_telemt_web;\n        proxy_http_version 1.1;\n        proxy_set_header Host $host;\n        proxy_set_header X-Forwarded-For $remote_addr;\n        proxy_set_header Upgrade $http_upgrade;\n        proxy_set_header Connection $xui_telemt_connection_upgrade;\n        proxy_connect_timeout 5s;\n        proxy_read_timeout 65s;\n        proxy_send_timeout 65s;\n        proxy_request_buffering off;\n        proxy_buffering off;\n        proxy_next_upstream off;\n    }\n}\n", telemtWebListenIP, state.ListenPort, listen, strings.Join(telemtWebHosts(state), " "), state.CertFile, state.KeyFile, locations)
	if err := os.MkdirAll(filepath.Dir(telemtWebNginxConf), 0o755); err != nil {
		return err
	}
	return os.WriteFile(telemtWebNginxConf, []byte(config), 0o644)
}

// Route imported Hiddify paths through the panel's NoRoute handler.
// Their URLs remain independent of the subscription listener.
func telemtWebSubscriptionTargets(s *SettingService) ([]HiddifyLegacySubscriptionAlias, string, error) {
	aliases, err := s.GetHiddifyLegacySubscriptionAliases()
	if err != nil || len(aliases) == 0 {
		return nil, "", err
	}
	port, err := s.GetPort()
	if err != nil {
		return nil, "", err
	}
	listen, err := s.GetListen()
	if err != nil {
		return nil, "", err
	}
	if listen == "" || listen == "0.0.0.0" {
		listen = "127.0.0.1"
	} else if listen == "::" {
		listen = "::1"
	} else if net.ParseIP(listen) == nil {
		return nil, "", fmt.Errorf("invalid panel listen address %q for Telemt WEB proxy", listen)
	}
	cert, err := s.GetCertFile()
	if err != nil {
		return nil, "", err
	}
	key, err := s.GetKeyFile()
	if err != nil {
		return nil, "", err
	}
	scheme := "http"
	if cert != "" && key != "" {
		scheme = "https"
	}
	return aliases, scheme + "://" + net.JoinHostPort(listen, fmt.Sprint(port)), nil
}

func telemtWebSubscriptionLocations(s *SettingService) (string, error) {
	aliases, upstream, err := telemtWebSubscriptionTargets(s)
	if err != nil {
		return "", err
	}
	return renderTelemtWebSubscriptionLocations(aliases, upstream), nil
}

func renderTelemtWebSubscriptionLocations(aliases []HiddifyLegacySubscriptionAlias, upstream string) string {
	var locations strings.Builder
	for _, alias := range aliases {
		path, err := normalizeHiddifyLegacySubPath(alias.Path)
		if err != nil || path == "" {
			continue
		}
		fmt.Fprintf(&locations, "    location ^~ /%s/ {\n        proxy_pass %s;\n        proxy_http_version 1.1;\n        proxy_set_header Host $host;\n        proxy_set_header X-Forwarded-For $remote_addr;\n        proxy_set_header X-Forwarded-Proto $scheme;\n        proxy_connect_timeout 5s;\n        proxy_read_timeout 65s;\n        proxy_send_timeout 65s;\n        proxy_request_buffering off;\n        proxy_buffering off;\n        proxy_max_temp_file_size 0;\n        proxy_next_upstream off;\n    }\n\n", path, upstream)
	}
	return locations.String()
}

// RefreshTelemtWebSubscriptionRoutes updates a previously generated Telemt
// vhost when aliases are imported or after the panel is upgraded.
func RefreshTelemtWebSubscriptionRoutes() error {
	telemtWebOperationMu.Lock()
	defer telemtWebOperationMu.Unlock()
	state, err := readTelemtWebState()
	if err != nil || !state.Enabled {
		return err
	}
	if telemtWebFrontend(state) != "nginx" {
		previous, err := os.ReadFile(telemtWebFrontDir + "/config")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := writeTelemtWebFrontendConfig(state); err != nil {
			return err
		}
		if err := telemtWebReloadFrontend(state); err != nil {
			if previous != nil {
				_ = os.WriteFile(telemtWebFrontDir+"/config", previous, 0o600)
			}
			return err
		}
		return nil
	}
	previous, err := os.ReadFile(telemtWebNginxConf)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := writeTelemtWebNginxConfig(state); err != nil {
		return err
	}
	updated, err := os.ReadFile(telemtWebNginxConf)
	if err != nil {
		return err
	}
	if string(updated) == string(previous) || !telemtWebNginxActive() {
		return nil
	}
	if err := telemtWebReloadNginx(); err != nil {
		_ = os.WriteFile(telemtWebNginxConf, previous, 0o644)
		return err
	}
	return nil
}

func removeTelemtWebNginxConfig() error {
	err := os.Remove(telemtWebNginxConf)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func telemtWebNginxActive() bool {
	if _, err := exec.LookPath("systemctl"); err == nil && systemctl("is-active", "--quiet", "nginx") == nil {
		return true
	}
	if _, err := exec.LookPath("rc-service"); err == nil && exec.CommandContext(context.Background(), "rc-service", "nginx", "status").Run() == nil {
		return true
	}
	return telemtWebNginxOwnsPort(80) || telemtWebNginxOwnsPort443()
}

func telemtWebNginxEnabled() bool {
	if _, err := exec.LookPath("systemctl"); err == nil {
		return systemctl("is-enabled", "--quiet", "nginx") == nil
	}
	if _, err := exec.LookPath("rc-update"); err == nil {
		output, err := exec.CommandContext(context.Background(), "rc-update", "show", "default").CombinedOutput()
		if err == nil {
			for _, line := range strings.Split(string(output), "\n") {
				fields := strings.Fields(line)
				if len(fields) > 0 && fields[0] == "nginx" {
					return true
				}
			}
		}
	}
	return false
}

func telemtWebEnsureNginxRunning() error {
	if err := exec.CommandContext(context.Background(), "nginx", "-t").Run(); err != nil {
		return errors.New("nginx configuration test failed")
	}
	if telemtWebNginxActive() {
		return nil
	}
	if systemctl("enable", "--now", "nginx") == nil {
		return nil
	}
	if _, err := exec.LookPath("rc-service"); err == nil {
		if err := exec.CommandContext(context.Background(), "rc-service", "nginx", "restart").Run(); err != nil {
			return fmt.Errorf("failed to start nginx: %w", err)
		}
		_, _ = exec.CommandContext(context.Background(), "rc-update", "add", "nginx", "default").Output()
		return nil
	}
	return errors.New("failed to start nginx")
}

func telemtWebStopNginx(disable bool) error {
	if _, err := exec.LookPath("systemctl"); err == nil {
		if telemtWebNginxActive() {
			if err := systemctl("stop", "nginx"); err != nil {
				return fmt.Errorf("failed to stop nginx: %w", err)
			}
		}
		if disable {
			if err := systemctl("disable", "nginx"); err != nil {
				return fmt.Errorf("failed to disable nginx: %w", err)
			}
		}
		if telemtWebNginxActive() {
			return errors.New("failed to stop nginx: service is still active")
		}
		return nil
	}
	if _, err := exec.LookPath("rc-service"); err == nil {
		if telemtWebNginxActive() {
			if err := exec.CommandContext(context.Background(), "rc-service", "nginx", "stop").Run(); err != nil {
				return fmt.Errorf("failed to stop nginx: %w", err)
			}
		}
		if disable {
			_, _ = exec.CommandContext(context.Background(), "rc-update", "del", "nginx", "default").Output()
		}
		if !telemtWebNginxActive() {
			return nil
		}
	}
	return errors.New("failed to stop nginx: service is still active")
}

func telemtWebRestoreNginx(state TelemtWebProxyState) error {
	if !state.NginxManaged {
		return nil
	}
	// Legacy state files do not contain the original enabled flag. Preserve the
	// previous behavior for them and fully disable the panel-managed service.
	disable := !state.NginxStateCaptured || !state.NginxWasEnabled
	return telemtWebStopNginx(disable)
}

func telemtWebReloadNginx() error {
	if !telemtWebNginxActive() {
		return nil
	}
	if _, err := exec.LookPath("systemctl"); err == nil {
		if err := systemctl("reload", "nginx"); err == nil {
			return nil
		}
	}
	if _, err := exec.LookPath("rc-service"); err == nil {
		if err := exec.CommandContext(context.Background(), "rc-service", "nginx", "reload").Run(); err == nil {
			return nil
		}
	}
	return errors.New("failed to reload nginx")
}

func telemtWebEnsureNginxPackage(ctx context.Context) error {
	if telemtWebCommandExists("nginx") {
		return nil
	}
	manager := telemtWebPackageManager()
	if manager == "" {
		return errors.New("cannot install nginx automatically: unsupported Linux package manager")
	}
	var args []string
	switch manager {
	case "apt-get":
		args = []string{"apt-get", "update"}
		if err := runTelemtWebCommand(ctx, args...); err != nil {
			return err
		}
		args = []string{"apt-get", "install", "-y", "nginx"}
	case "dnf":
		args = []string{"dnf", "install", "-y", "nginx"}
	case "yum":
		args = []string{"yum", "install", "-y", "nginx"}
	case "pacman":
		args = []string{"pacman", "-Syu", "--noconfirm", "--needed", "nginx"}
	case "zypper":
		args = []string{"zypper", "--non-interactive", "install", "nginx"}
	case "apk":
		args = []string{"apk", "add", "--no-cache", "nginx"}
	default:
		return errors.New("unsupported package manager")
	}
	return runTelemtWebCommand(ctx, args...)
}

func telemtWebEnsureCertificate(ctx context.Context, domain, panelCert, panelKey string) (string, string, error) {
	if telemtWebCertificatePairReady(panelCert, panelKey, domain) {
		return panelCert, panelKey, nil
	}
	acmeCert := "/etc/letsencrypt/live/" + domain + "/fullchain.pem"
	acmeKey := "/etc/letsencrypt/live/" + domain + "/privkey.pem"
	if telemtWebCertificatePairReady(acmeCert, acmeKey, domain) {
		return acmeCert, acmeKey, nil
	}
	if !telemtWebPortAvailable(80) && !telemtWebNginxOwnsPort(80) {
		return "", "", errors.New("automatic certificate issuance needs port 80 to be available or owned by nginx")
	}
	if err := telemtWebWriteAcmeNginxConfig(domain); err != nil {
		return "", "", err
	}
	defer removeTelemtWebAcmeConfig()
	if err := telemtWebEnsureNginxRunning(); err != nil {
		return "", "", err
	}
	if err := telemtWebReloadNginx(); err != nil {
		return "", "", err
	}
	if !telemtWebCommandExists("certbot") {
		if err := telemtWebInstallCertbot(ctx, telemtWebPackageManager()); err != nil {
			return "", "", err
		}
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(timeoutCtx, "certbot", "certonly", "--webroot", "-w", telemtWebDecoyDir, "--non-interactive", "--agree-tos", "--register-unsafely-without-email", "--cert-name", domain, "-d", domain, "--preferred-challenges", "http")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("certbot failed: %s", strings.TrimSpace(string(out)))
	}
	if !telemtWebCertificatePairReady(acmeCert, acmeKey, domain) {
		return "", "", errors.New("certbot completed but certificate files are missing or invalid")
	}
	if err := installTelemtWebRenewalHook(); err != nil {
		return "", "", err
	}
	return acmeCert, acmeKey, nil
}

func telemtWebWriteAcmeNginxConfig(domain string) error {
	if err := os.MkdirAll(filepath.Join(telemtWebDecoyDir, ".well-known", "acme-challenge"), 0o755); err != nil {
		return err
	}
	conf := fmt.Sprintf("server {\n    listen 80;\n    server_name %s;\n    root %s;\n    location /.well-known/acme-challenge/ {\n        try_files $uri =404;\n    }\n}\n", domain, telemtWebDecoyDir)
	return os.WriteFile(telemtWebAcmeConf, []byte(conf), 0o644)
}

func removeTelemtWebAcmeConfig() { _ = os.Remove(telemtWebAcmeConf); _ = telemtWebReloadNginx() }

func renderTelemtWebRenewalHook(state TelemtWebProxyState) string {
	script := "#!/bin/sh\nset -eu\n"
	if telemtWebFrontend(state) == "haproxy" {
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
		script += "umask 077\ntmp=$(mktemp /etc/x-ui/telemt-web-front/tls.pem.XXXXXX)\ntrap 'rm -f \"$tmp\"' EXIT\ncat -- " + quote(state.CertFile) + " " + quote(state.KeyFile) + " > \"$tmp\"\nmv -f -- \"$tmp\" /etc/x-ui/telemt-web-front/tls.pem\n"
	}
	if telemtWebFrontend(state) == "nginx" {
		script += "systemctl reload nginx >/dev/null 2>&1 || rc-service nginx reload >/dev/null 2>&1\n"
	} else {
		script += "systemctl try-reload-or-restart 3x-ui-telemt-web-front.service\n"
	}
	return script
}

func installTelemtWebRenewalHook(states ...TelemtWebProxyState) error {
	dir := "/etc/letsencrypt/renewal-hooks/deploy"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	state := TelemtWebProxyState{}
	if len(states) > 0 {
		state = states[0]
	}
	return os.WriteFile(filepath.Join(dir, "3x-ui-telemt-web.sh"), []byte(renderTelemtWebRenewalHook(state)), 0o755)
}

func telemtWebInstallCertbot(ctx context.Context, manager string) error {
	var args []string
	switch manager {
	case "apt-get":
		args = []string{"apt-get", "update"}
		if err := runTelemtWebCommand(ctx, args...); err != nil {
			return err
		}
		args = []string{"apt-get", "install", "-y", "certbot"}
	case "dnf":
		args = []string{"dnf", "install", "-y", "certbot"}
	case "yum":
		args = []string{"yum", "install", "-y", "certbot"}
	case "pacman":
		args = []string{"pacman", "-Syu", "--noconfirm", "--needed", "certbot"}
	case "zypper":
		args = []string{"zypper", "--non-interactive", "install", "certbot"}
	case "apk":
		args = []string{"apk", "add", "--no-cache", "certbot"}
	default:
		return errors.New("unsupported package manager")
	}
	return runTelemtWebCommand(ctx, args...)
}

func telemtWebPackageManager() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = strings.Trim(strings.TrimSpace(parts[1]), "\"")
		}
	}
	id := strings.ToLower(values["ID"])
	like := strings.ToLower(values["ID_LIKE"])
	switch {
	case id == "ubuntu" || id == "debian" || strings.Contains(like, "debian"):
		return "apt-get"
	case id == "fedora":
		return "dnf"
	case id == "rhel" || id == "almalinux" || id == "rocky" || id == "ol" || id == "amzn" || strings.Contains(like, "rhel") || strings.Contains(like, "fedora"):
		if telemtWebCommandExists("dnf") {
			return "dnf"
		}
		return "yum"
	case id == "centos":
		if telemtWebCommandExists("dnf") {
			return "dnf"
		}
		return "yum"
	case id == "arch" || id == "manjaro" || id == "parch" || strings.Contains(like, "arch"):
		return "pacman"
	case strings.Contains(id, "opensuse") || strings.Contains(like, "suse"):
		return "zypper"
	case id == "alpine":
		return "apk"
	default:
		return ""
	}
}

func telemtWebPortAvailable(port int) bool {
	if telemtWebCommandExists("ss") {
		output, err := exec.CommandContext(context.Background(), "ss", "-lntH", fmt.Sprintf("sport = :%d", port)).CombinedOutput()
		if err != nil {
			return false
		}
		return strings.TrimSpace(string(output)) == ""
	}
	listenConfig := net.ListenConfig{}
	listener, err := listenConfig.Listen(context.Background(), "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

func telemtWebPortOwner(port int) string {
	if !telemtWebCommandExists("ss") {
		return ""
	}
	output, err := exec.CommandContext(context.Background(), "ss", "-lntpH", fmt.Sprintf("sport = :%d", port)).CombinedOutput()
	if err != nil {
		return ""
	}
	return parseTelemtWebPortOwners(string(output))
}

// parseTelemtWebPortOwners collects every process from each ss users:(...) list,
// rather than only the first process on a socket.
func parseTelemtWebPortOwners(output string) string {
	seen := map[string]struct{}{}
	owners := make([]string, 0, 2)
	for _, match := range telemtWebPortOwnerPattern.FindAllStringSubmatch(output, -1) {
		if len(match) < 2 || match[1] == "" {
			continue
		}
		name := strings.ToLower(match[1])
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		owners = append(owners, name)
	}
	return strings.Join(owners, ", ")
}

func telemtWebNginxOwnsPort(port int) bool {
	if !telemtWebCommandExists("ss") {
		return false
	}
	output, err := exec.CommandContext(context.Background(), "ss", "-lntp", fmt.Sprintf("sport = :%d", port)).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "nginx")
}

func telemtWebNginxOwnsPort443() bool { return telemtWebNginxOwnsPort(443) }

func telemtWebVersionAtLeast(current, required string) bool {
	currentMatch := telemtWebVersionPattern.FindStringSubmatch(current)
	requiredMatch := telemtWebVersionPattern.FindStringSubmatch(required)
	if len(currentMatch) != 4 || len(requiredMatch) != 4 {
		return false
	}
	for i := 1; i <= 3; i++ {
		cv := atoiTelemtWeb(currentMatch[i])
		rv := atoiTelemtWeb(requiredMatch[i])
		if cv != rv {
			return cv > rv
		}
	}
	return true
}

func atoiTelemtWeb(value string) int {
	n := 0
	for _, r := range value {
		n = n*10 + int(r-'0')
	}
	return n
}

func telemtWebCommandExists(name string) bool { _, err := exec.LookPath(name); return err == nil }

func runTelemtWebCommand(ctx context.Context, args ...string) error {
	if len(args) == 0 {
		return errors.New("empty package command")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(timeoutCtx, args[0], args[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %s", args[0], strings.TrimSpace(string(output)))
	}
	return nil
}
