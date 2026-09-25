package service

import (
	"strings"
	"testing"
)

func TestAppendTelemtWebProxyConfig(t *testing.T) {
	base := "[access]\nreplay_check_len = 10\n\n[access.users]\nxui = \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"\n\n[[upstreams]]\ntype = \"direct\"\n"
	state := TelemtWebProxyState{
		Enabled:    true,
		Domain:     "example.com",
		Secret:     "0123456789abcdef0123456789abcdef",
		DecoyDir:   "/var/lib/x-ui/telemt-web",
		ListenPort: 15080,
		PublicAddr: "203.0.113.10:443",
	}
	got, err := appendTelemtWebProxyConfig(base, state)
	if err != nil {
		t.Fatalf("appendTelemtWebProxyConfig() error = %v", err)
	}
	for _, want := range []string{
		"webproxy = \"0123456789abcdef0123456789abcdef\"",
		"transport = \"web\"",
		"port = 15080",
		"host = \"example.com\"",
		"public_addr = \"203.0.113.10:443\"",
		"secret_mode = \"dd\"",
	} {
		if !containsTelemtWeb(got, want) {
			t.Fatalf("generated config missing %q:\n%s", want, got)
		}
	}
}

func TestAppendTelemtWebProxyConfigDisabled(t *testing.T) {
	base := "[access.users]\nxui = \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"\n"
	got, err := appendTelemtWebProxyConfig(base, TelemtWebProxyState{})
	if err != nil {
		t.Fatalf("appendTelemtWebProxyConfig() error = %v", err)
	}
	if got != base {
		t.Fatalf("disabled web proxy changed config")
	}
}

func TestTelemtWebPortAvailabilityOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "empty means available", output: "", want: true},
		{name: "blank means available", output: " \n\t", want: true},
		{name: "listener means occupied", output: "LISTEN 0 4096 0.0.0.0:443 0.0.0.0:*", want: false},
	}
	for _, tt := range tests {
		if got := strings.TrimSpace(tt.output) == ""; got != tt.want {
			t.Fatalf("%s: strings.TrimSpace(%q) == empty = %v, want %v", tt.name, tt.output, got, tt.want)
		}
	}
}

func TestTelemtWebVersionAtLeast(t *testing.T) {
	tests := []struct {
		current  string
		required string
		want     bool
	}{
		{"3.5.1", "3.5.1", true},
		{"v3.6.0", "3.5.1", true},
		{"3.5.0", "3.5.1", false},
		{"3.4.10", "3.5.1", false},
	}
	for _, tt := range tests {
		if got := telemtWebVersionAtLeast(tt.current, tt.required); got != tt.want {
			t.Fatalf("telemtWebVersionAtLeast(%q,%q) = %v, want %v", tt.current, tt.required, got, tt.want)
		}
	}
}

func containsTelemtWeb(value, want string) bool {
	for i := 0; i+len(want) <= len(value); i++ {
		if value[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
