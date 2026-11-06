package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

func TestSubscriptionFormatSettingsValidation(t *testing.T) {
	for _, tc := range []struct {
		mode, format string
		valid        bool
	}{
		{"", "", true},
		{"both", "auto", true},
		{"both", "links", true},
		{"both", "xray", true},
		{"xray", "xray", true},
		{"sing-box", "sing-box", true},
		{"xray", "sing-box", false},
		{"sing-box", "links", false},
		{"unknown", "auto", false},
		{"both", "unknown", false},
	} {
		s := &entity.AllSetting{SubFormatMode: tc.mode, SubDefaultFormat: tc.format}
		if err := validateSubscriptionFormats(s); (err == nil) != tc.valid {
			t.Errorf("%+v: %v", tc, err)
		}
	}
}
