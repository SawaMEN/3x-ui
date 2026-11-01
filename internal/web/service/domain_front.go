package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

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
		return rootRedirectDomainConfig(seedDomainFront(settings), settings), nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, fmt.Errorf("domain configuration: %w", err)
	}
	settings, err := s.GetAllSetting()
	if err != nil {
		return cfg, err
	}
	return rootRedirectDomainConfig(cfg, settings), nil
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
	domainFrontState.Lock()
	defer domainFrontState.Unlock()
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
	roots, rollback, err := prepareDomainACME(cfg, files, states)
	if err != nil {
		return domainfront.DomainFrontStatus{}, err
	}
	backend.ChallengeRoots = roots
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
			_ = manager.Apply(ctx, old, oldBackend)
		}
		return manager.Status(old), err
	}
	result := manager.StatusWithCertificates(cfg, files)
	result.ExistingCertificates = s.existingDomainCertificates()
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
	roots, rollback, err := prepareDomainACME(cfg, files, states)
	if err != nil {
		return err
	}
	backend.ChallengeRoots = roots
	if err := managedDomainFront().Apply(context.Background(), cfg, backend); err != nil {
		rollback()
		return err
	}
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
		PanelTarget:        localDomainTarget(settings.WebListen, settings.WebPort, settings.WebCertFile != "" && settings.WebKeyFile != ""),
		PanelHost:          certificateHost(settings.WebCertFile, settings.WebDomain), PanelPath: settings.WebBasePath,
		SubscriptionPaths: []string{settings.SubPath, settings.SubJsonPath, settings.SubClashPath, "/assets/"},
		Routes:            map[string][]domainfront.Route{},
	}
	panelPort := settings.WebPort
	if override, configured, err := config.GetPortOverride(); configured && err == nil {
		panelPort = override
		backend.PanelTarget = localDomainTarget(settings.WebListen, panelPort, settings.WebCertFile != "" && settings.WebKeyFile != "")
	}
	backend.SharedHTTP, backend.SharedHTTPS = domainSharesListener(cfg, settings.WebListen, panelPort, settings.WebCertFile != "" && settings.WebKeyFile != "")
	if backend.SharedHTTP || backend.SharedHTTPS {
		backend.PanelHandler = registeredDomainHandler(settings.WebListen, panelPort, settings.WebCertFile != "" && settings.WebKeyFile != "")
	}
	if settings.SubEnable {
		sharedHTTP, sharedHTTPS := domainSharesListener(cfg, settings.SubListen, settings.SubPort, settings.SubCertFile != "" && settings.SubKeyFile != "")
		if sharedHTTP || sharedHTTPS {
			backend.SubscriptionHandler = registeredDomainHandler(settings.SubListen, settings.SubPort, settings.SubCertFile != "" && settings.SubKeyFile != "")
		}
		backend.SharedHTTP = backend.SharedHTTP || sharedHTTP
		backend.SharedHTTPS = backend.SharedHTTPS || sharedHTTPS
	}
	backend.GetRedirect = s.GetRootRedirectURL
	if settings.SubCertFile != "" && settings.SubKeyFile != "" {
		backend.SubscriptionHost = certificateHost(settings.SubCertFile, settings.SubDomain)
	}
	for _, d := range cfg.Domains {
		if !d.Enabled {
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
			if settings.WebBasePath != "/" && domainPathOverlap(route.Path, settings.WebBasePath) && d.Panel {
				return backend, fmt.Errorf("inbound %d path conflicts with the panel path", id)
			}
			if d.Panel && settings.WebBasePath == "/" && (domainPathOverlap(route.Path, "/panel/") || domainPathOverlap(route.Path, "/login") || domainPathOverlap(route.Path, "/logout")) {
				return backend, fmt.Errorf("inbound %d path conflicts with panel routes", id)
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
