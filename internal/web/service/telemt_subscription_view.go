package service

import (
	"context"
	"net/url"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// SubscriptionProxyView reads an existing personal profile without provisioning users.
// Nodes expose only this subscription's credentials, never their full user list.
func (s TelemtService) SubscriptionProxyView(ctx context.Context, subID, host string) (coremodel.TelegramSubscription, error) {
	var result coremodel.TelegramSubscription
	settings := SettingService{}
	enabled, err := settings.GetTelemtSubscriptionProxyEnable()
	if err != nil || !enabled || !s.IsEnabled() {
		return result, err
	}
	var count int64
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("sub_id = ?", subID).Count(&count).Error; err != nil {
		return result, err
	}
	if count == 0 {
		return result, nil
	}
	telemtSubscriptionProfileMu.Lock()
	data, err := os.ReadFile(telemtConfigPath)
	telemtSubscriptionProfileMu.Unlock()
	if err != nil {
		return result, err
	}
	var cfg struct {
		Server struct {
			Port int `toml:"port"`
		} `toml:"server"`
		General struct {
			Modes struct {
				TLS bool `toml:"tls"`
			} `toml:"modes"`
		} `toml:"general"`
		Access struct {
			Users map[string]string `toml:"users"`
		} `toml:"access"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return result, err
	}
	username := telemtSubscriptionUsername(subID)
	if strings.TrimSpace(cfg.Access.Users[username]) != "" {
		if link, err := telemtGeneratedLinkContext(ctx, username, cfg.General.Modes.TLS); err == nil {
			if u, err := url.Parse(link); err == nil {
				query := u.Query()
				profileHost := query.Get("server")
				if profileHost == "" {
					profileHost = host
					query.Set("server", host)
					u.RawQuery = query.Encode()
				}
				result.Personal = &coremodel.TelegramProxyProfile{
					Host: profileHost, Port: cfg.Server.Port, TLS: cfg.General.Modes.TLS, Link: u.String(),
				}
			}
		}
	}
	if link, err := s.SubscriptionWebProxyLink(); err == nil && link != "" {
		result.WebProxyEnabled = true
		result.WebProxy = link
	}
	result.Enabled = result.Personal != nil || result.WebProxy != ""
	return result, nil
}

// SubscriptionWebProxyLink reads the advertised link without running the costly
// version, port-owner and frontend health checks used by the admin status page.
func (TelemtService) SubscriptionWebProxyLink() (string, error) {
	state, err := readTelemtWebState()
	if err != nil {
		return "", err
	}
	if state.Enabled && state.Domain != "" && state.Secret != "" {
		return telemtWebLink(state.Domain, state.Secret), nil
	}
	return "", nil
}
