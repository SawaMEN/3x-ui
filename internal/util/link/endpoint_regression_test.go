package link

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestLinkSchemesAreCaseInsensitive(t *testing.T) {
	for _, raw := range []string{
		"vless://UserABC@server.example:443#Name",
		"trojan://PasswordABC@server.example:443#Name",
		"hysteria2://AuthABC@server.example:443#Name",
		"wireguard://KeyABC@server.example:51820?publickey=PeerABC#Name",
		"snell://KeyABC@server.example:443?version=4#Name",
	} {
		scheme, rest, _ := strings.Cut(raw, ":")
		t.Run(scheme, func(t *testing.T) {
			original, err := ParseLink(raw)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseLink(strings.ToUpper(scheme) + ":" + rest)
			if err != nil || !reflect.DeepEqual(got, original) {
				t.Fatalf("case-insensitive scheme changed credentials or failed: %v", err)
			}
		})
	}
}

func TestLinkRejectsMissingEndpointAndRequiredCredentials(t *testing.T) {
	cases := []string{
		"vless://user@", "vless://@server.example", "trojan://password@", "trojan://@server.example",
		"hy2://auth@", "wireguard://key@?publickey=peer", "wireguard://@server.example?publickey=peer",
		"wireguard://key@server.example",
		"vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"port":443,"id":"user"}`)),
		"vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"port":443,"add":"server.example"}`)),
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if got, err := ParseLink(raw); err == nil {
				t.Fatalf("incomplete link imported as runtime outbound: %v", got)
			}
		})
	}
}

func TestVmessRejectsMalformedNumericFields(t *testing.T) {
	for _, override := range []map[string]any{{"port": 443.75}, {"aid": 0.5}, {"aid": "not-a-number"}, {"aid": true}} {
		fields := map[string]any{"port": 443, "id": "user", "add": "server.example"}
		for k, v := range override {
			fields[k] = v
		}
		data, _ := json.Marshal(fields)
		if result, err := ParseLink("vmess://" + base64.StdEncoding.EncodeToString(data)); err == nil {
			t.Fatalf("malformed numeric fields %v imported: %v", override, result)
		}
	}
}

func TestVmessPaddedURLSafeBase64(t *testing.T) {
	data := []byte(`{"port":443,"id":"user","add":"server.example","ps":"??🌀"}`)
	encoded := base64.URLEncoding.EncodeToString(data)
	if !strings.ContainsAny(encoded, "-_") || !strings.HasSuffix(encoded, "=") {
		t.Fatal("fixture must contain URL-safe alphabet and padding")
	}
	if _, err := ParseLink("vmess://" + encoded); err != nil {
		t.Fatalf("padded URL-safe VMess subscription entry rejected: %v", err)
	}
}
