package snell

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestPrepareGeneratesAndPreservesSnellSecrets(t *testing.T) {
	inbound := &model.Inbound{
		Protocol: model.Snell,
		Settings: `{"version":6,"mode":"default","clients":[{"email":"alice","enable":true}]}`,
	}
	if err := Prepare(inbound, ""); err != nil {
		t.Fatal(err)
	}
	first, err := Parse(inbound.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.PSK) != 64 || len(first.Clients) != 1 || len(first.Clients[0].Password) != 64 {
		t.Fatalf("PSK or client key not seeded: %+v", first)
	}
	previous := inbound.Settings
	inbound.Settings = `{"version":6,"mode":"unshaped","clients":[{"email":"alice","enable":true},{"email":"bob","enable":true}]}`
	if err := Prepare(inbound, previous); err != nil {
		t.Fatal(err)
	}
	second, err := Parse(inbound.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if second.PSK != first.PSK || second.Clients[0].Password != first.Clients[0].Password {
		t.Fatal("editing Snell rotated existing credentials")
	}
	if second.Clients[1].Password == "" || second.Clients[1].Password == second.Clients[0].Password {
		t.Fatal("new Snell user did not receive an independent key")
	}
}

func TestSnellSettingsKeepOnlyApplicableVersionFields(t *testing.T) {
	cases := []struct {
		name      string
		settings  string
		forbidden []string
	}{
		{"v6", `{"version":6,"mode":"default","obfsMode":"none","obfsHost":"old.example"}`, []string{"mode", "obfsMode", "obfsHost"}},
		{"v5", `{"version":5,"mode":"default","obfsMode":"none","obfsHost":"old.example"}`, []string{"mode", "obfsMode", "obfsHost"}},
		{"v5 HTTP", `{"version":5,"mode":"default","obfsMode":"http","obfsHost":"web.example"}`, []string{"mode"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inbound := &model.Inbound{Protocol: model.Snell, Settings: tc.settings}
			if err := Prepare(inbound, ""); err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &fields); err != nil {
				t.Fatal(err)
			}
			for _, key := range tc.forbidden {
				if _, ok := fields[key]; ok {
					t.Fatalf("unused field %q present in %s", key, inbound.Settings)
				}
			}
			if tc.name == "v5 HTTP" && fields["obfsHost"] != "web.example" {
				t.Fatalf("HTTP Host missing in %s", inbound.Settings)
			}
		})
	}
}

func TestSnellVersionSpecificOutbound(t *testing.T) {
	client := model.Client{Email: "alice", Password: "user-secret", Enable: true}
	for _, tc := range []struct {
		version       int
		wantClient    int
		wantParam     string
		unwantedParam string
	}{
		{5, 4, "obfs_mode", "mode"},
		{6, 6, "mode", "obfs_mode"},
	} {
		s := Settings{Version: tc.version, PSK: "test-long-psk-123456", Mode: "default", ObfsMode: "http", ObfsHost: "example.org"}
		out := s.Outbound("example.com", 443, client)
		if out["version"] != tc.wantClient || out["userkey"] != "user-secret" || out["psk"] != s.PSK {
			t.Fatalf("v%d outbound credentials mismatch: %#v", tc.version, out)
		}
		if _, ok := out[tc.wantParam]; !ok {
			t.Fatalf("v%d is missing %s", tc.version, tc.wantParam)
		}
		if _, ok := out[tc.unwantedParam]; ok {
			t.Fatalf("v%d leaked %s", tc.version, tc.unwantedParam)
		}
		if tc.version == 5 && out["obfs_host"] != "example.org" {
			t.Fatalf("v5 client HTTP host missing: %#v", out)
		}
	}
}

func TestSnellRejectsInvalidSecretsAndDuplicateUsers(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
	}{
		{"short v6 PSK", Settings{Version: 6, PSK: "short"}},
		{"control PSK", Settings{Version: 5, PSK: "abc\nxyz"}},
		{"invalid obfs host", Settings{Version: 5, PSK: "abcdef", ObfsHost: "x\r\ny"}},
		{"duplicate name", Settings{Version: 6, PSK: "123456789012", Clients: []model.Client{
			{Email: "Alice", Password: "one", Enable: true},
			{Email: "alice", Password: "two", Enable: true},
		}}},
		{"duplicate user key", Settings{Version: 6, PSK: "123456789012", Clients: []model.Client{
			{Email: "alice", Password: "key", Enable: true},
			{Email: "bob", Password: "key", Enable: true},
		}}},
		{"control in user key", Settings{Version: 6, PSK: "123456789012", Clients: []model.Client{
			{Email: "alice", Password: "bad\nkey", Enable: true},
		}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.settings)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Parse(string(raw)); err == nil {
				t.Fatalf("invalid Snell settings accepted: %s", raw)
			}
		})
	}
	if _, err := Parse(`{"version":6,"psk":"123456789012","mode":"unsafe-raw"}`); err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatal("unsafe-raw must not be enabled by presets or accepted by the panel")
	}
}

func TestSnellRejectsOverlongUserKey(t *testing.T) {
	userKey := strings.Repeat("x", 256)
	_, err := Parse(`{"version":5,"psk":"secret","clients":[{"email":"alice","password":"` + userKey + `","enable":true}]}`)
	if err == nil || !strings.Contains(err.Error(), "255") {
		t.Fatalf("expected invalid userkey length, got %v", err)
	}
}

func TestSnellQuicOnlyForV5(t *testing.T) {
	for _, tt := range []struct {
		version int
		wantOK  bool
	}{
		{version: 5, wantOK: true},
		{version: 6, wantOK: false},
	} {
		settings, err := Parse(fmt.Sprintf(`{"version":%d,"psk":"123456789012","quicProxy":true}`, tt.version))
		if tt.wantOK && (err != nil || !settings.QuicProxy) {
			t.Fatalf("v%d should support QUIC Proxy: %v", tt.version, err)
		}
		if !tt.wantOK && err == nil {
			t.Fatalf("v%d must reject QUIC Proxy", tt.version)
		}
	}
}

func TestSnellQuicClientVersionAndSettings(t *testing.T) {
	active := Settings{Version: 5, PSK: "testing-psk", QuicProxy: true}
	if got := active.ClientVersion(); got != 5 {
		t.Fatalf("QUIC client version = %d, want 5", got)
	}
	plain := Settings{Version: 5, PSK: "testing-psk"}
	if got := plain.ClientVersion(); got != 4 {
		t.Fatalf("compatible client version = %d, want 4", got)
	}
	outbound := active.Outbound("example.com", 443, model.Client{Email: "user", Password: "key"})
	if outbound["version"] != 5 || outbound["userkey"] != "key" {
		t.Fatalf("QUIC outbound lost v5 or userkey: %#v", outbound)
	}
}

func TestPreparePreservesKeyWhenEmailCaseChanges(t *testing.T) {
	previous := `{"version":6,"psk":"123456789012","clients":[{"email":"Alice","password":"existing-key","enable":true}]}`
	inbound := &model.Inbound{Protocol: model.Snell, Settings: `{"version":6,"clients":[{"email":"alice","enable":true}]}`}
	if err := Prepare(inbound, previous); err != nil {
		t.Fatal(err)
	}
	settings, err := Parse(inbound.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Clients[0].Password != "existing-key" {
		t.Fatal("email case change rotated the user key and invalidated subscriptions")
	}
}
