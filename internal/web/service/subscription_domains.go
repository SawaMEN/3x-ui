package service

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

func (s *SettingService) GetSubPublicURL() (string, error) {
	raw, err := s.getString("subPublicURL")
	if err != nil {
		return "", err
	}
	if raw = strings.TrimSpace(raw); raw != "" {
		return strings.TrimRight(raw, "/"), nil
	}
	cfg, err := s.GetDomainFrontConfig()
	if err != nil {
		return "", err
	}
	if cfg.Enabled && cfg.SubscriptionDomain != "" {
		host := cfg.SubscriptionDomain
		if cfg.PublicPort != 443 {
			host = net.JoinHostPort(host, strconv.Itoa(cfg.PublicPort))
		}
		return "https://" + host, nil
	}
	return "", nil
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
	cfg, err := s.GetDomainFrontConfig()
	if err != nil {
		return nil, err
	}
	if cfg.Enabled {
		for _, d := range cfg.Domains {
			if d.Enabled {
				hosts[d.Name] = true
			}
		}
	}
	if primary != "" {
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
		hosts[strings.ToLower(u.Hostname())] = true
	}
	aliases, err := s.getString("subAllowedDomains")
	if err != nil {
		return nil, err
	}
	for _, host := range strings.FieldsFunc(aliases, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == ';' }) {
		hosts[strings.TrimSuffix(strings.ToLower(host), ".")] = true
	}
	return hosts, nil
}

func validateSubscriptionDomains(settings *entity.AllSetting) error {
	settings.SubPublicURL = strings.TrimSpace(settings.SubPublicURL)
	if settings.SubPublicURL != "" {
		u, err := url.Parse(settings.SubPublicURL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return fmt.Errorf("subscription public URL must be an HTTP/HTTPS origin without a path, credentials, query or fragment")
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
