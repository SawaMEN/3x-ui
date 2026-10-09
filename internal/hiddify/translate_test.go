package hiddify

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func vlessInbound() map[string]any {
	return map[string]any{
		"protocol": "vless", "tag": "vless-test", "port": 443, "listen": "127.0.0.1",
		"settings": map[string]any{"decryption": "none", "clients": []any{
			map[string]any{"email": "alice", "id": "11111111-1111-4111-8111-111111111111"},
		}},
		"streamSettings": map[string]any{"network": "xhttp", "xhttpSettings": map[string]any{
			"path": "/audit", "mode": "stream-up", "sessionIDKey": "sid",
			"sessionIDTable": "", "serverMaxHeaderBytes": 0,
			"scMaxEachPostBytes": "", "xPaddingBytes": "100-1000",
			"extra": map[string]any{
				"noSSEHeader": true, "sessionIDKey": "sid", "xPaddingBytes": "100-1000",
				"xmux": map[string]any{"maxConcurrency": "16-32"},
			},
			"enableXmux": true, "xmux": map[string]any{"maxConcurrency": "16-32"},
		}},
	}
}

func TestTranslateEncryptedXHTTPPreservesSourceAndStrictSingBox(t *testing.T) {
	raw := vlessInbound()
	object(raw, "settings")["decryption"] = "mlkem768x25519plus.native.600s.test-key"
	original, _ := json.Marshal(raw)
	got, err := TranslateInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"type": "xhttp", "path": "/audit", "mode": "stream-up", "session_key": "sid",
		"x_padding_bytes": "100-1000", "no_sse_header": true,
		"xmux": map[string]any{"max_concurrency": "16-32"},
	}
	if !reflect.DeepEqual(got["transport"], want) || got["decryption"] != "mlkem768x25519plus.native.600s.test-key" {
		t.Fatalf("extensions lost: %#v", got)
	}
	after, _ := json.Marshal(raw)
	if string(after) != string(original) {
		t.Fatalf("persisted source changed: %s", after)
	}
	if _, err := singbox.TranslateXrayInbound(raw); err == nil || !strings.Contains(err.Error(), "xhttp") {
		t.Fatalf("stock sing-box incorrectly accepts Hiddify XHTTP: %v", err)
	}
}

func TestTranslateVlessOutboundEncryptionAndFinalMask(t *testing.T) {
	raw := map[string]any{
		"protocol": "vless", "tag": "out",
		"settings": map[string]any{"vnext": []any{map[string]any{
			"address": "example.com", "port": 443, "users": []any{map[string]any{
				"id": "11111111-1111-4111-8111-111111111111", "encryption": "mlkem768x25519plus.native.0rtt.test-key",
			}},
		}}},
		"streamSettings": map[string]any{"network": "tcp", "finalmask": map[string]any{
			"tcp": []any{map[string]any{"type": "fragment", "settings": map[string]any{"packets": "tlshello", "length": "10-20"}}},
		}},
	}
	got, err := TranslateOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got["encryption"] != "mlkem768x25519plus.native.0rtt.test-key" || got["final_mask"] == nil {
		t.Fatalf("outbound extensions lost: %#v", got)
	}
	if _, err := singbox.TranslateXrayOutbound(raw); err == nil || !strings.Contains(err.Error(), "encryption") {
		t.Fatalf("stock sing-box incorrectly accepts encrypted VLESS: %v", err)
	}
}

func TestTranslateRejectsProtectionLossAndUnknownXHTTP(t *testing.T) {
	cases := []struct {
		name string
		edit func(map[string]any)
		want string
	}{
		{"ML-DSA", func(r map[string]any) {
			object(r, "streamSettings")["realitySettings"] = map[string]any{"mldsa65Seed": "secret"}
		}, "mldsa65Seed cannot be preserved"},
		{"Vision seed", func(r map[string]any) { object(r, "settings")["testseed"] = []any{1, 2, 3, 4} }, "testseed cannot be preserved"},
		{"new session table", func(r map[string]any) {
			object(object(r, "streamSettings"), "xhttpSettings")["sessionIDTable"] = "BASE36"
		}, "unsupported xhttpSettings.sessionIDTable"},
		{"unknown option", func(r map[string]any) { object(object(r, "streamSettings"), "xhttpSettings")["futureSetting"] = true }, "unsupported xhttpSettings.futureSetting"},
		{"aliases", func(r map[string]any) { object(object(r, "streamSettings"), "xhttpSettings")["sessionKey"] = "other" }, "conflicting XHTTP aliases"},
		{"malformed options", func(r map[string]any) { object(r, "streamSettings")["xhttpSettings"] = "not an object" }, "xhttpSettings must be an object"},
		{"invalid mode", func(r map[string]any) { object(object(r, "streamSettings"), "xhttpSettings")["mode"] = "invalid" }, "invalid XHTTP mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := vlessInbound()
			delete(object(object(raw, "streamSettings"), "xhttpSettings"), "extra")
			tc.edit(raw)
			_, err := TranslateInbound(raw)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestTranslateXHTTPDownload(t *testing.T) {
	raw := map[string]any{
		"protocol": "vless", "tag": "out", "settings": map[string]any{
			"address": "up.example.com", "port": 443, "id": "11111111-1111-4111-8111-111111111111",
		}, "streamSettings": map[string]any{
			"network": "xhttp", "xhttpSettings": map[string]any{
				"path": "/up", "mode": "stream-up", "downloadSettings": map[string]any{
					"address": "down.example.com", "port": 8443, "network": "xhttp", "security": "tls",
					"tlsSettings":   map[string]any{"serverName": "down.example.com"},
					"xhttpSettings": map[string]any{"path": "/down", "mode": "auto", "xPaddingBytes": "100-200"},
				},
			},
		},
	}
	got, err := TranslateOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	download := object(object(got, "transport"), "download")
	if download["server"] != "down.example.com" || download["server_port"] != 8443 || download["path"] != "/down" || object(download, "tls")["server_name"] != "down.example.com" {
		t.Fatalf("download transport lost: %#v", download)
	}
	if download["mode"] != nil || download["uuid"] != nil {
		t.Fatalf("invalid fields leaked into download schema: %#v", download)
	}
}

func TestDownloadRejectsNestedSettingsIncludingExtra(t *testing.T) {
	for _, extra := range []bool{false, true} {
		t.Run(fmt.Sprintf("extra=%v", extra), func(t *testing.T) {
			xhttp := map[string]any{"downloadSettings": map[string]any{
				"address": "nested.example.com", "port": 443, "network": "xhttp",
			}}
			if extra {
				xhttp = map[string]any{"extra": xhttp}
			}
			_, err := translateDownload(map[string]any{
				"address": "down.example.com", "port": 443, "network": "xhttp", "xhttpSettings": xhttp,
			}, "test")
			if err == nil || !strings.Contains(err.Error(), "nested downloadSettings cannot be preserved") {
				t.Fatalf("nested download must fail explicitly: %v", err)
			}
		})
	}
}

func TestDownloadExtraReplacesInactiveRootSettings(t *testing.T) {
	download, err := translateDownload(map[string]any{
		"address": "down.example.com", "port": 443, "network": "xhttp",
		"xhttpSettings": map[string]any{
			"path": "/down", "downloadSettings": map[string]any{"unused": true},
			"extra": map[string]any{"xPaddingBytes": "100-200"},
		},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if download["path"] != "/down" || download["x_padding_bytes"] != "100-200" || download["download"] != nil {
		t.Fatalf("incorrect effective download settings: %#v", download)
	}
}

func TestDownloadRejectsMalformedXHTTPOptions(t *testing.T) {
	for _, xhttp := range []any{"invalid", map[string]any{"extra": "invalid"}} {
		_, err := translateDownload(map[string]any{
			"address": "down.example.com", "port": 443, "network": "xhttp", "xhttpSettings": xhttp,
		}, "test")
		if err == nil || !strings.Contains(err.Error(), "must be an object") {
			t.Fatalf("malformed download options must fail explicitly: %v", err)
		}
	}
}
