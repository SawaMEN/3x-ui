package singbox

import (
	"encoding/json"
	"strings"
	"testing"
)

func xdriveInbound() map[string]any {
	return map[string]any{"protocol": "vless", "tag": "cloud", "port": 443, "settings": map[string]any{"decryption": "mlkem768x25519plus.native.600s.private-key", "clients": []any{map[string]any{"id": "id", "email": "alice"}}}, "streamSettings": map[string]any{"network": "xdrive", "security": "none", "xdriveSettings": map[string]any{"service": "local", "remoteFolder": "/var/lib/xdrive", "pollIntervalMs": float64(50)}}}
}
func TestTranslateXDriveNative(t *testing.T) {
	raw := xdriveInbound()
	out, err := TranslateXrayInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if out["decryption"] == nil {
		t.Fatal("decryption dropped")
	}
	if out["listen_port"] != nil {
		t.Fatal("storage transport must not bind a port")
	}
	tr := out["transport"].(map[string]any)
	if tr["type"] != "xdrive" || tr["remote_folder"] != "/var/lib/xdrive" || tr["poll_interval_ms"] != float64(50) {
		t.Fatalf("transport lost: %#v", tr)
	}
}
func TestTranslateXDriveRejectsInvalid(t *testing.T) {
	for _, kind := range []string{"no-encryption", "wrong-protocol", "tls", "missing-folder", "too-large", "flow"} {
		t.Run(kind, func(t *testing.T) {
			raw := xdriveInbound()
			settings := raw["settings"].(map[string]any)
			stream := raw["streamSettings"].(map[string]any)
			xd := stream["xdriveSettings"].(map[string]any)
			switch kind {
			case "no-encryption":
				settings["decryption"] = "none"
			case "wrong-protocol":
				raw["protocol"] = "vmess"
			case "tls":
				stream["security"] = "tls"
			case "missing-folder":
				xd["remoteFolder"] = ""
			case "too-large":
				xd["segmentBytes"] = float64(16777217)
			case "flow":
				settings["clients"] = []any{map[string]any{"id": "id", "flow": "xtls-rprx-vision"}}
			}
			if _, err := TranslateXrayInbound(raw); err == nil {
				t.Fatal("invalid XDrive config accepted")
			}
		})
	}
}
func TestTranslateXDriveTemplate(t *testing.T) {
	out := map[string]any{"tag": "cloud", "encryption": "key"}
	stream := map[string]any{"network": "xdrive", "xdriveSettings": map[string]any{"service": "template", "remoteFolder": "folder", "template": map[string]any{"put": map[string]any{"url": "https://storage.example/{name}"}}, "secrets": []any{"secret"}}}
	if err := translateXDrive(out, "vless", stream, false); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(out)
	if !strings.Contains(string(data), `\"url\"`) {
		t.Fatalf("template not encoded as native string: %s", data)
	}
}
