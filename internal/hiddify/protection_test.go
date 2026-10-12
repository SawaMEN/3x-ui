package hiddify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestModifiedProtectionExtensions(t *testing.T) {
	raw := vlessInbound()
	settings := object(raw, "settings")
	settings["testseed"] = []any{900, 500, 1000, 256}
	stream := object(raw, "streamSettings")
	stream["network"] = "tcp"
	stream["security"] = "reality"
	seed := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	stream["realitySettings"] = map[string]any{"target": "127.0.0.1:443", "privateKey": base64.RawURLEncoding.EncodeToString(append([]byte{1}, make([]byte, 31)...)), "serverNames": []any{"localhost", "alt.localhost"}, "shortIds": []any{""}, "mldsa65Seed": seed, "minClientVer": "1.8.0", "maxClientVer": "30.0.0", "xver": 2, "limitFallbackUpload": map[string]any{"afterBytes": 100, "bytesPerSec": 200, "burstBytesPerSec": 300}}
	original, _ := json.Marshal(raw)
	out, err := TranslateInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	reality := object(object(out, "tls"), "reality")
	if reality["mldsa65_seed"] != seed || reality["min_client_version"] != "1.8.0" || reality["xver"] != float64(2) || !reflect.DeepEqual(out["testseed"], []uint32{900, 500, 1000, 256}) {
		t.Fatalf("extensions lost: %#v", out)
	}
	checkProtectionConfig(t, "inbounds", out)
	after, _ := json.Marshal(raw)
	if string(original) != string(after) {
		t.Fatal("source mutated")
	}
	user := map[string]any{"id": "11111111-1111-4111-8111-111111111111", "flow": "xtls-rprx-vision", "testpre": 2, "testseed": []any{900, 500, 1000, 256}}
	raw = map[string]any{"protocol": "vless", "tag": "out", "settings": map[string]any{"address": "127.0.0.1", "port": 443, "id": user["id"], "flow": user["flow"], "testpre": user["testpre"], "testseed": user["testseed"]}, "streamSettings": map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{"serverName": "localhost", "publicKey": seed, "mldsa65Verify": base64.RawURLEncoding.EncodeToString(make([]byte, 1952))}}}
	out, err = TranslateOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if out["testpre"] != uint32(2) || object(object(out, "tls"), "reality")["mldsa65_verify"] == nil {
		t.Fatalf("outbound options lost: %#v", out)
	}
	checkProtectionConfig(t, "outbounds", out)
	object(raw, "settings")["testpre"] = 65
	if _, err = TranslateOutbound(raw); err == nil {
		t.Fatal("unbounded pool accepted")
	}
	object(raw, "settings")["testpre"] = 2
	object(raw, "settings")["flow"] = ""
	if _, err = TranslateOutbound(raw); err == nil {
		t.Fatal("preconnect without Vision accepted")
	}
}

func checkProtectionConfig(t *testing.T, direction string, out map[string]any) {
	t.Helper()
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		return
	}
	config := map[string]any{direction: []any{out}, "dns": map[string]any{"servers": []any{map[string]any{"type": "local", "tag": "local"}}}, "route": map[string]any{"default_domain_resolver": "local"}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "check", "-c", path)
	command.Dir = filepath.Dir(binary)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("protection config rejected: %v: %s", err, output)
	}
}
