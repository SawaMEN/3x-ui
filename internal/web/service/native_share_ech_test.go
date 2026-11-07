package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNativeImportECHQueryFormats(t *testing.T) {
	cases := []struct {
		name  string
		query url.Values
		want  map[string]any
	}{
		{"uri ECH config", url.Values{"ech": {"AQIDBA"}}, map[string]any{"enabled": true, "config": []string{"AQIDBA"}}},
		{"explicit ECH config", url.Values{"tls.ech": {"AQIDBA"}}, map[string]any{"enabled": true, "config": []string{"AQIDBA"}}},
		{"disable ECH", url.Values{"ech": {"false"}}, map[string]any{"enabled": false}},
		{"ECH JSON", url.Values{"tls.ech": {`{"enabled":true,"query_server_name":"cloud.example"}`}}, map[string]any{"enabled": true, "query_server_name": "cloud.example"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := map[string]any{"type": "anytls", "tls": map[string]any{"enabled": true}}
			if err := applyNativeShareOptions(out, tc.query); err != nil {
				t.Fatal(err)
			}
			got := out["tls"].(map[string]any)["ech"]
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ECH = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestNativeImportECHMergesExistingNativeSettings(t *testing.T) {
	out := map[string]any{"type": "anytls", "tls": map[string]any{"ech": map[string]any{"enabled": true, "query_server_name": "test.example"}}}
	if err := applyNativeShareOptions(out, url.Values{"ech": {"AQIDBA"}}); err != nil {
		t.Fatal(err)
	}
	ech := out["tls"].(map[string]any)["ech"].(map[string]any)
	if ech["query_server_name"] != "test.example" || !reflect.DeepEqual(ech["config"], []string{"AQIDBA"}) {
		t.Fatalf("existing ECH options were lost: %#v", ech)
	}
	if err := applyNativeShareOptions(out, url.Values{"ech": {`{"enabled":`}}); err == nil {
		t.Fatal("accepted malformed ECH object")
	}
}

func echTestConfigList(t *testing.T) string {
	t.Helper()
	pair, err := generateECHKeyPair("ech-public.example")
	if err != nil {
		t.Fatal(err)
	}
	return pair.(map[string]any)["echConfigList"].(string)
}

func TestNodeRouteHysteriaECHNormalizesRawBase64(t *testing.T) {
	encoded := echTestConfigList(t)
	input := map[string]any{
		"type": "hysteria2", "tag": "node-route-1", "server": "node.example",
		"server_port": 443, "password": "password",
		"tls": map[string]any{
			"enabled": true, "server_name": "real.example",
			"ech": map[string]any{"enabled": true, "config": []any{encoded}},
		},
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ParseNativeOutboundShareLink(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if out["tag"] != "node-route-1" || out["type"] != "hysteria2" {
		t.Fatalf("route identity changed: %#v", out)
	}
	tls := out["tls"].(map[string]any)
	if tls["server_name"] != "real.example" || tls["enabled"] != true {
		t.Fatal("ECH normalization changed TLS verification")
	}
	ech := tls["ech"].(map[string]any)
	if ech["enabled"] != true {
		t.Fatal("ECH must remain enabled")
	}
	pemLines, ok := ech["config"].([]string)
	if !ok || len(pemLines) != 1 {
		t.Fatalf("invalid ECH PEM array: %#v", ech["config"])
	}
	block, rest := pem.Decode([]byte(pemLines[0]))
	decoded, _ := base64.StdEncoding.DecodeString(encoded)
	if block == nil || block.Type != "ECH CONFIGS" || len(rest) != 0 ||
		!reflect.DeepEqual(block.Bytes, decoded) {
		t.Fatal("ECHConfigList bytes were lost or wrapped incorrectly")
	}
	// URI parsing and managed node-route normalization can both run.
	if err := normalizeShareOutboundECH(out); err != nil {
		t.Fatal(err)
	}
	if ech["config"].([]string)[0] != pemLines[0] {
		t.Fatal("ECH normalization is not idempotent")
	}
	if binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY"); binary != "" {
		config := map[string]any{"log": map[string]any{"level": "warn"}, "outbounds": []any{out}}
		raw, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "node-ech.json")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
			t.Fatalf("Hiddify core rejected normalized Hysteria2 node route: %v: %s", err, output)
		}
	}
}

func TestNodeRouteHysteriaECHURIWithKernel(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("requires the pinned Hiddify core")
	}
	encoded := echTestConfigList(t)
	uri := "hy2://password@node.example:443?sni=real.example&ech=" + url.QueryEscape(encoded)
	parsed, err := parseShareWithKernel(context.Background(), uri, binary)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Outbounds) != 1 {
		t.Fatalf("unexpected Hysteria URI parse: %#v", parsed)
	}
	out := parsed.Outbounds[0]
	if out["type"] != "hysteria2" {
		t.Fatalf("Hysteria2 type lost: %#v", out)
	}
	tls := out["tls"].(map[string]any)
	ech := tls["ech"].(map[string]any)
	if ech["enabled"] != true {
		t.Fatal("ECH was disabled")
	}
	config, ok := ech["config"].([]string)
	if !ok || len(config) != 1 || !strings.HasPrefix(config[0], "-----BEGIN ECH CONFIGS-----") {
		t.Fatalf("ECH must be PEM before applying config: %#v", ech)
	}
	out["tag"] = "node-route-1"
	fullConfig := map[string]any{"outbounds": []any{out}}
	raw, err := json.Marshal(fullConfig)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "node-route.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
		t.Fatalf("Hiddify core rejected Hysteria2 URL route with ECH: %v: %s", err, output)
	}
}

func TestHysteriaECHNativeURIOptions(t *testing.T) {
	encoded := echTestConfigList(t)
	for _, query := range []url.Values{
		{"ech": {encoded}},
		{"tls.ech": {encoded}},
		{"tls.ech.config": {encoded}, "tls.ech.enabled": {"true"}},
	} {
		ech := map[string]any{"enabled": true}
		tls := map[string]any{"enabled": true, "ech": ech}
		out := map[string]any{"type": "hysteria2", "tls": tls}
		if err := applyNativeShareOptions(out, query); err != nil {
			t.Fatal(err)
		}
		if err := normalizeShareOutboundECH(out); err != nil {
			t.Fatal(err)
		}
		ech := out["tls"].(map[string]any)["ech"].(map[string]any)
		lines, ok := ech["config"].([]string)
		if !ok || len(lines) != 1 || !strings.HasPrefix(lines[0], "-----BEGIN ECH CONFIGS-----") {
			t.Fatalf("native ECH URI option not normalized: %#v", out)
		}
	}
}

func TestHysteriaECHRejectsMalformedInsteadOfDisabling(t *testing.T) {
	for _, value := range []any{
		[]any{"plain-not-base64!"},
		[]any{"-----BEGIN ECH KEYS-----\\nAQID\\n-----END ECH KEYS-----"},
		[]any{"cloudflare-ech.com+https://dns.example/dns-query"},
		[]any{"AQID"},
		[]any{true},
		true,
	} {
		out := map[string]any{
			"tls": map[string]any{"ech": map[string]any{"enabled": true, "config": value}},
		}
		if err := normalizeShareOutboundECH(out); err == nil {
			t.Errorf("invalid ECH material accepted: %#v", value)
		}
		if out["tls"].(map[string]any)["ech"].(map[string]any)["enabled"] != true {
			t.Fatal("invalid ECH must not be silently disabled")
		}
	}
}

func TestHysteriaECHPreservesDNSDiscoveryAndDisabled(t *testing.T) {
	noConfig := map[string]any{"tls": map[string]any{
		"ech": map[string]any{"enabled": true, "query_server_name": "ech.example"},
	}}
	if err := normalizeShareOutboundECH(noConfig); err != nil {
		t.Fatal(err)
	}
	if _, present := noConfig["tls"].(map[string]any)["ech"].(map[string]any)["config"]; present {
		t.Fatal("dynamic ECH acquired an incorrect empty PEM")
	}
	disabled := map[string]any{"tls": map[string]any{
		"ech": map[string]any{"enabled": false, "config": []any{"not PEM"}},
	}}
	if err := normalizeShareOutboundECH(disabled); err != nil {
		t.Fatal(err)
	}
}
