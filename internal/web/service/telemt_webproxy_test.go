package service

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
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
	state.ListenPort = 15081
	got, err = appendTelemtWebProxyConfig(base, state)
	if err != nil || !containsTelemtWeb(got, "port = 15081") {
		t.Fatalf("chosen backend port was not written: err=%v config=%s", err, got)
	}
}

func TestRemoveTelemtWebUserPreservesOtherTables(t *testing.T) {
	base := "[access.users]\nwebproxy = \"old-secret\"\nxui = \"user-secret\"\n\n[other.settings]\nwebproxy = \"keep-this-setting\"\n"
	want := "[access.users]\nxui = \"user-secret\"\n\n[other.settings]\nwebproxy = \"keep-this-setting\"\n"
	if got := removeTelemtWebUser(base); got != want {
		t.Fatalf("removing Telemt WEB user modified unrelated settings:\n%s", got)
	}
}

func TestRemoveTelemtWebManagedUsers(t *testing.T) {
	base := "[access.users]\n\"mtweb_12_alice\" = \"old-secret\"\nwebproxy = \"legacy\"\nxui = \"keep\"\n\n[other]\n\"mtweb_12_alice\" = \"keep-setting\"\n"
	want := "[access.users]\nxui = \"keep\"\n\n[other]\n\"mtweb_12_alice\" = \"keep-setting\"\n"
	if got := removeTelemtWebUser(base); got != want {
		t.Fatalf("stale credential or unrelated setting changed: %s", got)
	}
}

func TestTelemtWebSubscriptionLocationsRouteImportedHiddifyPath(t *testing.T) {
	locations := renderTelemtWebSubscriptionLocations([]HiddifyLegacySubscriptionAlias{
		{Path: "BackupPath123"},
		{Path: "../../other"},
	}, "http://127.0.0.1:2096")
	if !strings.Contains(locations, "location ^~ /BackupPath123/") ||
		!strings.Contains(locations, "proxy_pass http://127.0.0.1:2096;") ||
		!strings.Contains(locations, "proxy_set_header Host $host;") {
		t.Fatalf("missing subscription route: %s", locations)
	}
	if strings.Contains(locations, "other") || strings.Contains(locations, "location / {") {
		t.Fatalf("unsafe or overly broad subscription route: %s", locations)
	}
}

func TestPickTelemtWebListenPort(t *testing.T) {
	port, err := pickTelemtWebListenPort(telemtWebListenPort)
	if err != nil || port == telemtWebListenPort {
		t.Fatalf("primary Telemt port must not be reused: port=%d err=%v", port, err)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(telemtWebListenIP, strconv.Itoa(telemtWebListenPort)))
	if err != nil {
		t.Skipf("default WEB Proxy port already occupied: %v", err)
	}
	defer listener.Close()
	port, err = pickTelemtWebListenPort(8443)
	if err != nil || port == telemtWebListenPort {
		t.Fatalf("occupied WEB Proxy port must be skipped: port=%d err=%v", port, err)
	}
}

func TestSelectTelemtWebPublicIP(t *testing.T) {
	dns := []net.IP{net.ParseIP("198.51.100.8"), net.ParseIP("203.0.113.10")}
	local := []net.IP{net.ParseIP("203.0.113.10")}
	if got := selectTelemtWebPublicIP(dns, local); !got.Equal(local[0]) {
		t.Fatalf("expected server address, got %v", got)
	}
	if got := selectTelemtWebPublicIP(dns, nil); !got.Equal(dns[0]) {
		t.Fatalf("expected IPv4 DNS fallback for NAT, got %v", got)
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

func TestTelemtWebCaddyReloadPreservesWebSocketLanes(t *testing.T) {
	for _, tc := range []struct {
		version   string
		supported bool
	}{{"v2.6.2", false}, {"v2.7.0", true}, {"v2.11.9", true}} {
		t.Run(tc.version, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "caddy"), []byte("#!/bin/sh\nprintf '%s\\n' '"+tc.version+"'\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			data, err := telemtWebFrontendConfig(TelemtWebProxyState{Domain: "example.com", Frontend: "caddy", ListenPort: 15080}, telemtWebSubscriptionTarget{})
			if err != nil {
				t.Fatal(err)
			}
			var compact bytes.Buffer
			if err := json.Compact(&compact, data); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(compact.String(), `"stream_close_delay":"5m"`); got != tc.supported {
				t.Fatalf("Caddy %s stream delay=%v want=%v", tc.version, got, tc.supported)
			}
		})
	}
}
