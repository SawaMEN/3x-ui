package hiddify

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

// The compiled validator used to predate the selected executable and rejected
// XDrive itself. Check a genuine key and the core's MUX requirements together.
func TestStage5XDriveXrayValidator(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("requires the pinned compatible key generator")
	}
	output, err := exec.Command(binary, "vlessenc", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("key generator: %v", err)
	}
	var keys struct {
		Encryption string `json:"encryption"`
	}
	if err := json.Unmarshal(output, &keys); err != nil || keys.Encryption == "" {
		t.Fatal("invalid key pair", err)
	}
	for _, tc := range []struct {
		name    string
		mux     any
		allowed bool
	}{
		{"missing", nil, false},
		{"disabled", map[string]any{"enabled": false}, false},
		{"negative concurrency", map[string]any{"enabled": true, "concurrency": -1}, false},
		{"skip UDP443", map[string]any{"enabled": true, "xudpProxyUDP443": "skip"}, false},
		{"enabled", map[string]any{"enabled": true, "concurrency": 8, "xudpConcurrency": 8, "xudpProxyUDP443": "reject"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]any{"protocol": "vless", "tag": "storage", "settings": map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": 443, "users": []any{map[string]any{"id": "11111111-1111-4111-8111-111111111111", "encryption": keys.Encryption}}}}}, "streamSettings": map[string]any{"network": "xdrive", "security": "none", "xdriveSettings": map[string]any{"service": "local", "remoteFolder": filepath.Join(t.TempDir(), "storage")}}, "mux": tc.mux}
			data, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			err = xray.ValidateOutboundConfig(data)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v err=%v", tc.allowed, err)
			}
			if !tc.allowed && !strings.Contains(err.Error(), "mux") {
				t.Fatalf("wrong validation refusal: %v", err)
			}
		})
	}
}
