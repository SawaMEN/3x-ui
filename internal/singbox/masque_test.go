package singbox

import (
	"encoding/json"
	"testing"
)

func TestMASQUEEndpoint(t *testing.T) {
	raw := map[string]any{"protocol": "masque", "tag": "masque-443", "port": 443, "settings": map[string]any{"tls": map[string]any{"certificatePath": "cert.pem", "keyPath": "key.pem"}, "clients": []any{map[string]any{"email": "alice", "password": "secret", "enable": true}, map[string]any{"email": "disabled", "password": "other", "enable": false}}}}
	endpoint, err := TranslateMASQUEEndpoint(raw)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint["type"] != "masque-server" || endpoint["system"] != false || endpoint["listen"] != "0.0.0.0" {
		t.Fatalf("wrong endpoint: %#v", endpoint)
	}
	users := endpoint["users"].([]map[string]any)
	if len(users) != 1 || users[0]["username"] != "alice" || users[0]["password"] != "secret" {
		t.Fatalf("wrong users: %#v", users)
	}
	if _, exists := users[0]["name"]; exists {
		t.Fatal("unsupported user name field")
	}
	cfg := NewConfig()
	cfg.Endpoints = append(cfg.Endpoints, endpoint)
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !configUsesProtocol(data, "masque-server") {
		t.Fatal("endpoint not detected")
	}
	if _, err := TranslateXrayInbound(raw); err == nil {
		t.Fatal("endpoint accepted as inbound")
	}
	raw["settings"] = map[string]any{}
	if _, err := TranslateMASQUEEndpoint(raw); err == nil {
		t.Fatal("accepted missing certificate/users")
	}
}

func TestMASQUEProtocolDetection(t *testing.T) {
	data, _ := json.Marshal(map[string]any{"endpoints": []any{map[string]any{"type": "masque-client"}}})
	if !configUsesProtocol(data, "masque-client") {
		t.Fatal("client endpoint not detected")
	}
}

func TestMASQUEEndpointKeepsAdvancedOptions(t *testing.T) {
	endpoint, err := TranslateMASQUEEndpoint(map[string]any{"protocol": "masque", "tag": "masque", "port": 443, "settings": map[string]any{"tls": map[string]any{"certificatePath": "cert.pem", "keyPath": "key.pem"}, "clients": []any{map[string]any{"email": "alice", "password": "secret", "enable": true}}, "advanced": map[string]any{"udp_timeout": "5m", "initial_packet_size": float64(1200), "disable_path_mtu_discovery": true}}})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint["udp_timeout"] != "5m" || endpoint["initial_packet_size"] != float64(1200) || endpoint["disable_path_mtu_discovery"] != true {
		t.Fatalf("lost advanced options: %#v", endpoint)
	}
	if endpoint["system"] != false {
		t.Fatal("advanced options changed network stack")
	}
}

func TestMASQUEAdvancedOptionsFollowHTTPVersions(t *testing.T) {
	for _, version := range []int{1, 2, 3} {
		endpoint, err := TranslateMASQUEEndpoint(map[string]any{"port": 443, "settings": map[string]any{"version": []int{version}, "tls": map[string]any{"certificatePath": "cert.pem", "keyPath": "key.pem"}, "clients": []any{map[string]any{"email": "alice", "password": "secret", "enable": true}}, "advanced": map[string]any{"initial_packet_size": 1200, "disable_path_mtu_discovery": true, "stream_receive_window": "64 MB", "udp_timeout": "5m"}}})
		if err != nil {
			t.Fatal(err)
		}
		_, hasQUIC := endpoint["initial_packet_size"]
		_, hasWindow := endpoint["stream_receive_window"]
		if hasQUIC != (version == 3) || hasWindow != (version >= 2) || endpoint["udp_timeout"] != "5m" {
			t.Fatalf("HTTP/%d incompatible options: %#v", version, endpoint)
		}
	}
}
