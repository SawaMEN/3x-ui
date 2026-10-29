package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
)

func TestVetroffAutomaticTLSAndCredentials(t *testing.T) {
	setupConflictDB(t)
	cert, key := writeTestCertificate(t)
	svc := &SettingService{}
	if err := svc.SetCertFile(cert); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetKeyFile(key); err != nil {
		t.Fatal(err)
	}
	ib := &model.Inbound{Protocol: model.VetroffProxy, Port: 18443, Settings: `{"clients":[{"email":"alice@example.com","enable":true,"subId":"keep-sub","comment":"keep-comment","expiryTime":1900000000000}]}`}
	if err := prepareExternalVPN(ib, ""); err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hostname, Certificate, PrivateKey string
		Vetroff                           externalvpn.VetroffSettings
		Clients                           []model.Client
	}
	if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Hostname != "golden-fixture.test" || settings.Certificate != cert || settings.PrivateKey != key || !settings.Vetroff.AutoTLS || settings.Vetroff.Transport != "h2-connect" {
		t.Fatalf("automatic TLS/settings failed: %+v", settings)
	}
	if len(settings.Clients) != 1 || len(settings.Clients[0].Password) < 24 || settings.Clients[0].SubID != "keep-sub" || settings.Clients[0].Comment != "keep-comment" || settings.Clients[0].ExpiryTime != 1900000000000 {
		t.Fatalf("client metadata lost: %+v", settings.Clients)
	}
	previous := ib.Settings
	if err := prepareExternalVPN(ib, previous); err != nil {
		t.Fatal(err)
	}
	var repeated externalvpn.Settings
	if err := json.Unmarshal([]byte(ib.Settings), &repeated); err != nil {
		t.Fatal(err)
	}
	if repeated.Clients[0].Password != settings.Clients[0].Password {
		t.Fatal("automatic settings rotated existing credentials")
	}
	ib.Settings = `{"hostname":"wrong.example.com","vetroff":{"autoTLS":true},"clients":[]}`
	if err := prepareExternalVPN(ib, ""); err == nil || !strings.Contains(err.Error(), "no valid matching") {
		t.Fatalf("mismatched TLS selected: %v", err)
	}
}

func TestVetroffH3ListenerConflictsOnActualAddressAndPort(t *testing.T) {
	setupConflictDB(t)
	seedInboundConflict(t, "vetroff-custom-h3", "127.0.0.1", 18443, model.VetroffProxy, "", `{"vetroff":{"http3":true,"h3Listen":"127.0.0.2:19443"}}`)
	for _, row := range []struct {
		listen   string
		port     int
		protocol model.Protocol
		conflict bool
	}{{"127.0.0.2", 19443, model.Hysteria, true}, {"127.0.0.1", 19443, model.Hysteria, false}, {"127.0.0.2", 19443, model.VLESS, false}, {"127.0.0.1", 18443, model.VLESS, true}, {"127.0.0.1", 18443, model.Hysteria, false}} {
		ib := &model.Inbound{Listen: row.listen, Port: row.port, Protocol: row.protocol}
		detail, err := checkPortConflictTx(database.GetDB(), ib, 0)
		if err != nil {
			t.Fatal(err)
		}
		if (detail != nil) != row.conflict {
			t.Fatalf("%s:%d %s conflict=%+v, want %v", row.listen, row.port, row.protocol, detail, row.conflict)
		}
	}
}
