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
	raw, err := s.getString("subPublicURL")
	if err != nil {
		return "", err
	}
	if raw = strings.TrimSpace(raw); raw != "" {
		return strings.TrimRight(raw, "/"), nil
	}
	cfg, settings, err := s.subscriptionPublishingSettings()
	if err != nil {
		return "", err
	}
	if !cfg.Enabled || cfg.SubscriptionDomain == "" {
		return "", nil
	}
	port := cfg.PublicPort
	if managedSubscriptionListenerDomain(cfg, settings) != "" {
		port = settings.SubPort
	}
	host := cfg.SubscriptionDomain
	if port != 443 {
		host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return "https://" + host, nil
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
	name := cfg.SubscriptionDomain
	if !cfg.Enabled || name == "" || !settings.SubEnable || net.ParseIP(settings.SubListen).IsLoopback() {
		return ""
	}
	for _, d := range cfg.Domains {
		if d.Name != name || !d.Enabled || (d.Mode != "direct" && d.Mode != "sub_link_only") {
			continue
		}
		if raw := strings.TrimSpace(settings.SubPublicURL); raw != "" {
			u, err := url.Parse(raw)
			if err != nil || u.Scheme != "https" || !strings.EqualFold(strings.TrimSuffix(u.Hostname(), "."), name) {
				return ""
			}
			port := 443
			if u.Port() != "" {
				port, _ = strconv.Atoi(u.Port())
			}
			if port == settings.SubPort {
				return name
			}
			return ""
		}
		if cfg.PublicPort == cfg.HTTPSPort && strings.EqualFold(strings.TrimSuffix(settings.SubDomain, "."), name) {
			return name
		}
	}
	return ""
}

func (s *SettingService) GetSubCDNEnable() (bool, error) {
	publicURL, err := s.getString("subPublicURL")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(publicURL) != "" {
		return s.getBool("subCDNEnable")
	}
	cfg, err := s.GetDomainFrontConfig()
	if err != nil {
		return false, err
	}
	if cfg.Enabled && cfg.SubscriptionDomain != "" {
		for _, d := range cfg.Domains {
			if d.Enabled && d.Name == cfg.SubscriptionDomain {
				return d.Mode == "cdn", nil
			}
		}
	}
	return s.getBool("subCDNEnable")
}

func (s *SettingService) GetSubscriptionAllowedHosts(primary string) (map[string]bool, error) {
	hosts := map[string]bool{}
	restricted, err := s.getBool("subRestrictDomains")
	if err != nil {
		return nil, err
	}
	cfg, err := s.GetDomainFrontConfig()
	if err != nil {
		return nil, err
	}
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
	publicURL, err := s.GetSubPublicURL()
	if err != nil {
		return nil, err
	}
	if publicURL != "" {
		u, err := url.Parse(publicURL)
		if err != nil {
			return nil, err
		}
		hosts[strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")] = true
	}
	aliases, err := s.getString("subAllowedDomains")
	if err != nil {
		return nil, err
	}
	for _, host := range strings.FieldsFunc(aliases, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == ';' }) {
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
