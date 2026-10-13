package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNativeImportEveryRegisteredType(t *testing.T) {
	// Stable 3827d3a871dd registry, excluding hinvalid and removed stubs.
	outbounds := []string{"direct", "bridge", "block", "selector", "urltest", "balancer", "socks", "http", "shadowsocks", "snell", "vmess", "trojan", "naive", "tor", "ssh", "shadowtls", "vless", "vless-reverse", "wireguard_legacy", "anytls", "psiphon", "mieru", "sudoku", "openflux", "fptn", "pingtunnel", "xray", "dnstt", "gooserelay", "masque", "trusttunnel", "hysteria", "hysteria2", "tuic", "tailcat"}
	endpoints := []string{"wireguard", "awg", "warp", "tailscale", "openconnect", "openvpn-client", "openvpn-server", "masque-client", "masque-server", "tunnel_client", "tunnel_server"}
	for _, kind := range append(outbounds, endpoints...) {
		t.Run(kind, func(t *testing.T) {
			// Unknown/new settings must survive import unchanged, including nested options.
			original := map[string]any{"type": kind, "tag": "Тест", "future_option": map[string]any{"enabled": true, "value": "123"}, "final_mask": map[string]any{"tcp": []any{map[string]any{"type": "fragment", "settings": map[string]any{"length": "10-100"}}}}}
			raw, _ := json.Marshal(original)
			for _, input := range []string{string(raw), "singbox://" + base64.RawURLEncoding.EncodeToString(raw)} {
				got, err := ParseNativeOutboundImport(input)
				if err != nil {
					t.Fatal(err)
				}
				items := got.Outbounds
				if isNativeImportEndpoint(kind) {
					items = got.Endpoints
				}
				if len(items) != 1 || !reflect.DeepEqual(items[0], original) {
					t.Fatalf("native options lost: %#v", got)
				}
			}
		})
	}
}

func TestNativeImportBatchPreservesDependencies(t *testing.T) {
	raw := `{"outbounds":[{"type":"direct","tag":"exit"},{"type":"vless","tag":"proxy","detour":"exit","encryption":"mlkem768x25519plus.random.1rtt.KEY"}],"endpoints":[{"type":"awg","tag":"tunnel","awg":{"jc":5,"i1":"secret"}}]}`
	got, err := ParseNativeOutboundImport(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Outbounds) != 2 || len(got.Endpoints) != 1 || got.Outbounds[1]["detour"] != "exit" {
		t.Fatal("batch lost connections")
	}
	for _, raw := range []string{`{"type":"hinvalid"}`, `{"outbounds":{}}`, `{}`, `[1]`, `{"outbounds":[]}`, "singbox://not-base64!"} {
		if _, err := ParseNativeOutboundImport(raw); err == nil {
			t.Fatalf("accepted malformed input: %s", raw)
		}
	}
}

func TestNativeImportSelectorObject(t *testing.T) {
	raw := `{"type":"selector","tag":"group","outbounds":["proxy","direct"],"default":"proxy"}`
	got, err := ParseNativeOutboundImport(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Outbounds) != 1 || got.Outbounds[0]["default"] != "proxy" {
		t.Fatal("selector was treated as a full configuration")
	}
}

func TestNativeImportKernelMatrix(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_CONFIG_CHECK_BINARY to the stable owner release")
	}
	uuid := "12345678-1234-1234-1234-123456789abc"
	key := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("K", 32)))
	encrypted := "mlkem768x25519plus.native.0rtt." + key
	vmessData, _ := json.Marshal(map[string]any{"v": "2", "add": "proxy.test", "port": 443, "id": uuid, "aid": 0, "scy": "auto", "net": "xhttp", "type": "auto", "path": "/test"})
	vmess := "vmess://" + base64.StdEncoding.EncodeToString(vmessData)
	mask := url.QueryEscape(`{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","length":"10-100","interval":"1-10"}}]}`)
	for _, tc := range []struct {
		name, link, kind, field string
		want                    any
	}{
		{"vless-encryption", "vless://" + uuid + "@proxy.test:443?encryption=" + encrypted, "vless", "encryption", encrypted},
		{"vless-vision-encryption", "vless://" + uuid + "@proxy.test:443?encryption=" + encrypted + "&flow=xtls-rprx-vision", "vless", "flow", "xtls-rprx-vision"},
		{"vless-kcp", "vless://" + uuid + "@proxy.test:443?type=kcp&headerType=wechat&seed=test&mtu=1350&tti=50", "vless", "transport.seed", "test"},
		{"trojan-kcp", "trojan://password@proxy.test:443?type=kcp&headerType=srtp&seed=test", "trojan", "transport.type", "kcp"},
		{"vmess-xhttp", vmess, "vmess", "transport.type", "xhttp"},
		{"vless-xhttp", "vless://" + uuid + "@proxy.test:443?type=xhttp&path=%2Ftest&mode=auto", "vless", "transport.type", "xhttp"},
		{"vless-masks", "vless://" + uuid + "@proxy.test:443?security=tls&fm=" + mask, "vless", "final_mask.tcp", nil},
		{"trojan-ws", "trojan://password@proxy.test:443?type=ws&path=%2Fsocket", "trojan", "transport.path", "/socket"},
		{"http", "http://user:password@proxy.test:80", "http", "username", "user"},
		{"socks", "socks5://user:password@proxy.test:1080", "socks", "username", "user"},
		{"hy2", "hy2://password@proxy.test:443?sni=cdn.test&obfs=salamander&obfs-password=mask", "hysteria2", "obfs.password", "mask"},
		{"anytls", "anytls://password@proxy.test:443?sni=cdn.test", "anytls", "password", "password"},
		{"tuic", "tuic://" + uuid + ":password@proxy.test:443?congestion_control=bbr", "tuic", "uuid", uuid},
		{"hysteria-range", "hysteria://proxy.test:20000-30000?auth=pass&upmbps=100&downmbps=200&sni=cdn.test", "hysteria", "server_ports", "20000:30000"},
		{"mieru", "mierus://user:password@proxy.test?profile=test&port=443&protocol=TCP&multiplexing=MULTIPLEXING_HIGH&handshake-mode=HANDSHAKE_NO_WAIT", "mieru", "multiplexing", "MULTIPLEXING_HIGH"},
		{"ssh", "ssh://root:password@proxy.test:22", "ssh", "user", "root"},
		{"shadowsocks", "ss://YWVzLTEyOC1nY206cGFzcw@proxy.test:443", "shadowsocks", "password", "pass"},
		{"naive", "naive+https://user:password@proxy.test:443", "naive", "username", "user"},
		{"snell", "snell://secret@proxy.test:443?version=4", "snell", "psk", "secret"},
		{"trusttunnel", "trusttunnel://user:password@proxy.test:443", "trusttunnel", "username", "user"},
		{"pingtunnel", "pingtunnel://proxy.test?key=12345", "pingtunnel", "key", float64(12345)},
		{"typed-native-options", "anytls://password@proxy.test:443?idle_session_timeout=60s&min_idle_session=3&connect_timeout=10s&tls.server_name=cdn.test", "anytls", "idle_session_timeout", "60s"},
		{"native-json-mask", "vless://" + uuid + "@proxy.test:443?final_mask=" + mask, "vless", "final_mask.tcp", nil},
		{"native-overlay", "anytls://password@proxy.test:443?options=" + url.QueryEscape(`{"idle_session_timeout":"45s","tls":{"server_name":"cdn.test"}}`), "anytls", "idle_session_timeout", "45s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseShareWithKernel(context.Background(), tc.link, binary)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Outbounds) != 1 || got.Outbounds[0]["type"] != tc.kind {
				t.Fatalf("unexpected import: %#v", got)
			}
			encoded, _ := json.Marshal(got)
			path := filepath.Join(t.TempDir(), "import.json")
			if err := os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
				t.Fatalf("kernel rejected import: %s", output)
			}
			var field any = got.Outbounds[0]
			for _, part := range strings.Split(tc.field, ".") {
				m, ok := field.(map[string]any)
				if !ok {
					t.Fatalf("missing %s", tc.field)
				}
				field = m[part]
			}
			if tc.want == nil {
				if field == nil {
					t.Fatalf("missing %s", tc.field)
				}
			} else if !reflect.DeepEqual(field, tc.want) {
				t.Fatalf("%s: got %#v want %#v", tc.field, field, tc.want)
			}
		})
	}
}

func TestNativeImportKernelEndpoints(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("requires stable kernel")
	}
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("K", 32)))
	raw := "wg://proxy.test:51820?pk=" + url.QueryEscape(key) + "&peer_pk=" + url.QueryEscape(key) + "&local_address=10.90.0.12%2F32"
	got, err := parseShareWithKernel(context.Background(), raw, binary)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Endpoints) != 1 || len(got.Outbounds) != 0 {
		t.Fatalf("endpoint imported as outbound: %#v", got)
	}
}

func TestNativeShareOptionTypesAndPrecedence(t *testing.T) {
	out := map[string]any{"type": "anytls", "tls": map[string]any{"enabled": true}}
	q := url.Values{"tcp_fast_open": {"true"}, "minIdleSession": {"4"}, "idle_session_timeout": {"30s"}, "tls": {"{\"enabled\":true,\"server_name\":\"base.test\"}"}, "tls.server_name": {"override.test"}}
	if err := applyNativeShareOptions(out, q); err != nil {
		t.Fatal(err)
	}
	if out["tcp_fast_open"] != true || out["min_idle_session"] != float64(4) || out["tls"].(map[string]any)["server_name"] != "override.test" {
		t.Fatalf("typed options lost: %#v", out)
	}
	if err := applyNativeShareOptions(out, url.Values{"tcp_fast_open": {"invalid"}}); err == nil {
		t.Fatal("invalid native bool accepted")
	}
}

func TestNativeImportKernelBatchAdapters(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("requires stable kernel")
	}
	raw := "vless://12345678-1234-1234-1234-123456789abc@proxy.test:443?type=kcp&seed=batch&connect_timeout=7s#proxy -> ssh://root:password@relay.test:22#relay\nanytls://password@proxy.test:443?idle_session_timeout=42s#tls"
	got, err := parseShareWithKernel(context.Background(), raw, binary)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Outbounds) != 3 {
		t.Fatal("lost batch nodes")
	}
	ssh, kcp, tls := got.Outbounds[0], got.Outbounds[1], got.Outbounds[2]
	if ssh["type"] != "ssh" || kcp["detour"] != ssh["tag"] || kcp["connect_timeout"] != "7s" || kcp["transport"].(map[string]any)["seed"] != "batch" || tls["idle_session_timeout"] != "42s" {
		t.Fatalf("lost chain options: %#v", got.Outbounds)
	}
}

func TestNativeImportKernelEncryptionModes(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("requires stable kernel")
	}
	uuid := "12345678-1234-1234-1234-123456789abc"
	key := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("K", 32)))
	for _, mode := range []string{"native", "xorpub", "random"} {
		for _, rtt := range []string{"0rtt", "1rtt"} {
			t.Run(mode+"/"+rtt, func(t *testing.T) {
				encryption := "mlkem768x25519plus." + mode + "." + rtt + "." + key
				parsed, err := parseShareWithKernel(context.Background(), "vless://"+uuid+"@proxy.test:443?encryption="+encryption, binary)
				if err != nil {
					t.Fatal(err)
				}
				if parsed.Outbounds[0]["encryption"] != encryption {
					t.Fatal("VLESS encryption changed")
				}
			})
		}
	}
	for _, method := range []string{"none", "aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "xchacha20-ietf-poly1305", "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305"} {
		t.Run(method, func(t *testing.T) {
			password := "password"
			if strings.HasPrefix(method, "2022-") {
				size := 32
				if strings.Contains(method, "128") {
					size = 16
				}
				password = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("K", size)))
			}
			raw := "ss://" + base64.RawURLEncoding.EncodeToString([]byte(method+":"+password)) + "@proxy.test:443"
			parsed, err := parseShareWithKernel(context.Background(), raw, binary)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Outbounds[0]["method"] != method || parsed.Outbounds[0]["password"] != password {
				t.Fatal("Shadowsocks cipher/key changed")
			}
		})
	}
}
