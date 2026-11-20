package externalvpn

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

func TestPingtunnelSettingsValidation(t *testing.T) {
	base := Settings{Password: "a-strong-secret", Encrypt: "chacha20", ConnectTimeout: 3000, Congestion: "bb"}
	for _, tc := range []struct {
		name  string
		edit  func(*Settings)
		valid bool
	}{
		{"encrypted", func(*Settings) {}, true},
		{"legacy shared secret", func(s *Settings) { s.Password, s.EncryptKey = "", "legacy-strong-secret" }, true},
		{"legacy empty shared secret", func(s *Settings) { s.Password, s.EncryptKey = "", "short" }, false},
		{"aes128", func(s *Settings) { s.Encrypt = "aes128" }, true},
		{"aes256", func(s *Settings) { s.Encrypt = "aes256" }, true},
		{"socks5", func(s *Settings) { s.Forward = "socks5://127.0.0.1:1080" }, true},
		{"http", func(s *Settings) { s.Forward = "http://proxy.example:8080" }, true},
		{"no congestion", func(s *Settings) { s.Congestion = "none" }, true},
		{"missing password", func(s *Settings) { s.Password = "" }, false},
		{"short password", func(s *Settings) { s.Password = "short" }, false},
		{"invalid cipher", func(s *Settings) { s.Encrypt = "aes192" }, false},
		{"negative connections", func(s *Settings) { s.MaxConn = -1 }, false},
		{"negative timeout", func(s *Settings) { s.ConnectTimeout = -1 }, false},
		{"invalid congestion", func(s *Settings) { s.Congestion = "cubic" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			tc.edit(&s)
			err := (Instance{Protocol: model.Pingtunnel, Settings: s}).Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("Validate() error = %v, valid = %v", err, tc.valid)
			}
		})
	}
}

func TestPingtunnelClientLinkRoundTrip(t *testing.T) {
	inst := Instance{Protocol: model.Pingtunnel, Settings: Settings{
		Password: "correct-password", Encrypt: "chacha20", Congestion: "bb",
	}}
	link, err := ExportPingtunnelLink(inst, "vpn.example.test", "Pingtunnel ICMP")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "pingtunnel://") {
		t.Fatalf("unexpected native link %q", link)
	}
	cfg, err := DecodePingtunnelLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != "vpn.example.test" || cfg.Password != inst.Settings.Password ||
		!cfg.Socks5 || cfg.Listen != "127.0.0.1:1080" ||
		cfg.Encrypt != "chacha20" || cfg.Congestion != "bb" {
		t.Fatalf("unexpected native client config: %+v", cfg)
	}
	for _, invalid := range []string{"", "0.0.0.0", "[::]", "bad.example:443", "bad/path"} {
		if _, err := ExportPingtunnelLink(inst, invalid, "bad"); err == nil {
			t.Errorf("accepted invalid Pingtunnel endpoint %q", invalid)
		}
	}
}

func TestPingtunnelRejectsNonIPv4Listen(t *testing.T) {
	settings := Settings{Password: "correct-password", Encrypt: "chacha20"}
	for _, listen := range []string{"", "0.0.0.0", "127.0.0.1", "203.0.113.20"} {
		if err := (Instance{Protocol: model.Pingtunnel, Listen: listen, Settings: settings}).Validate(); err != nil {
			t.Errorf("IPv4 listen %q rejected: %v", listen, err)
		}
	}
	for _, listen := range []string{"::", "::1", "2001:db8::1", "[::1]", "vpn.example.org"} {
		if err := (Instance{Protocol: model.Pingtunnel, Listen: listen, Settings: settings}).Validate(); err == nil {
			t.Errorf("unsupported ICMP listen %q accepted", listen)
		}
	}
}

func TestPingtunnelLinkRejectsInvalidEndpointAndPayload(t *testing.T) {
	inst := Instance{Protocol: model.Pingtunnel, Settings: Settings{Password: "correct-password", Encrypt: "aes256", Congestion: "none"}}
	for _, address := range []string{"::1", "2001:db8::1", "127.0.0.1", "localhost", "test.localhost", "vpn..example.org", "bad_server.example.org", "bad/path", "vpn.example.org:443"} {
		if _, err := ExportPingtunnelLink(inst, address, "bad"); err == nil {
			t.Errorf("invalid public server %q accepted", address)
		}
	}
	link, err := ExportPingtunnelLink(inst, "vpn.example.org", "no congestion")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := DecodePingtunnelLink(link)
	if err != nil || cfg.Congestion != "none" {
		t.Fatalf("disabled congestion was not preserved: %+v, %v", cfg, err)
	}
	invalid := []PingtunnelClientConfig{cfg, cfg, cfg}
	invalid[0].Server = "::1"
	invalid[1].Congestion = "invalid"
	invalid[2].Listen = "127.0.0.1:not-a-port"
	for _, broken := range invalid {
		data, err := json.Marshal(broken)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodePingtunnelLink("pingtunnel://" + base64.RawURLEncoding.EncodeToString(data)); err == nil {
			t.Errorf("invalid native config was accepted: %+v", broken)
		}
	}
}

func TestPingtunnelWritesRoutedServerConfig(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	inst := Instance{ID: 173, Protocol: model.Pingtunnel, Listen: "0.0.0.0", Settings: Settings{
		Password: "strong-shared-password", Encrypt: "aes256", MaxConn: 12, ConnectTimeout: 1200,
		Congestion: "none", Forward: "http://bypass.example:8080",
	}}
	args, err := files(inst, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 6 || args[0] != "-c" || args[2] != "-forward" ||
		args[4] != "-congestion" || args[5] != "" {
		t.Fatalf("incorrect Pingtunnel launch flags: %#v", args)
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	wantForward := "socks5://" + sidecaregress.Address(inst.ID)
	if cfg["forward"] != wantForward || args[3] != wantForward {
		t.Fatalf("external relay bypassed sidecar: config=%v flags=%#v", cfg["forward"], args)
	}
	if cfg["password"] != "strong-shared-password" || cfg["encrypt"] != "aes256" ||
		cfg["maxconn"] != float64(12) || cfg["conntt"] != float64(1200) ||
		cfg["icmp_listen"] != "0.0.0.0" || cfg["type"] != "server" {
		t.Fatalf("wrong Pingtunnel server config: %#v", cfg)
	}
	for _, legacy := range []string{"key", "encrypt_key", "encryptKey"} {
		if _, ok := cfg[legacy]; ok {
			t.Fatalf("deprecated Pingtunnel field %q persisted", legacy)
		}
	}
	info, err := os.Stat(args[1])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("server secret file mode: %v", info.Mode())
	}
}
