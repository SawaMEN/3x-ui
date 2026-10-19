package olcrtc

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeJitsiDefaults(t *testing.T) {
	input := ClientInput{ClientID: "example"}
	if err := normalize(&input); err != nil {
		t.Fatal(err)
	}
	if input.Carrier != "jitsi" || input.Transport != "datachannel" {
		t.Fatalf("unexpected defaults: %+v", input)
	}
	if !strings.HasPrefix(input.RoomID, "https://meet.handyweb.org/olcrtc-") {
		t.Fatalf("unexpected room: %s", input.RoomID)
	}
	if len(input.Key) != 64 {
		t.Fatalf("key should contain 64 hex chars, got %d", len(input.Key))
	}
	if input.DNS != "1.1.1.1:53" {
		t.Fatalf("unexpected DNS: %s", input.DNS)
	}
}
func TestNormalizeRejectsUnsupportedCombinations(t *testing.T) {
	for _, tc := range []struct{ carrier, transport string }{
		{"telemost", "datachannel"},
		{"telemost", "seichannel"},
		{"wbstream", "datachannel"},
		{"other", "datachannel"},
	} {
		t.Run(tc.carrier+"_"+tc.transport, func(t *testing.T) {
			input := ClientInput{ClientID: "test", Carrier: tc.carrier, Transport: tc.transport, RoomID: "room"}
			if err := normalize(&input); err == nil {
				t.Fatalf("expected error for %+v", input)
			}
		})
	}
}
func TestNormalizeRequiresExternalRoom(t *testing.T) {
	for _, provider := range []string{"telemost", "wbstream"} {
		input := ClientInput{ClientID: "test", Carrier: provider}
		if err := normalize(&input); err == nil {
			t.Fatalf("%s must require existing room", provider)
		}
	}
}
func TestNormalizeRejectsBadInputs(t *testing.T) {
	for name, input := range map[string]ClientInput{
		"unsafe client": {ClientID: "../escape", Carrier: "jitsi"},
		"insecure room": {ClientID: "a", Carrier: "jitsi", RoomID: "http://meet.test/room"},
		"incorrect key": {ClientID: "a", Carrier: "jitsi", Key: "abc"},
		"invalid DNS":   {ClientID: "a", Carrier: "jitsi", DNS: "1.1.1.1:70000"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := normalize(&input); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}
func TestClientLink(t *testing.T) {
	var client Client
	client.ID = "user-1234"
	var location Location
	location.Carrier = "jitsi"
	location.Name = "Main"
	location.Transport.Type = "datachannel"
	location.Endpoint.RoomID = "https://meet.handyweb.org/test"
	location.Endpoint.Key = strings.Repeat("a", 64)
	client.Locations = []Location{location}
	link := ClientLink(client)
	expected := "olcrtc://jitsi?datachannel@https://meet.handyweb.org/test#" + strings.Repeat("a", 64) + "$Main"
	if link != expected {
		t.Fatalf("got %q, want %q", link, expected)
	}
}

func TestClientLinkWithPayload(t *testing.T) {
	client := Client{ID: "test"}
	var location Location
	location.Name = "Office"
	location.Carrier = "wbstream"
	location.Transport.Type = "vp8channel"
	location.Transport.Payload = map[string]string{"vp8-batch": "4", "vp8-fps": "30"}
	location.Endpoint.RoomID = "room-id"
	location.Endpoint.Key = strings.Repeat("f", 64)
	client.Locations = []Location{location}
	got := ClientLink(client)
	expected := "olcrtc://wbstream?vp8channel<vp8-batch=4&vp8-fps=30>@room-id#" + strings.Repeat("f", 64) + "$Office"
	if got != expected {
		t.Fatalf("got %q, want %q", got, expected)
	}
}

func TestNormalizeQuotaValidation(t *testing.T) {
	for name, quota := range map[string]Quota{
		"negative speed":   {SpeedMbps: -1},
		"negative traffic": {TrafficGB: -1},
		"invalid date":     {ExpiresAt: "2026-02-30"},
	} {
		t.Run(name, func(t *testing.T) {
			input := ClientInput{ClientID: "example", Quota: &quota}
			if err := normalize(&input); err == nil {
				t.Fatalf("expected invalid quota to fail: %+v", quota)
			}
		})
	}
	valid := ClientInput{
		ClientID: "example",
		Quota:    &Quota{SpeedMbps: 20, TrafficGB: 100, ExpiresAt: "2026-12-31"},
	}
	if err := normalize(&valid); err != nil {
		t.Fatalf("valid quota rejected: %v", err)
	}
}

func TestDecodeLiveRedactsSecrets(t *testing.T) {
	raw := []byte(`{
		"running_count": 1,
		"peer_count": 2,
		"clients": [{
			"client_id": "alice",
			"locations": [{
				"name": "Main",
				"key": "highly-sensitive-secret",
				"uri": "olcrtc://private-connection",
				"room_id": "private-room",
				"proxy": {"pass": "private-password"},
				"running": true,
				"runtime": {"status": "running", "peer_count": 2}
			}, {
				"running": false,
				"runtime": {"status": "stopped"}
			}]
		}]
	}`)
	result, err := decodeLive(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.PeerCount != 2 || result.RunningCount != 1 || len(result.Clients) != 1 {
		t.Fatalf("unexpected aggregate state: %+v", result)
	}
	c := result.Clients[0]
	if !c.Running || c.FailedCount != 1 || c.LocationCount != 2 || c.PeerCount != 2 {
		t.Fatalf("unexpected client summary: %+v", c)
	}
	response, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"highly-sensitive-secret", "private-room", "private-password", "private-connection"} {
		if strings.Contains(string(response), secret) {
			t.Fatalf("sensitive data leaked in live status: %s", secret)
		}
	}
}
func TestDecodeLiveInvalidJSON(t *testing.T) {
	if _, err := decodeLive([]byte("{")); err == nil {
		t.Fatal("invalid JSON must be rejected")
	}
}

func TestSimpleLocationRejectsAdvancedConfiguration(t *testing.T) {
	loc := Location{}
	loc.Link = "direct"
	loc.Data = "data"
	if !SimpleLocation(loc) {
		t.Fatal("default OlcRTC location must be editable")
	}
	loc.Proxy.Addr = "127.0.0.1"
	loc.Proxy.Port = 1080
	if SimpleLocation(loc) {
		t.Fatal("custom SOCKS5 proxy credentials could be destroyed by simple editor")
	}
	loc.Proxy = SocksProxy{}
	loc.Transport.Payload = map[string]string{"vp8-fps": "20"}
	if SimpleLocation(loc) {
		t.Fatal("custom transport payload could be destroyed by simple editor")
	}
	loc.Transport.Payload = nil
	loc.Link = "bridge"
	if SimpleLocation(loc) {
		t.Fatal("non-default link settings could be destroyed by simple editor")
	}
}

// An upstream installer may print credentials before failing; never leak them.
func TestInstallerFailureExit127RedactsCredentials(t *testing.T) {
	exitError := exec.Command("sh", "-c", "exit 127").Run()
	if exitError == nil {
		t.Fatal("expected exit status 127")
	}
	logFile := filepath.Join(t.TempDir(), "installer.log")
	if err := os.WriteFile(logFile, []byte("/tmp/install.sh: line 174: go: command not found\nPassword: private-admin-password\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := installerFailure(exitError, logFile).Error()
	if !strings.Contains(got, "go") || !strings.Contains(got, "127") || strings.Contains(got, "private-admin-password") {
		t.Fatalf("unexpected installer diagnostic: %s", got)
	}
	if err := os.WriteFile(logFile, []byte("/tmp/install.sh: line 174: private-admin-password: command not found\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got = installerFailure(exitError, logFile).Error()
	if strings.Contains(got, "private-admin-password") || !strings.Contains(got, "127") {
		t.Fatalf("unsafe installer diagnostic: %s", got)
	}
}

func TestMissingUpstreamRevisionIsNotAdvertisedAsAvailable(t *testing.T) {
	if got := revisionLabel(""); got != "" {
		t.Fatalf("expected an empty label for an unknown revision, got %q", got)
	}
}

func TestInstallerPrerequisitesCoverEarlyCommands(t *testing.T) {
	for _, tc := range []struct{ manager, pkg string }{
		{"apt", "coreutils"}, {"apt", "iproute2"}, {"apt", "gawk"},
		{"pacman", "coreutils"}, {"pacman", "iproute2"}, {"pacman", "gawk"},
	} {
		found := false
		for _, pkg := range installerPrerequisitePackages(tc.manager) {
			if pkg == tc.pkg {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s lacks %s", tc.manager, tc.pkg)
		}
	}
	if installerPrerequisitePackages("unsupported") != nil {
		t.Fatal("unsupported manager has packages")
	}
}
