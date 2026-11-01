package outbound

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

func TestSingBoxBatchUsesSelectedCore(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	original := newSingBoxBatchProcess
	defer func() { newSingBoxBatchProcess = original }()
	withStubProcess(t, func(_cfg *xray.Config, _path string) batchProcess { t.Fatal("sing-box probe started Xray"); return nil })
	calls := 0
	newSingBoxBatchProcess = func(cfg *singbox.Config, _path string) batchProcess {
		calls++
		if len(cfg.Inbounds) != 1 || cfg.Inbounds[0]["listen"] != "127.0.0.1" {
			t.Fatalf("unsafe test listeners: %v", cfg.Inbounds)
		}
		if cfg.Outbounds[0]["type"] != "ssh" {
			t.Fatalf("wrong translation: %v", cfg.Outbounds)
		}
		return &stubProcess{startErr: errors.New("sing-box error")}
	}
	result, err := (&OutboundService{CoreType: "sing-box"}).TestOutbound(`{"protocol":"ssh","tag":"ssh","settings":{"server":"example.com"}}`, "http://example.invalid", "", "http")
	if err != nil || calls != 1 || result.Error != "Failed to start test sing-box instance: sing-box error" {
		t.Fatalf("calls=%d result=%+v err=%v", calls, result, err)
	}
}

func TestNativeOutboundEndpointsAndTransport(t *testing.T) {
	for _, protocol := range []string{"ssh", "singbox:ssh", "singbox:http", "singbox:shadowsocks"} {
		settings := map[string]any{"server": "2001:db8::1", "server_port": 443}
		got := extractOutboundEndpoints(map[string]any{"protocol": protocol, "settings": settings})
		if len(got) != 1 || got[0] != "[2001:db8::1]:443" {
			t.Fatalf("%s endpoints=%v", protocol, got)
		}
	}
	for _, protocol := range []string{"tuic", "singbox:tuic", "hysteria2", "singbox:hysteria2", "selector", "singbox:urltest"} {
		if !outboundTransportIsUDP(map[string]any{"protocol": protocol}) {
			t.Fatalf("%s must use handshake probe", protocol)
		}
	}
}

// Optional integration check with the real installed release. All traffic
// stays on loopback; no Internet proxy credentials are required.
func TestSingBoxProbeWithRunningProductionProcess(t *testing.T) {
	binary := os.Getenv("HIDDIFY_RUNTIME_TEST_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_RUNTIME_TEST_BINARY to run real sing-box integration")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, singbox.GetBinaryName()), data, 0o700); err != nil {
		t.Fatal(err)
	}
	productionConfig := filepath.Join(dir, "production.json")
	if err := os.WriteFile(productionConfig, []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	production := hiddify.NewProcess(productionConfig, false)
	if err := production.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer production.Stop()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	socks, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer socks.Close()
	go serveStubSocks(socks)
	host, port, _ := net.SplitHostPort(socks.Addr().String())
	number, _ := strconv.Atoi(port)
	withEgressTraceProbe(t, func(_ *url.URL) *TestEgressResult { return nil })
	raw := mustJSON(t, map[string]any{"protocol": "singbox:socks", "tag": "proxy", "settings": map[string]any{"server": host, "server_port": number}})
	result, err := (&OutboundService{CoreType: "sing-box"}).TestOutbound(raw, server.URL, "", "http")
	if err == nil && strings.Contains(result.Error, "create netlink socket: operation not permitted") {
		t.Skip("runtime denies netlink sockets; real sing-box launch cannot be exercised here")
	}
	if err != nil || !result.Success || result.HTTPStatus != 204 {
		t.Fatalf("real probe=%+v err=%v", result, err)
	}
	if !production.IsRunning() {
		t.Fatal("probe stopped production process")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "xray_test_") {
			t.Fatalf("leaked test config %s", entry.Name())
		}
	}
}

func TestSingBoxBatchHTTPThroughNativeGroup(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	original := newSingBoxBatchProcess
	defer func() { newSingBoxBatchProcess = original }()
	withEgressTraceProbe(t, func(_ *url.URL) *TestEgressResult { return nil })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	calls := 0
	newSingBoxBatchProcess = func(cfg *singbox.Config, _path string) batchProcess {
		calls++
		if len(cfg.Outbounds) != 2 || cfg.Outbounds[0]["type"] != "anytls" || cfg.Outbounds[1]["type"] != "selector" {
			t.Fatalf("native dependencies lost: %v", cfg.Outbounds)
		}
		if _, err := cfg.Marshal(); err != nil {
			t.Fatal(err)
		}
		stubCfg := &xray.Config{}
		for _, inbound := range cfg.Inbounds {
			stubCfg.InboundConfigs = append(stubCfg.InboundConfigs, xray.InboundConfig{Port: inbound["listen_port"].(int)})
		}
		return &stubProcess{cfg: stubCfg, serveSocks: true}
	}
	all := `[{"protocol":"anytls","tag":"hop","settings":{"server":"example.com","server_port":443,"password":"secret","tls":{"enabled":true}}},{"protocol":"selector","tag":"best","settings":{"outbounds":["hop"],"default":"hop"}}]`
	result, err := (&OutboundService{CoreType: "sing-box"}).TestOutbound(`{"protocol":"selector","tag":"best","settings":{"outbounds":["hop"],"default":"hop"}}`, server.URL, all, "tcp")
	if err != nil || !result.Success || result.HTTPStatus != 204 || result.Mode != "http" || calls != 1 {
		t.Fatalf("calls=%d result=%+v err=%v", calls, result, err)
	}
}

func TestModifiedSingBoxProbePreservesXHTTPAndNativeResolver(t *testing.T) {
	source := &xray.Config{}
	source.OutboundConfigs = []byte(`[{"tag":"proxy","protocol":"vless","settings":{"address":"example.com","port":443,"id":"2a547fe4-5b0f-4a84-845c-131464875bf8","encryption":"none"},"streamSettings":{"network":"xhttp","xhttpSettings":{"path":"/relay","mode":"auto"}}}]`)
	source.RouterConfig = []byte(`{"rules":[{"type":"field","outboundTag":"proxy","network":"tcp,udp"}]}`)
	cfg, err := buildSingBoxBatchTestConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Outbounds[0]["transport"].(map[string]any)["type"] != "xhttp" {
		t.Fatal("modified transport lost")
	}
	if cfg.Route["default_domain_resolver"] != cfg.DNS["final"] {
		t.Fatal("domain resolver lost")
	}
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if binary := os.Getenv("SINGBOX_TEST_BINARY"); binary != "" {
		file := filepath.Join(t.TempDir(), "probe.json")
		if err := os.WriteFile(file, data, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "check", "-c", file)
		cmd.Dir = filepath.Dir(binary)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("native probe: %v: %s", err, output)
		}
	}
}
