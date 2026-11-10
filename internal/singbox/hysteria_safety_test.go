package singbox

import (
	"maps"
	"strings"
	"testing"
)

func hysteriaTestOutbound(protocol string) map[string]any {
	out := map[string]any{
		"type": protocol, "tag": "hy-test", "server": "example.com",
		"server_port": 443, "tls": map[string]any{"enabled": true},
	}
	if protocol == "hysteria2" {
		out["password"] = "secret"
	} else {
		out["auth_str"] = "secret"
		out["up_mbps"] = 100
		out["down_mbps"] = 100
	}
	return out
}

func TestHysteriaNativeOutboundSafetyMatrix(t *testing.T) {
	tests := []struct {
		name      string
		protocol  string
		change    func(map[string]any)
		errorPart string
	}{
		{"v1 valid", "hysteria", nil, ""},
		{"v2 valid", "hysteria2", nil, ""},
		{"v2 gecko valid", "hysteria2", func(out map[string]any) {
			out["obfs"] = map[string]any{"type": "gecko", "password": "obfs", "min_packet_size": 600, "max_packet_size": 1200}
		}, ""},
		{"v2 salamander valid", "hysteria2", func(out map[string]any) {
			out["obfs"] = map[string]any{"type": "salamander", "password": "obfs"}
		}, ""},
		{"v2 valid hopping", "hysteria2", func(out map[string]any) {
			delete(out, "server_port")
			out["server_ports"] = []any{"20000:30000", "40000"}
			out["hop_interval"] = "30s"
		}, ""},
		{"v1 valid hopping", "hysteria", func(out map[string]any) {
			delete(out, "server_port")
			out["server_ports"] = []any{"10000:20000"}
		}, ""},
		{"empty password", "hysteria2", func(out map[string]any) {
			delete(out, "password")
		}, "requires a password"},
		{"bad bbr", "hysteria2", func(out map[string]any) {
			out["bbr_profile"] = "unknown"
		}, "invalid bbr_profile"},
		{"inbound bandwidth switch", "hysteria2", func(out map[string]any) {
			out["ignore_client_bandwidth"] = true
		}, "inbound-only"},
		{"bad port range", "hysteria2", func(out map[string]any) {
			delete(out, "server_port")
			out["server_ports"] = []any{"60000:20000"}
		}, "server_ports range"},
		{"port zero", "hysteria2", func(out map[string]any) {
			delete(out, "server_port")
			out["server_ports"] = []any{"0"}
		}, "invalid server_ports"},
		{"port overflow", "hysteria2", func(out map[string]any) {
			delete(out, "server_port")
			out["server_ports"] = []any{"65536"}
		}, "invalid server_ports"},
		{"malformed port", "hysteria2", func(out map[string]any) {
			delete(out, "server_port")
			out["server_ports"] = []any{443}
		}, "server_ports must contain strings"},
		{"port conflict", "hysteria2", func(out map[string]any) {
			out["server_ports"] = []any{"443:444"}
		}, "cannot combine server_port"},
		{"wrong obfs shape", "hysteria2", func(out map[string]any) {
			out["obfs"] = "secret"
		}, "obfs must be an object"},
		{"missing obfs password", "hysteria2", func(out map[string]any) {
			out["obfs"] = map[string]any{"type": "gecko"}
		}, "requires a password"},
		{"bad obfs type", "hysteria2", func(out map[string]any) {
			out["obfs"] = map[string]any{"type": "unknown", "password": "secret"}
		}, "invalid obfs type"},
		{"reversed gecko sizes", "hysteria2", func(out map[string]any) {
			out["obfs"] = map[string]any{"type": "gecko", "password": "secret", "min_packet_size": 1200, "max_packet_size": 600}
		}, "exceeds max_packet_size"},
		{"negative gecko size", "hysteria2", func(out map[string]any) {
			out["obfs"] = map[string]any{"type": "gecko", "password": "secret", "min_packet_size": -1}
		}, "min_packet_size must be"},
		{"excessive gecko size", "hysteria2", func(out map[string]any) {
			out["obfs"] = map[string]any{"type": "gecko", "password": "secret", "max_packet_size": 3000}
		}, "max_packet_size must be"},
		{"gecko min exceeds default max", "hysteria2", func(out map[string]any) {
			out["obfs"] = map[string]any{"type": "gecko", "password": "secret", "min_packet_size": 1400}
		}, "exceeds max_packet_size"},
		{"gecko max below default min", "hysteria2", func(out map[string]any) {
			out["obfs"] = map[string]any{"type": "gecko", "password": "secret", "max_packet_size": 400}
		}, "exceeds max_packet_size"},
		{"bad native bandwidth", "hysteria2", func(out map[string]any) {
			out["up_mbps"] = -100
		}, "up_mbps must be"},
		{"fractional native bandwidth", "hysteria2", func(out map[string]any) {
			out["down_mbps"] = 10.5
		}, "down_mbps must be"},
		{"QUIC missing TLS", "hysteria2", func(out map[string]any) {
			delete(out, "tls")
		}, "requires TLS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := hysteriaTestOutbound(tt.protocol)
			if tt.change != nil {
				tt.change(out)
			}
			_, err := (&Config{Outbounds: []map[string]any{out}}).Marshal()
			if tt.errorPart == "" {
				if err != nil {
					t.Fatalf("valid %s: %v", tt.name, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.errorPart) {
				t.Fatalf("expected %q, got %v", tt.errorPart, err)
			}
		})
	}
}

func hysteriaTestInbound(version int, hy map[string]any) map[string]any {
	settings := maps.Clone(hy)
	settings["version"] = version
	return map[string]any{
		"protocol": "hysteria", "tag": "hy-in", "port": 443,
		"settings": map[string]any{"clients": []any{map[string]any{"email": "alice", "auth": "secret"}}},
		"streamSettings": map[string]any{
			"network": "hysteria", "security": "tls", "hysteriaSettings": settings,
		},
	}
}

func TestHysteriaCompatibilityRejectsNonQUICTransport(t *testing.T) {
	for _, version := range []int{1, 2} {
		hy := map[string]any{}
		if version == 1 {
			hy["up"], hy["down"] = 100, 100
		}
		in := hysteriaTestInbound(version, hy)
		in["streamSettings"].(map[string]any)["network"] = "ws"
		if _, err := TranslateXrayInbound(in); err == nil || !strings.Contains(err.Error(), "does not support transport") {
			t.Fatalf("Hysteria %d inbound WS: %v", version, err)
		}
		out := map[string]any{
			"protocol": "hysteria", "tag": "hy-out",
			"settings": map[string]any{"address": "example.com", "port": 443},
			"streamSettings": map[string]any{
				"network": "tcp", "hysteriaSettings": maps.Clone(hy),
			},
		}
		out["streamSettings"].(map[string]any)["hysteriaSettings"].(map[string]any)["version"] = version
		if _, err := TranslateXrayOutbound(out); err == nil || !strings.Contains(err.Error(), "does not support transport") {
			t.Fatalf("Hysteria %d outbound TCP: %v", version, err)
		}
	}
}

func TestHysteria2MasqueradeValidationMatrix(t *testing.T) {
	tests := []struct {
		name       string
		masquerade map[string]any
		errorPart  string
	}{
		{"proxy valid", map[string]any{"type": "proxy", "url": "https://example.com"}, ""},
		{"proxy malformed", map[string]any{"type": "proxy", "url": "not-a-url"}, "requires a valid HTTP(S)"},
		{"file valid", map[string]any{"type": "file", "dir": "/var/www/html"}, ""},
		{"file missing directory", map[string]any{"type": "file"}, "requires a directory"},
		{"string valid", map[string]any{
			"type": "string", "statusCode": 404, "content": "Not found",
			"headers": map[string]any{"Content-Type": "text/plain", "X-Test": []any{"one", "two"}},
		}, ""},
		{"status invalid", map[string]any{"type": "string", "statusCode": 42}, "status_code must be"},
		{"status overflow", map[string]any{"type": "string", "statusCode": 700}, "status_code must be"},
		{"header invalid", map[string]any{"type": "string", "headers": map[string]any{"Content-Type": 123}}, "invalid value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, err := TranslateXrayInbound(hysteriaTestInbound(2, map[string]any{"masquerade": tt.masquerade}))
			if tt.errorPart != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errorPart) {
					t.Fatalf("expected %q, got %v", tt.errorPart, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.name == "string valid" {
				m := in["masquerade"].(map[string]any)
				headers := m["headers"].(map[string][]string)
				if len(headers["Content-Type"]) != 1 || headers["Content-Type"][0] != "text/plain" || len(headers["X-Test"]) != 2 {
					t.Fatalf("headers not normalized to sing-box HTTPHeader: %#v", headers)
				}
			}
		})
	}
}

func TestHysteria1RejectsIncompatibleSettings(t *testing.T) {
	tests := []struct {
		name      string
		options   map[string]any
		errorPart string
	}{
		{"v2 obfs object", map[string]any{"obfs": map[string]any{"type": "salamander", "password": "x"}}, "obfs must be a string"},
		{"v2 masquerade", map[string]any{"masquerade": map[string]any{"type": "file", "dir": "/tmp"}}, "does not support masquerade"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := maps.Clone(tt.options)
			options["up"], options["down"] = 100, 100
			_, err := TranslateXrayInbound(hysteriaTestInbound(1, options))
			if err == nil || !strings.Contains(err.Error(), tt.errorPart) {
				t.Fatalf("expected %q, got %v", tt.errorPart, err)
			}
		})
	}
}

func TestTranslateHysteriaQUICOptionAliases(t *testing.T) {
	for _, version := range []int{1, 2} {
		options := map[string]any{
			"idleTimeout":               "45s",
			"keep_alive_period":         "12s",
			"streamReceiveWindow":       "64 MB",
			"connection_receive_window": "128 MB",
			"maxConcurrentStreams":      32,
			"initialPacketSize":         1252,
			"disablePathMtuDiscovery":   true,
		}
		if version == 1 {
			options["up"], options["down"] = 100, 100
		} else {
			options["brutalDebug"] = true
		}
		in, err := TranslateXrayInbound(hysteriaTestInbound(version, options))
		if err != nil {
			t.Fatalf("version %d: %v", version, err)
		}
		for key, expected := range map[string]any{
			"idle_timeout":               "45s",
			"keep_alive_period":          "12s",
			"stream_receive_window":      "64 MB",
			"connection_receive_window":  "128 MB",
			"max_concurrent_streams":     32,
			"initial_packet_size":        1252,
			"disable_path_mtu_discovery": true,
		} {
			if got := in[key]; got != expected {
				t.Errorf("v%d %s = %#v, expected %#v", version, key, got, expected)
			}
		}
		if version == 2 && in["brutal_debug"] != true {
			t.Fatal("Hysteria2 brutal_debug option was lost")
		}
	}
}

func TestTranslateHysteriaRejectsInvalidQUICOptions(t *testing.T) {
	for _, tt := range []struct {
		name      string
		settings  map[string]any
		errorPart string
	}{
		{"bad duration", map[string]any{"idleTimeout": "banana"}, "invalid idle_timeout"},
		{"bad duration type", map[string]any{"keepAlivePeriod": 15}, "must be a Go duration"},
		{"bad stream count", map[string]any{"maxConcurrentStreams": -5}, "max_concurrent_streams must be"},
		{"bad packet size", map[string]any{"initialPacketSize": 90000}, "initial_packet_size must be"},
		{"bad mtu switch", map[string]any{"disablePathMtuDiscovery": "yes"}, "must be a boolean"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := TranslateXrayInbound(hysteriaTestInbound(2, tt.settings))
			if err == nil || !strings.Contains(err.Error(), tt.errorPart) {
				t.Fatalf("expected %q, got %v", tt.errorPart, err)
			}
		})
	}
}

func TestHysteria2HopIntervalValidation(t *testing.T) {
	for _, tt := range []struct {
		name      string
		interval  string
		max       string
		errorPart string
	}{
		{"valid range", "30s", "1m", ""},
		{"max before min", "30s", "10s", "must not be shorter"},
		{"malformed duration", "30bananas", "1m", "positive duration"},
		{"negative", "-1s", "1m", "positive duration"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := hysteriaTestOutbound("hysteria2")
			out["hop_interval"] = tt.interval
			out["hop_interval_max"] = tt.max
			_, err := (&Config{Outbounds: []map[string]any{out}}).Marshal()
			if tt.errorPart == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.errorPart) {
				t.Fatalf("expected %q, got %v", tt.errorPart, err)
			}
		})
	}
}

func TestHysteriaOutboundNetworkOption(t *testing.T) {
	for _, version := range []int{1, 2} {
		settings := map[string]any{"version": version, "auth": "secret", "network": "udp"}
		if version == 1 {
			settings["up"], settings["down"] = 100, 100
		}
		out, err := TranslateXrayOutbound(map[string]any{
			"protocol": "hysteria", "tag": "hy-udp",
			"settings": map[string]any{"address": "example.com", "port": 443},
			"streamSettings": map[string]any{
				"network": "hysteria", "security": "tls",
				"hysteriaSettings": settings,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if out["network"] != "udp" {
			t.Fatalf("version %d lost network option: %#v", version, out["network"])
		}
	}
}

func TestHysteria2RealmValidation(t *testing.T) {
	for _, tt := range []struct {
		name      string
		mutate    func(map[string]any)
		errorPart string
	}{
		{"valid realm", nil, ""},
		{"invalid scheme", func(r map[string]any) { r["server_url"] = "ssh://realm.example.com" }, "valid HTTP(S)"},
		{"missing realm id", func(r map[string]any) { r["realm_id"] = "" }, "realm_id"},
		{"invalid realm id", func(r map[string]any) { r["realm_id"] = "!broken" }, "realm_id"},
		{"empty stun server", func(r map[string]any) { r["stun_servers"] = []any{""} }, "empty or malformed"},
		{"invalid stun item", func(r map[string]any) { r["stun_servers"] = []any{1} }, "must be strings"},
		{"invalid ip_version", func(r map[string]any) { r["ip_version"] = 7 }, "ip_version"},
		{"IPv6 incompatible mapping", func(r map[string]any) {
			r["ip_version"] = 6
			r["port_mapping"] = map[string]any{"enabled": true}
		}, "requires IPv4"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			realm := map[string]any{
				"server_url":   "https://realm.example.com",
				"realm_id":     "site_01",
				"stun_servers": []any{"stun.example.com:3478"},
			}
			if tt.mutate != nil {
				tt.mutate(realm)
			}
			out := hysteriaTestOutbound("hysteria2")
			delete(out, "server")
			delete(out, "server_port")
			out["realm"] = realm
			_, err := (&Config{Outbounds: []map[string]any{out}}).Marshal()
			if tt.errorPart == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.errorPart) {
				t.Fatalf("expected %q, got %v", tt.errorPart, err)
			}
		})
	}
}

func TestTranslateHysteria2RejectsInvalidUserPasswords(t *testing.T) {
	tests := []struct {
		name      string
		clients   []any
		errorPart string
	}{
		{"valid", []any{map[string]any{"email": "alice", "auth": "alice-token"}, map[string]any{"email": "bob", "auth": "bob-token"}}, ""},
		{"empty password", []any{map[string]any{"email": "alice", "auth": ""}}, "requires a non-empty authentication password"},
		{"missing password", []any{map[string]any{"email": "alice"}}, "requires a non-empty authentication password"},
		{"duplicate password", []any{map[string]any{"email": "alice", "auth": "shared"}, map[string]any{"email": "bob", "auth": "shared"}}, "duplicates the authentication password"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := hysteriaTestInbound(2, map[string]any{})
			raw["settings"] = map[string]any{"clients": tt.clients}
			_, err := TranslateXrayInbound(raw)
			if tt.errorPart == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.errorPart) {
				t.Fatalf("expected %q, got %v", tt.errorPart, err)
			}
		})
	}
}

func TestHysteria2LegacyInboundTCPPlaceholder(t *testing.T) {
	raw := hysteriaTestInbound(2, map[string]any{})
	raw["streamSettings"].(map[string]any)["network"] = "tcp"
	out, err := TranslateXrayInbound(raw)
	if err != nil {
		t.Fatalf("legacy default TCP placeholder should not break Hysteria2 QUIC: %v", err)
	}
	if out["type"] != "hysteria2" {
		t.Fatalf("expected Hysteria2 listener, got %#v", out["type"])
	}
	if _, exists := out["transport"]; exists {
		t.Fatalf("legacy TCP placeholder must not create a stream transport: %#v", out["transport"])
	}
}
