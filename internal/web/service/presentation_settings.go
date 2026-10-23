package service

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

type SubscriptionAppearance struct {
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
	return SanitizeHTTPURL(raw)
}

func (s *SettingService) GetSubscriptionAppearance() (SubscriptionAppearance, error) {
	var appearance SubscriptionAppearance
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
