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

func TestTranslateXDriveYandexWebDAVToNativeSingBox(t *testing.T) {
	raw := xdriveInbound()
	template := map[string]any{"flatten": true}
	template["auth"] = map[string]any{"type": "basic", "username": "{secret0}", "password": "{secret1}"}
	template["put"] = map[string]any{"method": "PUT", "url": "https://webdav.yandex.ru/{folder}/{name}"}
	template["get"] = map[string]any{"method": "GET", "url": "https://webdav.yandex.ru/{folder}/{name}"}
	template["list"] = map[string]any{"method": "PROPFIND", "url": "https://webdav.yandex.ru/{folder}/", "namesRegex": "<d:href>[^<]*/([^/<]+)</d:href>"}
	template["delete"] = map[string]any{"method": "DELETE", "url": "https://webdav.yandex.ru/{folder}/{name}"}
	settings := map[string]any{"service": "template", "remoteFolder": "xdrive", "secrets": []any{"dedicated-user", "app-password"}, "template": template}
	raw["streamSettings"] = map[string]any{"network": "xdrive", "security": "none", "xdriveSettings": settings}

	out, err := TranslateXrayInbound(raw)
	if err != nil {
		t.Fatalf("Yandex WebDAV preset must translate to native sing-box: %v", err)
	}
	transport := out["transport"].(map[string]any)
	if transport["type"] != "xdrive" || transport["service"] != "template" || transport["remote_folder"] != "xdrive" {
		t.Fatalf("native sing-box XDRIVE transport lost fields: %#v", transport)
	}
	config := transport["template"].(string)
	if !strings.Contains(config, "webdav.yandex.ru") || !strings.Contains(config, "PROPFIND") {
		t.Fatalf("Yandex WebDAV template missing from native sing-box: %s", config)
	}
	if out["listen_port"] != nil {
		t.Fatal("Yandex storage transport must not bind an inbound port")
	}
}

func TestTranslateXDriveYandexS3ToNativeSingBox(t *testing.T) {
	raw := xdriveInbound()
	s3 := map[string]any{"endpoint": "https://storage.yandexcloud.net", "region": "ru-central1", "bucket": "xdrive-bucket", "pathStyle": true}
	settings := map[string]any{"service": "s3", "remoteFolder": "xdrive", "secrets": []any{"access-key", "secret-key"}, "s3": s3}
	raw["streamSettings"] = map[string]any{"network": "xdrive", "security": "none", "xdriveSettings": settings}

	out, err := TranslateXrayInbound(raw)
	if err != nil {
		t.Fatalf("Yandex S3 preset must translate to native sing-box: %v", err)
	}
	transport := out["transport"].(map[string]any)
	nativeS3 := transport["s3"].(map[string]any)
	if nativeS3["endpoint"] != "https://storage.yandexcloud.net" || nativeS3["region"] != "ru-central1" || nativeS3["path_style"] != true {
		t.Fatalf("Yandex Object Storage S3 settings changed: %#v", nativeS3)
	}
}
