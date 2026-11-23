package service

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

func (s *SettingService) GetSubRestrictDomains() (bool, error) {
	return s.getBool("subRestrictDomains")
}

func (s *SettingService) GetSubPublicURL() (string, error) {
	cfg, settings, err := s.subscriptionPublishingSettings()
	if err != nil {
		return "", err
	}
	return effectiveSubscriptionPublicURL(cfg, settings), nil
}

func effectiveSubscriptionPublicURL(cfg domainfront.DomainFrontConfig, settings *entity.AllSetting) string {
	if url := subscriptionPublicURL(cfg, settings); url != "" {
		return url
	}
	if settings.SubEnable {
		return telemtIngressSubscriptionOrigin()
	}
	return ""
}

func configuredSubscriptionDomain(cfg domainfront.DomainFrontConfig) (domainfront.DomainFrontDomain, bool) {
	if !cfg.Enabled || cfg.SubscriptionDomain == "" {
		return domainfront.DomainFrontDomain{}, false
	}
	for _, domain := range cfg.Domains {
		if !domain.Enabled || domain.Name != cfg.SubscriptionDomain {
			continue
		}
		switch domain.Mode {
		case "direct", "sub_link_only", "cdn", "relay":
			return domain, true
		}
	}
	return domainfront.DomainFrontDomain{}, false
}

func subscriptionPublicURL(cfg domainfront.DomainFrontConfig, settings *entity.AllSetting) string {
	domain, ok := configuredSubscriptionDomain(cfg)
	if !ok {
		return strings.TrimRight(strings.TrimSpace(settings.SubPublicURL), "/")
	}
	port := cfg.PublicPort
	if managedSubscriptionListenerDomain(cfg, settings) != "" {
		port = settings.SubPort
	}
	host := domain.Name
	if port != 443 {
		host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return "https://" + host
}

// The standalone listener remains a valid publication after the gateway
// is disabled. A loopback-only listener still requires an explicit proxy URL.
func directSubscriptionOrigin(settings *entity.AllSetting) string {
	if !settings.SubEnable || settings.SubPort <= 0 || net.ParseIP(settings.SubListen).IsLoopback() {
		return ""
	}
	host, err := netsafe.NormalizeHost(strings.ToLower(strings.TrimSpace(settings.SubDomain)))
	if err != nil || host == "" {
		return ""
	}
	secure := settings.SubCertFile != "" && settings.SubKeyFile != ""
	scheme, defaultPort := "http", 80
	if secure {
		scheme, defaultPort = "https", 443
	}
	if settings.SubPort != defaultPort {
		host = net.JoinHostPort(host, strconv.Itoa(settings.SubPort))
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return scheme + "://" + host
}

func (s *SettingService) subscriptionPublishingSettings() (domainfront.DomainFrontConfig, *entity.AllSetting, error) {
	settings, err := s.GetAllSetting()
	if err != nil {
		return domainfront.DomainFrontConfig{}, nil, err
	}
	cfg, err := s.domainFrontConfig(settings)
	return cfg, settings, err
}

// ManagedSubscriptionListenerDomain returns the gateway identity for direct
// HTTPS on the configured subscription listener.
func (s *SettingService) ManagedSubscriptionListenerDomain() (string, error) {
	cfg, settings, err := s.subscriptionPublishingSettings()
	if err != nil {
		return "", err
	}
	return managedSubscriptionListenerDomain(cfg, settings), nil
}

func managedSubscriptionListenerDomain(cfg domainfront.DomainFrontConfig, settings *entity.AllSetting) string {
	domain, ok := configuredSubscriptionDomain(cfg)
	if !ok || (domain.Mode != "direct" && domain.Mode != "sub_link_only") ||
		!settings.SubEnable || net.ParseIP(settings.SubListen).IsLoopback() {
		return ""
	}
	if cfg.PublicPort == cfg.HTTPSPort && strings.EqualFold(strings.TrimSuffix(settings.SubDomain, "."), domain.Name) {
		return domain.Name
	}
	// An address saved for the same domain and the direct listener port can
	// still select that listener. The gateway always supplies the hostname.
	if u, err := url.Parse(strings.TrimSpace(settings.SubPublicURL)); err == nil &&
		u.Scheme == "https" && strings.EqualFold(strings.TrimSuffix(u.Hostname(), "."), domain.Name) {
		port := 443
		if u.Port() != "" {
			port, _ = strconv.Atoi(u.Port())
		}
		if port == settings.SubPort {
			return domain.Name
		}
	}
	return ""
}

func (s *SettingService) GetSubCDNEnable() (bool, error) {
	cfg, settings, err := s.subscriptionPublishingSettings()
	if err != nil {
		return false, err
	}
	if domain, ok := configuredSubscriptionDomain(cfg); ok {
		return domain.Mode == "cdn", nil
	}
	return settings.SubCDNEnable, nil
}

func (s *SettingService) GetSubscriptionAllowedHosts(primary string) (map[string]bool, error) {
	hosts := map[string]bool{}
	cfg, settings, err := s.subscriptionPublishingSettings()
	if err != nil {
		return nil, err
	}
	restricted := settings.SubRestrictDomains
	if cfg.Enabled && !restricted {
		for _, d := range cfg.Domains {
			if d.Enabled {
				hosts[d.Name] = true
			}
		}
	}
	if primary != "" && !restricted {
		hosts[strings.TrimSuffix(strings.ToLower(primary), ".")] = true
	}
	publicURL := effectiveSubscriptionPublicURL(cfg, settings)
	if restricted && publicURL == "" {
		publicURL = directSubscriptionOrigin(settings)
	}
	if publicURL != "" {
		u, err := url.Parse(publicURL)
		if err != nil {
			return nil, err
		}
		hosts[strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")] = true
	}
	for _, host := range strings.FieldsFunc(settings.SubAllowedDomains, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == ';' }) {
		hosts[strings.TrimSuffix(strings.ToLower(host), ".")] = true
	}
	if restricted && publicURL == "" {
		return nil, fmt.Errorf("restricted subscriptions require a public subscription URL")
	}
	return hosts, nil
}

func validateSubscriptionDomains(settings *entity.AllSetting, managedOrigin bool) error {
	settings.SubPublicURL = strings.TrimSpace(settings.SubPublicURL)
	if settings.SubRestrictDomains && settings.SubPublicURL == "" && !managedOrigin {
		return fmt.Errorf("restricted subscriptions require a public subscription URL")
	}
	if settings.SubPublicURL != "" {
		u, err := url.Parse(settings.SubPublicURL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return fmt.Errorf("subscription public URL must be an HTTP/HTTPS origin without a path, credentials, query or fragment")
		}
		if port := u.Port(); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("subscription public port must be between 1 and 65535")
			}
		}
		if _, err := netsafe.NormalizeHost(u.Hostname()); err != nil {
			return fmt.Errorf("invalid subscription public hostname: %w", err)
		}
		if settings.SubCDNEnable && u.Scheme != "https" {
			return fmt.Errorf("CDN subscription public URL must use HTTPS")
		}
		settings.SubPublicURL = strings.TrimRight(u.String(), "/")
	}
	if settings.SubCDNEnable && settings.SubPublicURL == "" {
		return fmt.Errorf("CDN mode requires a subscription public URL")
	}
	var hosts []string
	for _, host := range strings.FieldsFunc(settings.SubAllowedDomains, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == ';' }) {
		host = strings.TrimSuffix(strings.ToLower(host), ".")
		if net.ParseIP(host) != nil {
			return fmt.Errorf("subscription aliases must be domain names")
		}
		if _, err := netsafe.NormalizeHost(host); err != nil {
			return fmt.Errorf("invalid subscription alias: %w", err)
		}
		hosts = append(hosts, host)
	}
	settings.SubAllowedDomains = strings.Join(hosts, ",")
	return nil
}
