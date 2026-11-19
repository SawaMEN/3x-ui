package externalvpn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

func TestPingtunnelAndTrustTunnelUseCoreEgress(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	ping := Instance{ID: 4, Protocol: model.Pingtunnel, Settings: Settings{Password: "strong-secret", Encrypt: "chacha20", Forward: "http://old.example:80"}}
	args, err := files(ping, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["forward"] != "socks5://"+sidecaregress.Address(4) {
		t.Fatal("pingtunnel bypasses core")
	}
	if config["password"] != "strong-secret" || config["encrypt"] != "chacha20" || config["nolog"] != true {
		t.Fatalf("not a Pingtunnel 3.x server config: %#v", config)
	}
	if _, legacy := config["key"]; legacy {
		t.Fatal("Pingtunnel 3.x rejects legacy key field")
	}
	if _, legacy := config["encrypt_key"]; legacy {
		t.Fatal("Pingtunnel 3.x rejects legacy encrypt_key field")
	}
	if config["conntt"] != float64(3000) {
		t.Fatal("default Pingtunnel connect timeout must be 3000 ms")
	}
	trust := Instance{ID: 5, Protocol: model.TrustTunnel, Port: 2443, Settings: Settings{Hostname: "example.test", Clients: []Client{{Email: "alice", Password: "p", Enable: true}}}}
	args, err = files(trust, "127.0.0.1:19000")
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(args[0])
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := toml.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	forward := settings["forward_protocol"].(map[string]any)
	if forward["direct"] != nil || forward["socks5"].(map[string]any)["address"] != sidecaregress.Address(5) {
		t.Fatal("TrustTunnel bypasses core")
	}
	creds, err := os.ReadFile(filepath.Join(filepath.Dir(args[0]), "credentials.toml"))
	if err != nil || !strings.Contains(string(creds), "alice") {
		t.Fatal("lost client authentication")
	}
}

func TestPingtunnelRejectsVersionsWithDirectUDPFallback(t *testing.T) {
	for _, version := range []string{"2.8", "3.0", "3.1"} {
		path := filepath.Join(t.TempDir(), "pingtunnel")
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'pingtunnel version "+version+"'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		err := requirePingtunnelForwardVersion(path)
		if (err == nil) != (version != "2.8") {
			t.Fatalf("version %s: %v", version, err)
		}
	}
}
