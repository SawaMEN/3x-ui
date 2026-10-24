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
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, fmt.Errorf("domain configuration: %w", err)
	}
	return cfg, nil
}

func DomainFrontStatus() (domainfront.DomainFrontStatus, error) {
	s := &SettingService{}
	cfg, err := s.GetDomainFrontConfig()
	if err != nil {
		return domainfront.DomainFrontStatus{}, err
	}
	domainFrontState.Lock()
	defer domainFrontState.Unlock()
	return managedDomainFront().Status(cfg), nil
}

func ApplyDomainFront(ctx context.Context, cfg domainfront.DomainFrontConfig) (domainfront.DomainFrontStatus, error) {
	domainFrontState.Lock()
	defer domainFrontState.Unlock()
	s := &SettingService{}
	if err := cfg.Validate(); err != nil {
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
	manager := managedDomainFront()
	if err := manager.Apply(ctx, cfg, backend); err != nil {
		return manager.Status(old), err
	}
	raw, err := json.Marshal(cfg)
	if err == nil {
		err = s.setString("domainFrontConfig", string(raw))
	}
	if err != nil {
		oldBackend, buildErr := s.domainFrontBackend(old)
		if buildErr == nil {
			_ = manager.Apply(ctx, old, oldBackend)
		}
		return manager.Status(old), err
	}
	return manager.Status(cfg), nil
}

func StartDomainFront() error {
	domainFrontState.Lock()
	defer domainFrontState.Unlock()
	s := &SettingService{}
	cfg, err := s.GetDomainFrontConfig()
	if err != nil {
		return err
	}
	backend, err := s.domainFrontBackend(cfg)
	if err != nil {
		return err
	}
	return managedDomainFront().Apply(context.Background(), cfg, backend)
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
		PanelHost:          settings.WebDomain, PanelPath: settings.WebBasePath,
		SubscriptionPaths: []string{settings.SubPath, settings.SubJsonPath, settings.SubClashPath, "/assets/"},
		Routes:            map[string][]domainfront.Route{},
	}
	backend.RedirectURL, _ = s.GetRootRedirectURL()
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
		wanted[inbound.Id] = true
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
