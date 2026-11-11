package service

import (
	"path/filepath"
	"testing"
)

func TestParseXrayAccessLogConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		want string
		bad  bool
	}{
		{name: "absolute path", data: `{"log":{"access":"/var/log/xray/access.log"}}`, want: "/var/log/xray/access.log"},
		{name: "disabled", data: `{"log":{"access":"none"}}`, want: "none"},
		{name: "missing", data: `{"log":{"loglevel":"warning"}}`, want: ""},
		{name: "malformed", data: `{"log":`, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseXrayAccessLogConfig([]byte(tc.data))
			if (err != nil) != tc.bad {
				t.Fatalf("parse error = %v; expected error: %v", err, tc.bad)
			}
			if !tc.bad && got != tc.want {
				t.Fatalf("log path = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestResolveXrayAccessLogPath(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "bin", "xray.json")
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"./access.log", filepath.Join(filepath.Dir(configPath), "access.log")},
		{"logs/access.log", filepath.Join(filepath.Dir(configPath), "logs", "access.log")},
		{"none", "none"},
		{"", ""},
		{"/var/log/xray/access.log", "/var/log/xray/access.log"},
	} {
		if got := resolveXrayAccessLogPath(tc.input, configPath); got != tc.want {
			t.Errorf("resolveXrayAccessLogPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
