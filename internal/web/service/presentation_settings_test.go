package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

func TestPresentationSettingsRejectUnsafeInput(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings entity.AllSetting
		want     string
	}{
		{"unknown theme", entity.AllSetting{SubAppearanceTheme: "unknown"}, "unsupported subscription theme"},
		{"missing destination", entity.AllSetting{RootRedirectEnable: true}, "root redirect URL is required"},
		{"script destination", entity.AllSetting{RootRedirectURL: "javascript:alert(1)"}, `invalid root redirect URL: unsupported URL scheme "javascript"`},
		{"script logo", entity.AllSetting{SubAppearanceLogoURL: "data:image/svg+xml,test"}, `invalid subscription logo URL: unsupported URL scheme "data"`},
		{"css injection", entity.AllSetting{SubAppearanceAccent: "red;display:none"}, "subscription accent must be a six-digit hex color"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePresentationSettings(&tc.settings)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSubscriptionThemeKeepsClassicForExistingSettings(t *testing.T) {
	settings := entity.AllSetting{SubAppearanceEnable: true}
	if err := validatePresentationSettings(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.SubAppearanceTheme != "classic" {
		t.Fatal("existing branding must retain classic layout")
	}
	settings.SubAppearanceTheme = "whitemax"
	if err := validatePresentationSettings(&settings); err != nil {
		t.Fatal(err)
	}
}
