package externalvpn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func maxTestInstance() Instance {
	return Instance{ID: 96, Protocol: model.OpenFlux, Port: 443, Settings: Settings{Context: "saved-context", Clients: []Client{{Email: "alice", Password: strings.Repeat("a", 32), Enable: true}}, Transports: []OpenFluxTransport{{Type: "oneme", Token: "server-private-token", UID: "9223372036854775807", Priority: 50}}}}
}

func TestOpenFluxMAXPrivateConfig(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	inst := maxTestInstance()
	if err := inst.Validate(); err != nil {
		t.Fatal(err)
	}
	cmd, cancel, err := GetManager().additionalCommand(inst, "")
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if strings.Contains(strings.Join(cmd.Args, " "), inst.Settings.Transports[0].Token) {
		t.Fatal("MAX token leaked into process arguments")
	}
	path := filepath.Join(directory(), "96", "openflux.conf")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, expected := range []string{"Type = oneme", "Token = server-private-token", "UID = 9223372036854775807"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q", expected)
		}
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm() != 0o600 {
		t.Fatal("MAX config is not private")
	}
	// A client must call the server, while an exit does not need a peer ID.
	rawOutbound := map[string]any{"protocol": "openflux", "settings": map[string]any{"secret": strings.Repeat("a", 32), "context": "saved-context", "transports": []OpenFluxTransport{{Type: "oneme", Token: "client-token", UID: "123"}}}}
	if err := ValidateAdditionalOutbound(rawOutbound); err != nil {
		t.Fatal(err)
	}
	rawOutbound["settings"].(map[string]any)["transports"] = []OpenFluxTransport{{Type: "oneme", Token: "client-token"}}
	if err := ValidateAdditionalOutbound(rawOutbound); err == nil {
		t.Fatal("client without server ID accepted")
	}
	inst.Settings.Transports[0].UID = ""
	if err := inst.Validate(); err != nil {
		t.Fatal("receiving MAX server unnecessarily requires peer ID")
	}
}

func TestOpenFluxMAXValidationAndNoCredentialSharing(t *testing.T) {
	inst := maxTestInstance()
	for _, token := range []string{"", "token\nRole=client", "token;comment", " token", "token#comment"} {
		inst.Settings.Transports[0].Token = token
		if err := inst.Validate(); err == nil {
			t.Fatal("unsafe MAX token accepted")
		}
	}
	inst = maxTestInstance()
	for _, uid := range []string{"0", "-1", "9223372036854775808", "1.0", "123;comment"} {
		inst.Settings.Transports[0].UID = uid
		if err := inst.Validate(); err == nil {
			t.Fatal("unsafe MAX ID accepted")
		}
	}
	inst = maxTestInstance()
	if _, err := ExportAdditionalLink(inst, "alice", "example.org", 443, "MAX"); err == nil {
		t.Fatal("unsupported MAX share link generated")
	}
	inst.Settings.Transports = append(inst.Settings.Transports, OpenFluxTransport{Type: "direct", Priority: 100, Token: "stale-server-token", UID: "123"})
	link, err := ExportAdditionalLink(inst, "alice", "example.org", 443, "MAX + direct")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeOpenFluxLink(link)
	if err != nil || len(decoded.Transports) != 1 || decoded.Transports[0].Type != "direct" {
		t.Fatalf("shareable fallback lost: %+v %v", decoded, err)
	}
	if decoded.Transports[0].Token != "" || decoded.Transports[0].UID != "" {
		t.Fatal("MAX credentials leaked into subscription")
	}
}
