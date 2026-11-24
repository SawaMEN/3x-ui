package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

type telemtIngressPreviewRoute struct {
	Name    string `json:"name"`
	Public  string `json:"public"`
	Backend string `json:"backend"`
	TLS     bool   `json:"tls"`
}

type telemtIngressPreview struct {
	VPNMigrations             []telemtVPNMigrationPlan    `json:"vpnMigrations,omitempty"`
	Routes                    []telemtIngressPreviewRoute `json:"routes"`
	SuggestedAliases          []string                    `json:"suggestedAliases"`
	SuggestedBasePath         string                      `json:"suggestedBasePath,omitempty"`
	CertificateAction         string                      `json:"certificateAction"`
	PanelPort                 int                         `json:"panelPort"`
	SubscriptionPort          int                         `json:"subscriptionPort,omitempty"`
	TelemtPort                int                         `json:"telemtPort,omitempty"`
	SuggestedPanelPort        int                         `json:"suggestedPanelPort,omitempty"`
	SuggestedSubscriptionPort int                         `json:"suggestedSubscriptionPort,omitempty"`
	Manual                    []string                    `json:"manual"`
}

// Reuse the same port conflict checks as inbound creation. This excludes
// reserved panel/core/relay/forwarded ports even while their services are off.
func telemtIngressFindPort(ctx context.Context, first, last int, reserved ...int) (int, error) {
	db := database.GetDB()
	if db == nil {
		return 0, errors.New("database unavailable for managed port selection")
	}
	for port := first; port <= last; port++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if slices.Contains(reserved, port) {
			continue
		}
		candidate := &model.Inbound{Protocol: model.HTTP, Listen: "127.0.0.1", Port: port, Settings: "{}"}
		conflict, err := checkPortConflictTx(db.WithContext(ctx), candidate, 0)
		if err != nil {
			return 0, err
		}
		if conflict != nil {
			continue
		}
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", net.JoinHostPort(candidate.Listen, strconv.Itoa(port)))
		if err == nil {
			_ = listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no unreserved local TCP port in %d–%d", first, last)
}

func pickTelemtWebManagedPort(ctx context.Context, primaryPort int) (int, error) {
	return telemtIngressFindPort(ctx, telemtWebListenPort, telemtWebListenPort+99, primaryPort)
}

func telemtIngressSuggestedPath(path string, protected []string) string {
	available := func(candidate string) bool {
		if candidate == "" {
			return false
		}
		for _, prefix := range protected {
			if domainPathOverlap("/"+candidate, prefix) {
				return false
			}
		}
		return true
	}
	if available(path) {
		return path
	}
	for i := 0; i < 100; i++ {
		candidate := telemtWebIngressDefaultPath
		if i > 0 {
			candidate = fmt.Sprintf("telegram-web-%d", i)
		}
		if available(candidate) {
			return candidate
		}
	}
	return ""
}

func telemtIngressCertificateAction(state, old TelemtWebProxyState, cert, key string) string {
	hosts := telemtWebHosts(state)
	for _, pair := range [][2]string{{cert, key}, {old.CertFile, old.KeyFile}, {"/etc/letsencrypt/live/" + state.Domain + "/fullchain.pem", "/etc/letsencrypt/live/" + state.Domain + "/privkey.pem"}} {
		if telemtWebCertificateCoversHosts(pair[0], pair[1], hosts) {
			return "reuse"
		}
	}
	if len(state.Aliases) != 0 {
		return "san-required"
	}
	return "issue-http01"
}

func buildTelemtIngressPreview(ctx context.Context, state, old TelemtWebProxyState, cert, key string, nativePort, backendPort int) (*telemtIngressPreview, error) {
	service := &SettingService{}
	settings, err := service.GetAllSetting()
	if err != nil {
		return nil, err
	}
	gateway, err := service.domainFrontConfig(settings)
	if err != nil {
		return nil, err
	}
	preview := &telemtIngressPreview{Routes: []telemtIngressPreviewRoute{}, SuggestedAliases: append([]string{}, state.Aliases...), Manual: []string{}, PanelPort: settings.WebPort, TelemtPort: backendPort}
	port, panelOverridden, err := config.GetPortOverride()
	if err != nil {
		return nil, err
	}
	if panelOverridden {
		preview.PanelPort = port
		if port == 443 {
			preview.Manual = append(preview.Manual, "panel-env-port")
		}
	}
	panelHost := normalizeTelemtWebDomain(settings.WebDomain)
	if panelHost == "" {
		panelHost = state.Domain
	}
	subHost := ""
	if settings.SubEnable {
		preview.SubscriptionPort = settings.SubPort
		publicURL := subscriptionPublicURL(gateway, settings)
		if publicURL == "" {
			subHost = state.Domain
		} else if u, parseErr := url.Parse(publicURL); parseErr == nil && u.Scheme == "https" && u.Host == u.Hostname() && (u.Path == "" || u.Path == "/") && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
			subHost = normalizeTelemtWebDomain(u.Hostname())
		} else {
			preview.Manual = append(preview.Manual, "subscription-url")
		}
	}
	for _, host := range []string{panelHost, subHost} {
		if host == "" || host == state.Domain || slices.Contains(preview.SuggestedAliases, host) {
			continue
		}
		if len(preview.SuggestedAliases) >= 7 || len(host) > 253 || !telemtWebDomainPattern.MatchString(host) {
			preview.Manual = append(preview.Manual, "required-host:"+host)
			continue
		}
		preview.SuggestedAliases = append(preview.SuggestedAliases, host)
	}
	// Alias proposals are not applied without explicit user approval; DNS and
	// certificate checks run again when the proposal is accepted.
	protected := []string{settings.WebBasePath}
	if settings.SubEnable {
		protected = append(protected, settings.SubPath, "/assets")
		if settings.SubJsonEnable {
			protected = append(protected, settings.SubJsonPath)
		}
		if settings.SubClashEnable {
			protected = append(protected, settings.SubClashPath, "/mihomo", "/clash-legacy")
		}
	}
	aliases, err := service.GetHiddifyLegacySubscriptionAliases()
	if err != nil {
		return nil, err
	}
	for _, alias := range aliases {
		if path := strings.Trim(alias.Path, "/"); path != "" {
			protected = append(protected, "/"+path)
		}
	}
	preview.SuggestedBasePath = telemtIngressSuggestedPath(state.BasePath, protected)
	if preview.SuggestedBasePath == "" {
		preview.Manual = append(preview.Manual, "web-path")
	}
	if settings.WebCertFile == "" || settings.WebKeyFile == "" {
		preview.Manual = append(preview.Manual, "panel-tls")
	}
	state.Aliases = preview.SuggestedAliases
	preview.CertificateAction = telemtIngressCertificateAction(state, old, cert, key)
	// Suggested ports do not move existing listeners; changing the panel
	// requires a separate restart and may disconnect the administrator.
	if !panelOverridden && (preview.PanelPort <= 0 || preview.PanelPort == 443) {
		preview.SuggestedPanelPort, err = telemtIngressFindPort(ctx, 18080, 18179, nativePort, backendPort, preview.SubscriptionPort)
		if err != nil {
			preview.Manual = append(preview.Manual, "port-unavailable")
		}
		preview.Manual = append(preview.Manual, "panel-port")
	}
	if settings.SubEnable && (preview.SubscriptionPort <= 0 || preview.SubscriptionPort == 443 || preview.SubscriptionPort == preview.PanelPort) {
		preview.SuggestedSubscriptionPort, err = telemtIngressFindPort(ctx, 18080, 18179, nativePort, backendPort, preview.PanelPort, preview.SuggestedPanelPort)
		if err != nil && !slices.Contains(preview.Manual, "port-unavailable") {
			preview.Manual = append(preview.Manual, "port-unavailable")
		}
		preview.Manual = append(preview.Manual, "subscription-port")
	}
	state.ManagedInboundID = old.ManagedInboundID
	preview.VPNMigrations, err = planTelemtVPNMigrations(ctx, state, nativePort, backendPort, preview.PanelPort, preview.SubscriptionPort, preview.SuggestedPanelPort, preview.SuggestedSubscriptionPort)
	if err != nil {
		return nil, err
	}
	return preview, nil
}

func fillTelemtIngressPreview(preview *telemtIngressPreview, state TelemtWebProxyState, plan telemtIngressPlan) {
	if preview == nil {
		return
	}
	add := func(name string, route telemtIngressRoute) {
		if route.host == "" || route.address == "" {
			return
		}
		for _, path := range route.paths {
			preview.Routes = append(preview.Routes, telemtIngressPreviewRoute{Name: name, Public: "https://" + route.host + path + "/", Backend: route.address, TLS: route.certFile != ""})
		}
	}
	add("Panel", plan.panel)
	add("Subscriptions", plan.subscriptions)
	if preview.TelemtPort > 0 {
		preview.Routes = append(preview.Routes, telemtIngressPreviewRoute{Name: "Telemt WEB", Public: "https://" + state.Domain + "/" + state.BasePath + "/", Backend: net.JoinHostPort(telemtWebListenIP, strconv.Itoa(preview.TelemtPort))})
	}
	for _, route := range plan.proxies {
		preview.Routes = append(preview.Routes, telemtIngressPreviewRoute{Name: string(route.protocol), Public: net.JoinHostPort(route.publicAddress, "443") + " (SNI: " + route.host + ")", Backend: route.address, TLS: true})
	}
}
