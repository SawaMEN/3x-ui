package masque

import (
	"encoding/json"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"testing"
)

func TestPrepareDefaultsAndPreservesCredentials(t *testing.T) {
	ib := &model.Inbound{Protocol: model.MASQUE, Settings: `{"clients":[{"email":"alice","enable":true}]}`}
	if err := Prepare(ib, ""); err != nil {
		t.Fatal(err)
	}
	s, err := Parse(ib.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if s.MTU != 1280 || s.Path != DefaultPath || len(s.Address) != 2 || !s.TLS.Enabled || s.Clients[0].Password == "" {
		t.Fatalf("incomplete defaults: %+v", s)
	}
	previous := ib.Settings
	password := s.Clients[0].Password
	s.Clients[0].Password = ""
	data, _ := json.Marshal(s)
	ib.Settings = string(data)
	if err := Prepare(ib, previous); err != nil {
		t.Fatal(err)
	}
	s, _ = Parse(ib.Settings)
	if s.Clients[0].Password != password {
		t.Fatal("update changed credentials")
	}
}

func TestInvalidSettings(t *testing.T) {
	for _, raw := range []string{`{"version":[4]}`, `{"version":[3,3]}`, `{"address":["bad"]}`, `{"address":["10.0.0.1/24","10.1.0.1/24"]}`, `{"address":["10.0.0.1/32"]}`, `{"mtu":1279}`, `{"advertiseRoutes":["bad"]}`, `{"path":"https://server/"}`, `{"tls":{"certificatePath":"cert"}}`} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestMASQUETemplates(t *testing.T) {
	for _, path := range []string{DefaultPath, "/tunnel", "/tunnel{?target,ipproto}", "/tunnel?address={target}&protocol={ipproto}", "/tunnel?fixed=1{&target,ipproto}"} {
		t.Run(path, func(t *testing.T) {
			data, _ := json.Marshal(map[string]any{"path": path})
			if _, err := Parse(string(data)); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, path := range []string{"/bad path", "/{target", "/{}", "/{unknown}", "/{+target}", "/{target,ipproto}", "/{target*}", "/{target:3}", "/{target}}", "/%zz", "/tunnel#{target}", "/tunnel?{target}"} {
		if err := ValidatePath(path); err == nil {
			t.Errorf("accepted invalid template %q", path)
		}
	}
}

func TestMASQUEBasicUsernames(t *testing.T) {
	for _, email := range []string{"alice:bob", " ", "alice\x7f", "alice\n"} {
		data, _ := json.Marshal(map[string]any{"clients": []model.Client{{Email: email}}})
		if _, err := Parse(string(data)); err == nil {
			t.Errorf("accepted username %q", email)
		}
	}
	if _, err := Parse(`{"clients":[{"email":"alice"},{"email":"ALICE"}]}`); err == nil {
		t.Fatal("accepted duplicate panel identity")
	}
}

func TestMASQUETLSWhitespace(t *testing.T) {
	settings, err := Parse(`{"tls":{"certificatePath":" cert.pem ","keyPath":" key.pem ","serverName":" example.com "}}`)
	if err != nil {
		t.Fatal(err)
	}
	if settings.TLS.CertificatePath != "cert.pem" || settings.TLS.KeyPath != "key.pem" || settings.TLS.ServerName != "example.com" {
		t.Fatalf("unnormalized TLS: %+v", settings.TLS)
	}
}
