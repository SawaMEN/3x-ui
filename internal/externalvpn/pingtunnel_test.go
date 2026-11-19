package externalvpn

import (
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
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
