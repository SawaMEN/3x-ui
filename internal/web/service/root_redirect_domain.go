package service

import (
	"errors"
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

func rootRedirectDomainConfig(cfg domainfront.DomainFrontConfig, settings *entity.AllSetting) domainfront.DomainFrontConfig {
	if !settings.RootRedirectEnable || settings.RootRedirectURL == "" || cfg.Enabled {
		return cfg
	}
	if len(cfg.Domains) == 0 {
		cfg.Domains = seedDomainFront(settings).Domains
	}
	if len(cfg.Domains) > 0 {
		cfg.Enabled = true
		cfg.HTTPPort, cfg.HTTPSPort, cfg.PublicPort = 80, 443, 443
	}
	return cfg
}

func (s *SettingService) GetRootRedirectSettings() (bool, string, error) {
	enabled, err := s.getBool("rootRedirectEnable")
	if err != nil {
		return false, "", err
	}
	target, err := s.getString("rootRedirectURL")
	return enabled, target, err
}

// Apply the domain listener immediately, restoring the saved option if a port
// or certificate prevents the domain-wide redirect from becoming usable.
func (s *SettingService) ReconcileRootRedirect(oldEnabled bool, oldURL string) error {
	enabled, target, err := s.GetRootRedirectSettings()
	if err != nil {
		return err
	}
	cfg, err := s.GetDomainFrontConfig()
	if err == nil && enabled {
		var names []string
		for _, d := range cfg.Domains {
			if d.Enabled {
				names = append(names, d.Name)
			}
		}
		err = domainfront.ValidateRootRedirectDomains(target, names)
	}
	if err == nil && enabled && len(cfg.Domains) == 0 {
		err = fmt.Errorf("для перенаправления домена укажите домен панели или подписки в настройках домена и TLS")
	}
	if err == nil {
		err = StartDomainFront()
	}
	if err == nil {
		return nil
	}
	restoreErr := errors.Join(s.setBool("rootRedirectEnable", oldEnabled), s.setString("rootRedirectURL", oldURL))
	if restoreErr == nil {
		restoreErr = StartDomainFront()
	}
	return errors.Join(err, restoreErr)
}
