package externalvpn

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFPTNBridgeUsesOwnerCoreAndVPNOnlyEgress(t *testing.T) {
	if !strings.Contains(fptnOutboundScript, "/usr/bin/panel-core srun -c") || strings.Contains(fptnOutboundScript, "singbox") {
		t.Fatal("FPTN launches old/client core")
	}
	cfg := fptnBridgeConfig("11111111-1111-4111-8111-111111111111")
	outbound := cfg["outbounds"].([]any)[0].(map[string]any)
	if outbound["bind_interface"] != "fptn0" {
		t.Fatal("dead FPTN tunnel can escape through default interface")
	}
	dns := cfg["dns"].(map[string]any)["servers"].([]any)[0].(map[string]any)
	if dns["detour"] != outbound["tag"] {
		t.Fatal("DNS bypasses FPTN tunnel")
	}
	if binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY"); binary != "" {
		data, _ := json.Marshal(cfg)
		path := filepath.Join(t.TempDir(), "fptn.json")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "check", "-c", path)
		cmd.Dir = filepath.Dir(binary)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("owner core rejects FPTN bridge: %v %s", err, out)
		}
	}
}
