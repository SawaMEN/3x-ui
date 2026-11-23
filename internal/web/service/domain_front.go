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

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

type ManagedSNIDomain struct {
	Name          string `json:"name"`
	Source        string `json:"source"`
	CertFile      string `json:"certFile"`
	KeyFile       string `json:"keyFile"`
	NodeID        int    `json:"nodeId,omitempty"`
	NodeName      string `json:"nodeName,omitempty"`
	ExpiresAt     int64  `json:"expiresAt,omitempty"`
	Issuer        string `json:"issuer,omitempty"`
	RealityTarget string `json:"realityTarget,omitempty"`
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
	add := func(name, source, certFile, keyFile string, port int) {
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
		out = append(out, ManagedSNIDomain{Name: name, Source: source, CertFile: certFile, KeyFile: keyFile, ExpiresAt: leaf.NotAfter.Unix(), Issuer: leaf.Issuer.CommonName, RealityTarget: managedSNIRealityTarget(name, port)})
	}
	// Telemt WEB owns the cover site at public TCP 443. Prefer it when
	// several managed certificates happen to cover the same hostname.
	state, err := readTelemtWebState()
	if err == nil && state.Enabled && telemtWebFrontendActive(state) {
		add(state.Domain, "telemt", state.CertFile, state.KeyFile, 443)
		for _, alias := range state.Aliases {
			add(alias, "telemt", state.CertFile, state.KeyFile, 443)
		}
	}
	// Local sites always take precedence, because their certificate paths
	// belong to the controlling host rather than a connected node.
	if status.Config.Enabled && status.Running {
		enabled := make(map[string]bool)
		ports := make(map[string]int)
		for _, domain := range status.Config.Domains {
			enabled[domain.Name] = domain.Enabled
			if domain.Mode != "cert_only" && domain.Mode != "shared_sni" {
				ports[domain.Name] = status.Config.PublicPort
			}
		}
		for _, cert := range status.Certificates {
			if enabled[cert.Name] && (cert.State == "valid" || cert.State == "renewal_due") {
				add(cert.Name, "gateway", cert.CertFile, cert.KeyFile, ports[cert.Name])
			}
		}
	}
	// Existing panel and subscription certificates can also be reused on
	// this host. Validate their SANs, dates and key pairs with the same add()
	// check; never copy these paths to a connected node.
	if settings, err := (&SettingService{}).GetAllSetting(); err == nil {
		subPort := 0
		if settings.SubEnable {
			subPort = settings.SubPort
		}
		add(settings.SubDomain, "subscription", settings.SubCertFile, settings.SubKeyFile, subPort)
		panelPort := settings.WebPort
		if override, configured, err := config.GetPortOverride(); configured && err == nil {
			panelPort = override
		}
		add(settings.WebDomain, "panel", settings.WebCertFile, settings.WebKeyFile, panelPort)
	}
	// Installed copies are local files on THIS host, not paths borrowed
	// from the controller. Their stable symlink follows certificate updates.
	for _, imported := range LocalDistributedTLSCertificates() {
		add(imported.Domain, "distributed", imported.CertFile, imported.KeyFile, 0)
	}
	if includeNodes {
		out = mergeNodeManagedSNIDomains(out, discoverNodeSNI(ctx), seen)
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
		case "subscription":
			return 2
		case "panel":
			return 3
		default:
			return 4
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

// Remote SNI certificates are scoped to their owning node. Do not collapse
// entries from different nodes, or suppress a remote certificate just because
// the controlling panel exposes the same hostname with its own certificate.
func mergeNodeManagedSNIDomains(existing, incoming []ManagedSNIDomain, seenNames map[string]bool) []ManagedSNIDomain {
	seen := make(map[int]map[string]bool)
	for _, site := range existing {
		if site.Source == "remote" && site.NodeID > 0 {
			if seen[site.NodeID] == nil {
				seen[site.NodeID] = make(map[string]bool)
			}
			seen[site.NodeID][site.Name] = true
		}
	}
	for _, site := range incoming {
		name, err := netsafe.NormalizeHost(strings.ToLower(site.Name))
		if err != nil || site.Source != "remote" || site.NodeID <= 0 ||
			!strings.Contains(name, ".") || net.ParseIP(name) != nil {
			continue
		}
		site.Name = name
		site.RealityTarget = normalizeManagedSNIRealityTarget(name, site.RealityTarget)
		// Discovery deliberately carries metadata only. Requiring node-local
		// paths discards every summary; accepting old peers must not expose paths.
		site.CertFile, site.KeyFile = "", ""
		if seen[site.NodeID] == nil {
			seen[site.NodeID] = make(map[string]bool)
		}
		if seen[site.NodeID][site.Name] {
			continue
		}
		seen[site.NodeID][site.Name] = true
		seenNames[site.Name] = true
		existing = append(existing, site)
	}
	return existing
}

// A connected node may expose its panel, subscription or gateway identities.
// The original source is verified on that node before the controller presents
// its file paths as a node-local certificate inventory entry.
func managedNodeCertificateSourceSupported(source string) bool {
	switch source {
	case "telemt", "gateway", "panel", "subscription", "distributed":
		return true
	default:
		return false
	}
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
			if !managedNodeCertificateSourceSupported(site.Source) || site.Name == "" {
				continue
			}
			// A node confirms ownership of its own certificate. Its absolute
			// paths are intentionally not returned to the parent or browser:
			// the same path may refer to a different key on another machine.
			out = append(out, ManagedSNIDomain{
				Name:          site.Name,
				Source:        "remote",
				NodeID:        result.NodeID,
				NodeName:      result.NodeName,
				ExpiresAt:     site.ExpiresAt,
				Issuer:        site.Issuer,
				RealityTarget: site.RealityTarget,
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
	settings, err := s.GetAllSetting()
	if err != nil {
		return domainfront.DefaultConfig(), err
	}
	return s.domainFrontConfig(settings)
}

func (s *SettingService) domainFrontConfig(settings *entity.AllSetting) (domainfront.DomainFrontConfig, error) {
	raw, err := s.getString("domainFrontConfig")
	cfg := domainfront.DefaultConfig()
	if err != nil {
		return cfg, err
	}
	if strings.TrimSpace(raw) == "" {
		cfg = seedDomainFront(settings)
	} else if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, fmt.Errorf("domain configuration: %w", err)
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

// DomainFrontLiveCertificateStatus reads the gateway's certificate state
// without rescanning installer certificates or spawning external commands.
func DomainFrontLiveCertificateStatus(cfg domainfront.DomainFrontConfig) domainfront.DomainFrontStatus {
	domainFrontState.Lock()
	manager := domainFrontState.manager
	domainFrontState.Unlock()
	if manager == nil {
		return domainfront.DomainFrontStatus{}
	}
	return manager.Status(cfg)
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
		if domain.Enabled && domain.Mode != "shared_sni" && domain.Mode != "cert_only" {
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
	panel, err := s.GetAllSetting()
	if err != nil {
		return domainfront.DomainFrontStatus{}, err
	}
	manager := managedDomainFront()
	subFallback := make(map[string]string)
	oldDomain, hadGatewaySubscription := configuredSubscriptionDomain(old)
	_, hasGatewaySubscription := configuredSubscriptionDomain(cfg)
	if hadGatewaySubscription && !hasGatewaySubscription && panel.SubEnable {
		managedListener := managedSubscriptionListenerDomain(old, panel)
		if strings.TrimSpace(panel.SubDomain) == "" && strings.TrimSpace(panel.SubPublicURL) == "" && !net.ParseIP(panel.SubListen).IsLoopback() {
			panel.SubDomain = oldDomain.Name
			subFallback["subDomain"] = panel.SubDomain
		}
		if managedListener == oldDomain.Name && panel.SubCertFile == "" && panel.SubKeyFile == "" {
			// Keep the certificate already presented by the direct TLS
			// listener available after the gateway stops and after restart.
			for _, cert := range manager.Status(old).Certificates {
				if cert.Name == oldDomain.Name && telemtWebCertificatePairReady(cert.CertFile, cert.KeyFile, cert.Name) {
					panel.SubCertFile, panel.SubKeyFile = cert.CertFile, cert.KeyFile
					subFallback["subCertFile"], subFallback["subKeyFile"] = cert.CertFile, cert.KeyFile
					break
				}
			}
			if panel.SubCertFile == "" {
				return manager.Status(old), fmt.Errorf("no usable certificate for the direct subscription listener")
			}
		}
	}
	if panel.SubRestrictDomains && subscriptionPublicURL(cfg, panel) == "" && directSubscriptionOrigin(panel) == "" {
		return manager.Status(old), fmt.Errorf("restricted subscriptions require a public subscription URL or a publicly listening subscription service with a domain")
	}
	if err := validatePanelCertificateSource(old, cfg, panel.WebDomain, panel.WebCertFile, panel.WebKeyFile); err != nil {
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
	if err := manager.ApplyAndCommit(ctx, cfg, backend, func() error {
		raw, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		return database.GetDB().Transaction(func(tx *gorm.DB) error {
			for key, value := range subFallback {
				if err := saveSettingIn(tx, key, value); err != nil {
					return err
				}
			}
			return saveSettingIn(tx, "domainFrontConfig", string(raw))
		})
	}); err != nil {
		rollback()
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
	subTLS := settings.SubCertFile != "" && settings.SubKeyFile != ""
	subListener := registeredDomainSubscriptionListener(settings.SubListen, settings.SubPort)
	if subListener != nil {
		subTLS = subListener.secure
	}
	backend := domainfront.Backend{
		SubscriptionTarget: localDomainTarget(settings.SubListen, settings.SubPort, subTLS),
		PanelPath:          settings.WebBasePath,
		PanelHost:          strings.TrimSuffix(strings.ToLower(strings.TrimSpace(settings.WebDomain)), "."),
		SubscriptionPaths:  []string{settings.SubPath, settings.SubJsonPath, settings.SubClashPath, "/assets/"},
		Routes:             map[string][]domainfront.Route{},
		DecoyDirectories:   map[string]string{},
	}
	panelPort := settings.WebPort
	if override, configured, err := config.GetPortOverride(); configured && err == nil {
		panelPort = override
	}
	backend.SharedHTTP, backend.SharedHTTPS = domainSharesListener(cfg, settings.WebListen, panelPort, settings.WebCertFile != "" && settings.WebKeyFile != "")
	if err := validateCoverPanelPaths(cfg, backend.PanelHost, backend.PanelPath, backend.SharedHTTPS); err != nil {
		return backend, err
	}
	if settings.SubEnable {
		sharedHTTP, sharedHTTPS := domainSharesListener(cfg, settings.SubListen, settings.SubPort, subTLS)
		if subListener != nil {
			backend.SubscriptionHandler = subListener.handler
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
	if subTLS {
		backend.SubscriptionHost = certificateHost(settings.SubCertFile, settings.SubDomain)
	}
	for _, d := range cfg.Domains {
		if !d.Enabled || d.Mode == "shared_sni" || d.Mode == "cert_only" {
			continue
		}
		if d.HasCoverSite() {
			// Reuse Telemt's existing decoy-site renderer and atomic file writer
			// instead of maintaining a second set of website templates.
			sitePath := filepath.Join(config.GetDBFolderPath(), "domain-tls", "sni-sites", fmt.Sprintf("%x", sha256.Sum256([]byte(d.Name))))
			var site *TelemtWebSiteOptions
			if d.SiteCatalog != "" || d.SiteTemplate != "" {
				var siteErr error
				site, siteErr = managedCoverSiteOptions(d.SiteCatalog, d.SiteTemplate, d.SiteHTML)
				if siteErr != nil {
					return backend, fmt.Errorf("cover site %s: %w", d.Name, siteErr)
				}
			}
			dir, siteErr := prepareTelemtWebSite(d.Name, filepath.Join(sitePath, "default"), filepath.Join(sitePath, "snapshots"), site)
			if siteErr != nil {
				return backend, fmt.Errorf("cover site %s: %w", d.Name, siteErr)
			}
			backend.DecoyDirectories[d.Name] = dir
		}
		if d.Mode == "sni_only" {
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

// validateCoverPanelPaths is also used by preflight, without rendering files.
func validateCoverPanelPaths(cfg domainfront.DomainFrontConfig, host, path string, shared bool) error {
	if !cfg.Enabled || !shared || (path != "" && path != "/") {
		return nil
	}
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	for _, d := range cfg.Domains {
		if d.Enabled && d.HasCoverSite() && (d.Mode != "sni_only" || d.SiteCatalog != "") &&
			strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(d.Name), "."), host) {
			return fmt.Errorf("cover site %s: configure a non-root panel path or separate panel port", d.Name)
		}
	}
	return nil
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
	for _, inbound := range inbounds {
		if host := telemtIngressNaiveHost(inbound); host != nil {
			out[inbound.Id] = append(out[inbound.Id], host)
		}
	}
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
