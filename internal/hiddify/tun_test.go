package hiddify

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTunTranslationOwnerCore(t *testing.T) {
	raw := map[string]any{"protocol": "tun", "tag": "home-tun", "settings": map[string]any{"name": "hiddify0", "mtu": 1500, "gateway": []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"}, "dns": []string{"1.1.1.1"}, "autoSystemRoutingTable": []string{"0.0.0.0/0", "::/0"}, "autoOutboundsInterface": "auto"}}
	before, _ := json.Marshal(raw)
	got, err := TranslateInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got["type"] != "tun" || got["auto_route"] != true || got["interface_name"] != "hiddify0" || got["dns_mode"] != "native" || got["stack"] != "system" {
		t.Fatalf("mapping: %#v", got)
	}
	after, _ := json.Marshal(raw)
	if string(before) != string(after) {
		t.Fatal("source modified")
	}
	if binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY"); binary != "" {
		cfg := map[string]any{"inbounds": []any{got}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": map[string]any{"auto_detect_interface": true, "final": "direct"}}
		data, _ := json.Marshal(cfg)
		path := filepath.Join(t.TempDir(), "tun.json")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "check", "-c", path)
		cmd.Dir = filepath.Dir(binary)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("owner kernel rejects TUN: %v %s", err, out)
		}
	}
}
func TestTunRejectsUnmappedAndMalformedSettings(t *testing.T) {
	for _, settings := range []map[string]any{
		{"gateway": []any{"bad"}},
		{"gateway": []any{"172.19.0.1/30"}, "dns": []any{"resolver.example"}},
		{"gateway": []any{"172.19.0.1/30"}, "mtu": 1.5},
		{"gateway": []any{"172.19.0.1/30"}, "userLevel": 1},
		{"gateway": []any{"172.19.0.1/30"}, "name": "tun0", "autoOutboundsInterface": "tun0"},
		{"gateway": []any{"172.19.0.1/30"}, "unknownOption": true},
	} {
		if _, err := TranslateInbound(map[string]any{"protocol": "tun", "tag": "test", "settings": settings}); err == nil {
			t.Fatalf("accepted %#v", settings)
		}
	}
}
