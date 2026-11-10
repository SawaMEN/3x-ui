package service

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

type ManagedSNIDomain struct {
	Name     string `json:"name"`
	Source   string `json:"source"`
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`
	NodeID   int    `json:"nodeId,omitempty"`
	NodeName string `json:"nodeName,omitempty"`
}

// ManagedSNIDomains reads the *main panel's* live HTTPS endpoints. Remote node
// editors may reuse these public names, but must never reuse their file paths
// without explicitly provisioning a certificate on that remote host.
func ManagedSNIDomains(ctx context.Context, includeNodes bool) ([]ManagedSNIDomain, error) {
	status, err := DomainFrontStatus()
	if err != nil {
		return nil, err
	}
	out := make([]ManagedSNIDomain, 0, len(status.Certificates)+1)
	seen := make(map[string]bool)
	add := func(name, source, certFile, keyFile string) {
		name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
		if name == "" || seen[name] || certFile == "" || keyFile == "" {
			return
		}
		pair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil || len(pair.Certificate) == 0 {
			return
		}
		leaf, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil || leaf.VerifyHostname(name) != nil || time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
			return
		}
		seen[name] = true
		out = append(out, ManagedSNIDomain{Name: name, Source: source, CertFile: certFile, KeyFile: keyFile})
	}
	// Telemt WEB owns the cover site at public TCP 443. Prefer it when
	// several managed certificates happen to cover the same hostname.
	state, err := readTelemtWebState()
	if err == nil && state.Enabled && telemtWebFrontendActive(state) {
		add(state.Domain, "telemt", state.CertFile, state.KeyFile)
		for _, alias := range state.Aliases {
			add(alias, "telemt", state.CertFile, state.KeyFile)
		}
	}
	// Local sites always take precedence, because their certificate paths
	// belong to the controlling host rather than a connected node.
	if status.Config.Enabled && status.Running {
		enabled := make(map[string]bool)
		for _, domain := range status.Config.Domains {
			enabled[domain.Name] = domain.Enabled
		}
		for _, cert := range status.Certificates {
			if enabled[cert.Name] && (cert.State == "valid" || cert.State == "renewal_due") {
				add(cert.Name, "gateway", cert.CertFile, cert.KeyFile)
			}
		}
	}
	if includeNodes {
		for _, site := range discoverNodeSNI(ctx) {
			site.Name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(site.Name)), ".")
			if site.Name == "" || !strings.Contains(site.Name, ".") || net.ParseIP(site.Name) != nil || seen[site.Name] {
				continue
			}
			seen[site.Name] = true
			out = append(out, site)
		}
	}
	// Manual references survive an offline or older child node.
	for _, domain := range status.Config.Domains {
		if domain.Enabled && domain.Mode == "shared_sni" && !seen[domain.Name] {
			seen[domain.Name] = true
			out = append(out, ManagedSNIDomain{Name: domain.Name, Source: "remote"})
		}
	}
	priority := func(source string) int {
		switch source {
		case "telemt":
			return 0
		case "gateway":
			return 1
		default:
			return 2
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := priority(out[i].Source), priority(out[j].Source); a != b {
			return a < b
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

type nodeSNIResponse struct {
	NodeID   int
	NodeName string
	Sites    []runtime.RemoteManagedSNI
}

// Discover active cover sites on connected nodes. A single stale node may not
// delay the SNI dropdown beyond five seconds. Unreachable nodes do not prevent
// locally stored SNI references from loading.
func discoverNodeSNI(ctx context.Context) []ManagedSNIDomain {
	manager := runtime.GetManager()
	db := database.GetDB()
	if manager == nil || db == nil {
		return nil
	}
	var nodes []model.Node
	if err := db.Where("enable = ?", true).Limit(64).Find(&nodes).Error; err != nil {
		return nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	limit := make(chan struct{}, 6)
	results := make(chan nodeSNIResponse, len(nodes))
	var wg sync.WaitGroup
	for _, node := range nodes {
		if node.Id <= 0 {
			continue
		}
		wg.Add(1)
		go func(node model.Node) {
			defer wg.Done()
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-requestCtx.Done():
				return
			}
			remote, err := manager.RemoteFor(&node)
			if err != nil {
				return
			}
			sites, err := remote.ListManagedSNI(requestCtx)
			if err == nil && len(sites) > 0 {
				results <- nodeSNIResponse{NodeID: node.Id, NodeName: node.Name, Sites: sites}
			}
		}(node)
	}
	wg.Wait()
	close(results)
	var out []ManagedSNIDomain
	for result := range results {
		for _, site := range result.Sites {
			if (site.Source != "telemt" && site.Source != "gateway") || site.CertFile == "" || site.KeyFile == "" {
				continue
			}
			out = append(out, ManagedSNIDomain{
				Name:     site.Name,
				Source:   "remote",
				CertFile: site.CertFile,
				KeyFile:  site.KeyFile,
				NodeID:   result.NodeID,
				NodeName: result.NodeName,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NodeID != out[j].NodeID {
			return out[i].NodeID < out[j].NodeID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

var domainFrontState struct {
	sync.Mutex
	manager *domainfront.Manager
}

func managedDomainFront() *domainfront.Manager {
	if domainFrontState.manager == nil {
		domainFrontState.manager = domainfront.NewManager(filepath.Join(config.GetDBFolderPath(), "domain-tls"))
	}
	return domainFrontState.manager
}

func (s *SettingService) GetDomainFrontConfig() (domainfront.DomainFrontConfig, error) {
	raw, err := s.getString("domainFrontConfig")
	cfg := domainfront.DefaultConfig()
	if err != nil {
		return cfg, err
	}
	if strings.TrimSpace(raw) == "" {
		settings, err := s.GetAllSetting()
		if err != nil {
			return cfg, err
		}
		return privateDomainFrontConfig(rootRedirectDomainConfig(seedDomainFront(settings), settings)), nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, fmt.Errorf("domain configuration: %w", err)
	}
	settings, err := s.GetAllSetting()
	if err != nil {
		return cfg, err
	}
	return privateDomainFrontConfig(rootRedirectDomainConfig(cfg, settings)), nil
}

// Legacy gateway panel flags are ignored: the admin UI has a dedicated listener.
func privateDomainFrontConfig(cfg domainfront.DomainFrontConfig) domainfront.DomainFrontConfig {
	for i := range cfg.Domains {
		cfg.Domains[i].Panel = false
	}
	return cfg
}

func DomainFrontStatus() (domainfront.DomainFrontStatus, error) {
	s := &SettingService{}
	cfg, err := s.GetDomainFrontConfig()
	if err != nil {
		return domainfront.DomainFrontStatus{}, err
	}
	domainFrontState.Lock()
	defer domainFrontState.Unlock()
	files, _, err := s.domainCertificateFiles(cfg)
	if err != nil {
		return domainfront.DomainFrontStatus{}, err
	}
	result := managedDomainFront().StatusWithCertificates(cfg, files)
	result.ExistingCertificates = s.existingDomainCertificates()
	return result, nil
}

func ApplyDomainFront(ctx context.Context, cfg domainfront.DomainFrontConfig) (domainfront.DomainFrontStatus, error) {
	// Reject new gateway panel routes, but keep the legacy flag readable in
	// stored configurations and ignored by the gateway manager.
	for _, domain := range cfg.Domains {
		if domain.Panel {
			return domainfront.DomainFrontStatus{}, fmt.Errorf("domain %q cannot expose the admin panel; use the configured web port", domain.Name)
		}
	}
	domainFrontState.Lock()
	defer domainFrontState.Unlock()
	hasLocalDomain := false
	for _, domain := range cfg.Domains {
		if domain.Enabled && domain.Mode != "shared_sni" {
			hasLocalDomain = true
			break
		}
	}
	if cfg.Enabled && hasLocalDomain {
		if state, stateErr := readTelemtWebState(); stateErr == nil && state.Enabled && telemtWebFrontendActive(state) && cfg.HTTPSPort == 443 {
			return domainfront.DomainFrontStatus{}, fmt.Errorf("TCP 443 is already serving the Telemt WEB cover site (%s); reuse it as SNI instead of starting another gateway", state.Domain)
		}
	}
	s := &SettingService{}
	files, states, err := s.domainCertificateFiles(cfg)
	if err != nil {
		return domainfront.DomainFrontStatus{}, err
	}
	if err := cfg.ValidateWithCertificates(files); err != nil {
		return domainfront.DomainFrontStatus{}, err
	}
	backend, err := s.domainFrontBackend(cfg)
	if err != nil {
		return domainfront.DomainFrontStatus{}, err
	}
	old, err := s.GetDomainFrontConfig()
	if err != nil {
		return domainfront.DomainFrontStatus{}, err
	}
	backend.Certificates = files
	if err := configureDomainDNS01(cfg, &backend); err != nil {
		return domainfront.DomainFrontStatus{}, err
	}
	roots, rollback, err := prepareDomainACME(cfg, files, states)
	if err != nil {
		return domainfront.DomainFrontStatus{}, err
	}
	backend.ChallengeRoots = roots
	backend.CertificateEvent = DomainCertificateAuditedEventsForDNS(cfg.HTTPPort, cfg.HTTPSPort, cfg.DNS01Provider != "")
	manager := managedDomainFront()
	if err := manager.Apply(ctx, cfg, backend); err != nil {
		rollback()
		return manager.Status(old), err
	}
	raw, err := json.Marshal(cfg)
	if err == nil {
		err = s.setString("domainFrontConfig", string(raw))
	}
	if err != nil {
		rollback()
		oldBackend, buildErr := s.domainFrontBackend(old)
		if buildErr == nil {
			if tokenErr := configureDomainDNS01(old, &oldBackend); tokenErr == nil {
				_ = manager.Apply(ctx, old, oldBackend)
			}
		}
		return manager.Status(old), err
	}
	result := manager.StatusWithCertificates(cfg, files)
	result.ExistingCertificates = s.existingDomainCertificates()
	TriggerFirewallSync()
	return result, nil
}

func StartDomainFront() error {
	domainFrontState.Lock()
	defer domainFrontState.Unlock()
	s := &SettingService{}
	cfg, err := s.GetDomainFrontConfig()
	if err != nil {
		return err
	}
	backend, err := s.domainFrontBackend(domainFrontRoutingConfig(cfg))
	if err != nil {
		return err
	}
	files, states, err := s.domainCertificateFiles(cfg)
	if err != nil {
		return err
	}
	backend.Certificates = files
	if err := configureDomainDNS01(cfg, &backend); err != nil {
		return err
	}
	roots, rollback, err := prepareDomainACME(cfg, files, states)
	if err != nil {
		return err
	}
	backend.ChallengeRoots = roots
	backend.CertificateEvent = DomainCertificateAuditedEventsForDNS(cfg.HTTPPort, cfg.HTTPSPort, cfg.DNS01Provider != "")
	if err := managedDomainFront().Apply(context.Background(), cfg, backend); err != nil {
		rollback()
		return err
	}
	TriggerFirewallSync()
	return nil
}

func StopDomainFront() {
	domainFrontState.Lock()
	defer domainFrontState.Unlock()
	if domainFrontState.manager != nil {
		domainFrontState.manager.Stop()
	}
}

func localDomainTarget(listen string, port int, tls bool) string {
	if listen == "" || listen == "0.0.0.0" || listen == "::" {
		listen = "127.0.0.1"
	}
	scheme := "http"
	if tls {
		scheme = "https"
	}
	return scheme + "://" + net.JoinHostPort(listen, strconv.Itoa(port))
}

func (s *SettingService) domainFrontBackend(cfg domainfront.DomainFrontConfig) (domainfront.Backend, error) {
	settings, err := s.GetAllSetting()
	if err != nil {
		return domainfront.Backend{}, err
	}
	backend := domainfront.Backend{
		SubscriptionTarget: localDomainTarget(settings.SubListen, settings.SubPort, settings.SubCertFile != "" && settings.SubKeyFile != ""),
		PanelPath:          settings.WebBasePath,
		SubscriptionPaths:  []string{settings.SubPath, settings.SubJsonPath, settings.SubClashPath, "/assets/"},
		Routes:             map[string][]domainfront.Route{},
		DecoyDirectories:   map[string]string{},
	}
	panelPort := settings.WebPort
	if override, configured, err := config.GetPortOverride(); configured && err == nil {
		panelPort = override
	}
	backend.SharedHTTP, backend.SharedHTTPS = domainSharesListener(cfg, settings.WebListen, panelPort, settings.WebCertFile != "" && settings.WebKeyFile != "")
	if settings.SubEnable {
		sharedHTTP, sharedHTTPS := domainSharesListener(cfg, settings.SubListen, settings.SubPort, settings.SubCertFile != "" && settings.SubKeyFile != "")
		if sharedHTTP || sharedHTTPS {
			backend.SubscriptionHandler = registeredDomainHandler(settings.SubListen, settings.SubPort, settings.SubCertFile != "" && settings.SubKeyFile != "")
		}
		backend.SharedHTTP = backend.SharedHTTP || sharedHTTP
		backend.SharedHTTPS = backend.SharedHTTPS || sharedHTTPS
	}
	// Imported Hiddify URLs are dispatched through NoRoute. Never send
	// their secret paths to a public decoy, even if no explicit route matches.
	legacyAliases, err := s.GetHiddifyLegacySubscriptionAliases()
	if err != nil {
		return backend, err
	}
	for _, alias := range legacyAliases {
		if alias.Path != "" {
			backend.ProtectedPaths = append(backend.ProtectedPaths, "/"+strings.Trim(alias.Path, "/"))
		}
	}
	backend.GetRedirect = s.GetRootRedirectURL
	if settings.SubCertFile != "" && settings.SubKeyFile != "" {
		backend.SubscriptionHost = certificateHost(settings.SubCertFile, settings.SubDomain)
	}
	for _, d := range cfg.Domains {
		if !d.Enabled || d.Mode == "shared_sni" {
			continue
		}
		if d.Mode == "sni_only" {
			// Reuse Telemt's existing decoy-site renderer and atomic file writer
			// instead of maintaining a second set of website templates.
			sitePath := filepath.Join(config.GetDBFolderPath(), "domain-tls", "sni-sites", fmt.Sprintf("%x", sha256.Sum256([]byte(d.Name))))
			dir, siteErr := prepareTelemtWebSite(d.Name, filepath.Join(sitePath, "default"), filepath.Join(sitePath, "snapshots"), nil)
			if siteErr != nil {
				return backend, fmt.Errorf("SNI site %s: %w", d.Name, siteErr)
			}
			backend.DecoyDirectories[d.Name] = dir
			continue
		}
		for _, id := range d.InboundIDs {
			var inbound model.Inbound
			if err := database.GetDB().First(&inbound, id).Error; err != nil {
				return backend, fmt.Errorf("%s inbound %d: %w", d.Name, id, err)
			}
			route, err := domainInboundRoute(&inbound, d.Name)
			if err != nil {
				return backend, err
			}
			for _, prefix := range backend.SubscriptionPaths {
				if domainPathOverlap(route.Path, prefix) {
					return backend, fmt.Errorf("inbound %d path conflicts with subscription/assets paths", id)
				}
			}
			for _, existing := range backend.Routes[d.Name] {
				if domainPathOverlap(existing.Path, route.Path) {
					return backend, fmt.Errorf("%s: inbound paths overlap", d.Name)
				}
			}
			backend.Routes[d.Name] = append(backend.Routes[d.Name], route)
		}
	}
	return backend, nil
}

func domainPathOverlap(a, b string) bool {
	a = strings.TrimRight(a, "/")
	b = strings.TrimRight(b, "/")
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func domainInboundRoute(inbound *model.Inbound, host string) (domainfront.Route, error) {
	if !inbound.Enable || inbound.NodeID != nil {
		return domainfront.Route{}, fmt.Errorf("inbound %d must be enabled and local", inbound.Id)
	}
	if strings.HasPrefix(inbound.Listen, "@") || strings.HasPrefix(inbound.Listen, "/") {
		return domainfront.Route{}, fmt.Errorf("inbound %d must use an IP listener", inbound.Id)
	}
	var stream struct {
		Network  string `json:"network"`
		Security string `json:"security"`
		WS       struct {
			Path string `json:"path"`
		} `json:"wsSettings"`
		Upgrade struct {
			Path string `json:"path"`
		} `json:"httpupgradeSettings"`
		XHTTP struct {
			Path string `json:"path"`
		} `json:"xhttpSettings"`
		Split struct {
			Path string `json:"path"`
		} `json:"splithttpSettings"`
		GRPC struct {
			ServiceName string `json:"serviceName"`
		} `json:"grpcSettings"`
	}
	if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
		return domainfront.Route{}, err
	}
	path := ""
	switch stream.Network {
	case "ws":
		path = stream.WS.Path
	case "httpupgrade":
		path = stream.Upgrade.Path
	case "xhttp":
		path = stream.XHTTP.Path
	case "splithttp":
		path = stream.Split.Path
	case "grpc":
		path = "/" + strings.Trim(stream.GRPC.ServiceName, "/")
	default:
		return domainfront.Route{}, fmt.Errorf("inbound %d needs WS, HTTPUpgrade, XHTTP or gRPC transport for the domain gateway", inbound.Id)
	}
	if stream.Security != "" && stream.Security != "none" && stream.Security != "tls" {
		return domainfront.Route{}, fmt.Errorf("inbound %d has unsupported gateway TLS mode", inbound.Id)
	}
	parsed, err := url.ParseRequestURI(path)
	if err != nil || parsed.Path == "/" || !strings.HasPrefix(parsed.Path, "/") || strings.ContainsAny(parsed.Path, "\\ \r\n\t") {
		return domainfront.Route{}, fmt.Errorf("inbound %d needs a unique non-root transport path", inbound.Id)
	}
	for _, part := range strings.Split(parsed.Path, "/") {
		if part == "." || part == ".." {
			return domainfront.Route{}, fmt.Errorf("inbound %d path cannot contain dot segments", inbound.Id)
		}
	}
	path = parsed.Path

	return domainfront.Route{Path: path, Target: localDomainTarget(inbound.Listen, inbound.Port, stream.Security == "tls"), Host: host, H2C: stream.Network == "grpc" && stream.Security != "tls"}, nil
}

func (s *SettingService) DomainFrontHosts(inbounds []*model.Inbound) (map[int][]*model.Host, error) {
	cfg, err := s.GetDomainFrontConfig()
	out := map[int][]*model.Host{}
	if err != nil || !cfg.Enabled {
		return out, err
	}
	wanted := map[int]bool{}
	for _, inbound := range inbounds {
		_, routeErr := domainInboundRoute(inbound, "")
		wanted[inbound.Id] = routeErr == nil
	}
	for _, d := range cfg.Domains {
		if !d.Enabled || d.Mode == "sub_link_only" {
			continue
		}
		for _, id := range d.InboundIDs {
			if wanted[id] {
				out[id] = append(out[id], &model.Host{InboundId: id, Remark: d.Name, Address: d.Name, Port: cfg.PublicPort, Security: "tls", Sni: d.Name, HostHeader: d.Name})
			}
		}
	}
	return out, nil
}

func RefreshDomainFrontRoutes() error {
	domainFrontState.Lock()
	defer domainFrontState.Unlock()
	if domainFrontState.manager == nil {
		return nil
	}
	s := &SettingService{}
	cfg, err := s.GetDomainFrontConfig()
	if err != nil || !cfg.Enabled {
		return err
	}
	backend, err := s.domainFrontBackend(domainFrontRoutingConfig(cfg))
	if err != nil {
		return err
	}
	return domainFrontState.manager.RefreshRoutes(backend)
}

func domainFrontRoutingConfig(cfg domainfront.DomainFrontConfig) domainfront.DomainFrontConfig {
	cfg.Domains = append([]domainfront.DomainFrontDomain(nil), cfg.Domains...)
	for i := range cfg.Domains {
		ids := []int{}
		for _, id := range cfg.Domains[i].InboundIDs {
			var inbound model.Inbound
			if err := database.GetDB().First(&inbound, id).Error; err != nil {
				continue
			}
			if _, err := domainInboundRoute(&inbound, cfg.Domains[i].Name); err == nil {
				ids = append(ids, id)
			}
		}
		cfg.Domains[i].InboundIDs = ids
	}
	return cfg
}
