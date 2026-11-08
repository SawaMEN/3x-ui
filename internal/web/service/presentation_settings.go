package service

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

type SubscriptionAppearance struct {
	Theme       string `json:"theme"`
	Enabled     bool   `json:"enabled"`
	Title       string `json:"title"`
	Description string `json:"description"`
	LogoURL     string `json:"logoURL"`
	Accent      string `json:"accent"`
}

func (s *SettingService) GetRootRedirectURL() (string, error) {
	enabled, err := s.getBool("rootRedirectEnable")
	if err != nil || !enabled {
		return "", err
	}
	raw, err := s.getString("rootRedirectURL")
	if err != nil {
		return "", err
	}
	u, err := domainfront.ValidateRootRedirectURL(raw)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *SettingService) GetSubscriptionAppearance() (SubscriptionAppearance, error) {
	var appearance SubscriptionAppearance
	theme, err := s.getString("subAppearanceTheme")
	if err != nil {
		return appearance, err
	}
	appearance.Theme = normalizeSubscriptionTheme(theme)
	enabled, err := s.getBool("subAppearanceEnable")
	if err != nil || !enabled {
		return appearance, err
	}
	appearance.Enabled = true
	for key, target := range map[string]*string{
		"subAppearanceTitle":       &appearance.Title,
		"subAppearanceDescription": &appearance.Description,
		"subAppearanceLogoURL":     &appearance.LogoURL,
		"subAppearanceAccent":      &appearance.Accent,
	} {
		*target, err = s.getString(key)
		if err != nil {
			return SubscriptionAppearance{}, err
		}
	}
	return appearance, nil
}

var appearanceColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validatePresentationSettings(settings *entity.AllSetting) error {
	if strings.TrimSpace(settings.RootRedirectURL) != "" {
		if _, err := domainfront.ValidateRootRedirectURL(settings.RootRedirectURL); err != nil {
			return fmt.Errorf("invalid root redirect URL: %w", err)
		}
		if settings.RootRedirectEnable {
			var names []string
			for _, d := range seedDomainFront(settings).Domains {
				names = append(names, d.Name)
			}
			if err := domainfront.ValidateRootRedirectDomains(settings.RootRedirectURL, names); err != nil {
				return err
			}
		}
	}
	for key, value := range map[string]*string{
		"root redirect URL":     &settings.RootRedirectURL,
		"subscription logo URL": &settings.SubAppearanceLogoURL,
	} {
		clean, err := SanitizeHTTPURL(*value)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", key, err)
		}
		*value = clean
	}
	if settings.RootRedirectEnable && settings.RootRedirectURL == "" {
		return fmt.Errorf("root redirect URL is required")
	}
	settings.SubAppearanceTheme = normalizeSubscriptionTheme(settings.SubAppearanceTheme)
	switch settings.SubAppearanceTheme {
	case "classic", "luna", "underground", "light", "dark", "ultra-dark", "colorful", "blue-gray", "cyberpunk":
	default:
		return fmt.Errorf("unsupported subscription theme")
	}
	settings.SubAppearanceAccent = strings.TrimSpace(settings.SubAppearanceAccent)
	if settings.SubAppearanceAccent == "" {
		settings.SubAppearanceAccent = "#808080"
	}
	if !appearanceColor.MatchString(settings.SubAppearanceAccent) {
		return fmt.Errorf("subscription accent must be a six-digit hex color")
	}
	if utf8.RuneCountInString(settings.SubAppearanceTitle) > 160 || utf8.RuneCountInString(settings.SubAppearanceDescription) > 2000 {
		return fmt.Errorf("subscription title or description is too long")
	}
	return nil
}

func normalizeSubscriptionTheme(theme string) string {
	switch theme {
	case "":
		return "classic"
	case "whitemax":
		return "luna"
	default:
		return theme
	}
}
