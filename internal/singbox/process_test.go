package singbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcessSupportsNativeAPI(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"Unknown", false},
		{"", false},
		{"1.13.9", false},
		{"1.14.0", true},
		{"1.14.1", true},
		{"2.0.0", true},
	}
	for _, tc := range cases {
		p := &Process{version: tc.version}
		if got := p.SupportsNativeAPI(); got != tc.want {
			t.Fatalf("version %q: got %v, want %v", tc.version, got, tc.want)
		}
	}
}

func TestManagedPathsAreAbsolute(t *testing.T) {
	old, had := os.LookupEnv("XUI_BIN_FOLDER")
	defer func() {
		if had {
			_ = os.Setenv("XUI_BIN_FOLDER", old)
		} else {
			_ = os.Unsetenv("XUI_BIN_FOLDER")
		}
	}()
	_ = os.Setenv("XUI_BIN_FOLDER", "bin")
	if !filepath.IsAbs(GetBinaryPath()) || !filepath.IsAbs(GetConfigPath()) {
		t.Fatalf("managed sing-box paths must be absolute: binary=%q config=%q", GetBinaryPath(), GetConfigPath())
	}
}
