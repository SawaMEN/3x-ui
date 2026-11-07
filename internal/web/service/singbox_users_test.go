package service

import (
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestSingBoxInboundRequiresUsers(t *testing.T) {
	tests := []struct {
		name     string
		protocol model.Protocol
		want     bool
	}{
		{name: "vless", protocol: model.VLESS, want: true},
		{name: "vmess", protocol: model.VMESS, want: true},
		{name: "trojan", protocol: model.Trojan, want: true},
		{name: "masque", protocol: model.MASQUE, want: true},
		{name: "naive", protocol: model.NaiveProxy, want: true},
		{name: "hysteria", protocol: model.Hysteria, want: true},
		{name: "shadowtls", protocol: model.ShadowTLS, want: true},
		{name: "anytls", protocol: model.AnyTLS, want: true},
		{name: "tuic", protocol: model.TUIC, want: true},
		{name: "shadowsocks", protocol: model.Shadowsocks, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := singBoxInboundRequiresUsers(tt.protocol); got != tt.want {
				t.Fatalf("singBoxInboundRequiresUsers(%v) = %v, want %v", tt.protocol, got, tt.want)
			}
		})
	}
}

func TestSingBoxExcludesLocalSidecarInbounds(t *testing.T) {
	for _, protocol := range []model.Protocol{
		model.MTProto, model.AmneziaWG, model.Mieru,
		model.Pingtunnel, model.TrustTunnel, model.Sudoku, model.VKTurnProxy,
	} {
		if !isLocalSidecarInbound(protocol) {
			t.Errorf("%s must be handled by its local sidecar", protocol)
		}
	}
	for _, protocol := range []model.Protocol{
		model.VLESS, model.VMESS, model.Trojan, model.Shadowsocks,
		model.AnyTLS, model.ShadowTLS, model.HTTP, model.Mixed, model.TUIC,
	} {
		if isLocalSidecarInbound(protocol) {
			t.Errorf("%s must remain available to sing-box", protocol)
		}
	}
}

func TestSingBoxTUICInboundPreservesUsersAndTLS(t *testing.T) {
	ib := &model.Inbound{
		Tag: "tuic-443", Protocol: model.TUIC, Port: 443,
		Settings: `{"server":{"certificate":"/tls/cert.pem","private_key":"/tls/key.pem","congestion_control":"bbr","alpn":["h3"],"authentication_timeout":5}}`,
	}
	clients := []any{map[string]any{"email": "alice", "uuid": "768e8bdd-bee3-4442-9006-b26464148aaa", "password": "secret"}}
	got, err := singBoxTUICInbound(ib, clients)
	if err != nil {
		t.Fatal(err)
	}
	if got["type"] != "tuic" || got["listen"] != "0.0.0.0" || got["listen_port"] != 443 || got["auth_timeout"] != "5s" {
		t.Fatalf("unexpected TUIC listener: %v", got)
	}
	users := got["users"].([]map[string]any)
	if len(users) != 1 || users[0]["name"] != "alice" || users[0]["uuid"] != clients[0].(map[string]any)["uuid"] {
		t.Fatalf("unexpected TUIC users: %v", users)
	}
	tls := got["tls"].(map[string]any)
	if tls["certificate_path"] != "/tls/cert.pem" || tls["key_path"] != "/tls/key.pem" {
		t.Fatalf("unexpected TUIC TLS: %v", tls)
	}
}

func TestSingBoxTUICInboundPanelTLSFallback(t *testing.T) {
	ib := &model.Inbound{
		Tag: "tuic-no-cert", Protocol: model.TUIC, Port: 8443,
		Settings: `{"server":{"congestion_control":"bbr","alpn":["h3"]}}`,
	}
	original := ib.Settings
	clients := []any{map[string]any{"email": "alice", "uuid": "768e8bdd-bee3-4442-9006-b26464148aaa", "password": "secret"}}
	lookup := func(key string) (string, error) {
		switch key {
		case "webCertFile":
			return "/panel/cert.pem", nil
		case "webKeyFile":
			return "/panel/key.pem", nil
		default:
			return "", nil
		}
	}
	got, err := singBoxTUICInbound(ib, clients, lookup)
	if err != nil {
		t.Fatal(err)
	}
	tls := got["tls"].(map[string]any)
	if tls["certificate_path"] != "/panel/cert.pem" || tls["key_path"] != "/panel/key.pem" {
		t.Fatalf("panel TLS fallback not used: %v", tls)
	}
	if ib.Settings != original {
		t.Fatal("TLS fallback changed the persisted inbound settings")
	}

	// A partial explicitly configured pair must not silently mix private
	// key material with the panel's unrelated certificate.
	ib.Settings = `{"server":{"certificate":"/custom/cert.pem"}}`
	if _, err := singBoxTUICInbound(ib, clients, lookup); err == nil ||
		!strings.Contains(err.Error(), "requires a TLS certificate and private key") {
		t.Fatalf("incomplete explicit TLS pair accepted: %v", err)
	}

	ib.Settings = `{"server":{"certificate":"/custom/cert.pem","private_key":"/custom/key.pem"}}`
	got, err = singBoxTUICInbound(ib, clients, lookup)
	if err != nil {
		t.Fatal(err)
	}
	tls = got["tls"].(map[string]any)
	if tls["certificate_path"] != "/custom/cert.pem" || tls["key_path"] != "/custom/key.pem" {
		t.Fatalf("explicit TUIC TLS identity replaced: %v", tls)
	}
}

func TestMASQUECoreAndPortCompatibility(t *testing.T) {
	if coreSupportsInboundProtocol(CoreTypeXray, model.MASQUE) {
		t.Fatal("MASQUE accepted on Xray")
	}
	if !coreSupportsInboundProtocol(CoreTypeSingBox, model.MASQUE) || isXrayManagedProtocol(model.MASQUE) {
		t.Fatal("incorrect MASQUE runtime")
	}
	for settings, expected := range map[string]transportBits{
		`{}`: transportTCP | transportUDP, `{"version":[3]}`: transportUDP,
		`{"version":[1,2]}`: transportTCP, `{"version":[2,3]}`: transportTCP | transportUDP,
	} {
		if got := inboundTransports(model.MASQUE, "", settings); got != expected {
			t.Errorf("%s: got %v, want %v", settings, got, expected)
		}
	}
}
