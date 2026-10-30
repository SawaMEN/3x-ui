package externalvpn

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func vetroffOutboundTestLink(raw string) string {
	return "vetroff://" + base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func vetroffOutboundTestConfig(link string, enableUDP bool) map[string]any {
	return map[string]any{
		"tag":      "vetroff-out",
		"protocol": "vetroff",
		"settings": map[string]any{"link": link, "enableUDP": enableUDP, "udpStream": enableUDP},
	}
}

func TestVetroffManagedOutboundProfileValidation(t *testing.T) {
	valid := `{"v":1,"servers":["https://vpn.example.com:443"],"username":"alice","password":"a-unique-secret","transport":"h2-connect"}`
	link := vetroffOutboundTestLink(valid)
	if err := ValidateAdditionalOutbound(vetroffOutboundTestConfig(link, true)); err != nil {
		t.Fatalf("valid Vetroff profile rejected: %v", err)
	}
	testCases := map[string]string{
		"bad scheme":        "https://vpn.example.com",
		"bad version":       vetroffOutboundTestLink(strings.Replace(valid, `"v":1`, `"v":2`, 1)),
		"plaintext server":  vetroffOutboundTestLink(strings.Replace(valid, "https://", "http://", 1)),
		"duplicate field":   vetroffOutboundTestLink(strings.Replace(valid, `"v":1`, `"v":1,"v":1`, 1)),
		"unknown field":     vetroffOutboundTestLink(strings.Replace(valid, `"v":1`, `"v":1,"skip_tls_verify":true`, 1)),
		"no password":       vetroffOutboundTestLink(strings.Replace(valid, `"password":"a-unique-secret"`, `"password":""`, 1)),
		"invalid transport": vetroffOutboundTestLink(strings.Replace(valid, `"h2-connect"`, `"unknown"`, 1)),
	}
	for name, candidate := range testCases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateAdditionalOutbound(vetroffOutboundTestConfig(candidate, false)); err == nil {
				t.Fatal("malformed/untrusted Vetroff profile accepted")
			}
		})
	}
	bond := vetroffOutboundTestLink(`{"v":1,"servers":["https://a.example","https://b.example"],"username":"alice","password":"secret","bond":true}`)
	if err := ValidateAdditionalOutbound(vetroffOutboundTestConfig(bond, true)); err == nil {
		t.Fatal("UDP with Bond silently accepted")
	}
	invalidUDP := vetroffOutboundTestConfig(link, false)
	invalidUDP["settings"].(map[string]any)["udpStream"] = true
	if err := ValidateAdditionalOutbound(invalidUDP); err == nil {
		t.Fatal("UDP stream without UDP accepted")
	}
}

func TestVetroffManagedOutboundSidecarWiring(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("shell-based sidecar smoke test requires Linux")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	bundle := filepath.Join(dir, "vetroff-bundle", "vetroff-necko")
	if err := os.MkdirAll(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	// The stub records CLI arguments and stays alive; the actual Vetroff
	// protocol handshake is tested upstream, not simulated here.
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$PWD/argv.txt\"\nexec sleep 30\n"
	if err := os.WriteFile(filepath.Join(bundle, "vetroff"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	oldPlans, oldProcesses := outboundPlans, outboundProcesses
	outboundPlans = map[string]*managedOutbound{}
	outboundProcesses = map[string]*managedOutbound{}
	t.Cleanup(func() {
		KeepOutbounds(nil)
		outboundPlans, outboundProcesses = oldPlans, oldProcesses
	})
	profile := `{"v":1,"servers":["https://vpn.example.com"],"username":"alice","password":"secret"}`
	raw := vetroffOutboundTestConfig(vetroffOutboundTestLink(profile), true)
	if !IsAdditionalOutbound(raw) {
		t.Fatal("Vetroff not recognized as managed outbound")
	}
	planned, err := PlanOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := outboundProcesses["vetroff-out"]; ok {
		t.Fatal("config preview started a sidecar")
	}
	bridge, err := EnsureOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !OutboundsApplied([]map[string]any{raw}) {
		t.Fatal("Vetroff outbound sidecar not tracked")
	}
	b, err := json.Marshal(bridge)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"protocol":"socks"`) ||
		!strings.Contains(string(b), `"address":"127.0.0.1"`) {
		t.Fatalf("managed outbound not bridged to loopback SOCKS: %s", b)
	}
	plannedJSON, _ := json.Marshal(planned)
	if string(b) != string(plannedJSON) {
		t.Fatal("preview bridge address differs from applied address")
	}
	argsPath := filepath.Join(bundle, "argv.txt")
	var args []byte
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		args, err = os.ReadFile(argsPath)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Vetroff CLI was not started: %v", err)
	}
	linkPath := ""
	lines := strings.Split(strings.TrimSpace(string(args)), "\n")
	for i, v := range lines {
		if v == "--link-file" && i+1 < len(lines) {
			linkPath = lines[i+1]
		}
	}
	if len(lines) < 6 || lines[0] != "client" || linkPath == "" ||
		!strings.Contains(string(args), "--listen\n127.0.0.1:") ||
		!strings.Contains(string(args), "--enable-udp\n--udp-stream") {
		t.Fatalf("unexpected Vetroff client flags: %q", string(args))
	}
	info, err := os.Stat(linkPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("connection link file must be owner-only: %v %v", info, err)
	}
	stored, err := os.ReadFile(linkPath)
	if err != nil || strings.TrimSpace(string(stored)) != raw["settings"].(map[string]any)["link"] {
		t.Fatal("client profile file was not written safely")
	}
	RemoveOutboundProtocol("vetroff")
	if _, ok := outboundProcesses["vetroff-out"]; ok {
		t.Fatal("removal left the Vetroff client running")
	}
	if _, ok := outboundPlans["vetroff-out"]; ok {
		t.Fatal("removal left a restart plan")
	}
	if OutboundsApplied([]map[string]any{raw}) {
		t.Fatal("removed client incorrectly reported as applied")
	}
}
