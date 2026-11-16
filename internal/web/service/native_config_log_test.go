package service

import (
	"bytes"
	"testing"
)

// The modified HiddifyCli rejects log.disable_color: it is a CLI flag,
// not part of the sing-box JSON configuration schema. Stored native
// templates can still contain this legacy field after a panel update.
func TestNativeConfigOmitsUnsupportedLogDisableColor(t *testing.T) {
	for _, tc := range []struct {
		name     string
		core     string
		template string
		level    string
	}{
		{name: "modified-default", core: CoreTypeSingBox, level: "info"},
		{name: "modified-legacy-template", core: CoreTypeSingBox, template: `{"log":{"level":"warn","disable_color":true}}`, level: "warn"},
		{name: "upstream-default", core: CoreTypeSingBoxUpstream, level: "info"},
		{name: "upstream-legacy-template", core: CoreTypeSingBoxUpstream, template: `{"log":{"level":"warn","disable_color":true}}`, level: "warn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupBulkDB(t)
			if tc.template != "" {
				if err := singBoxSettingService.setString(NativeCore(tc.core).templateKey(), tc.template); err != nil {
					t.Fatal(err)
				}
			}

			cfg, err := NativeCore(tc.core).GetConfig()
			if err != nil {
				t.Fatal(err)
			}
			if _, found := cfg.Log["disable_color"]; found {
				t.Fatalf("unsupported log.disable_color included in %s config: %#v", tc.core, cfg.Log)
			}
			if cfg.Log["level"] != tc.level {
				t.Fatalf("log level was changed: got %v, want %q", cfg.Log["level"], tc.level)
			}
			if cfg.Log["disabled"] != false || cfg.Log["timestamp"] != true {
				t.Fatalf("required runtime logging options changed: %#v", cfg.Log)
			}
			data, err := cfg.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte(`"disable_color"`)) {
				t.Fatalf("unsupported log.disable_color leaked into serialized config: %s", data)
			}
		})
	}
}
