package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

const nodeRouteLiveUUID = "11111111-1111-4111-8111-111111111111"

func nodeRouteLivePort(t *testing.T, network string) int {
	t.Helper()
	if network == "udp" {
		conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		return conn.LocalAddr().(*net.UDPAddr).Port
	}
	conn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	return conn.Addr().(*net.TCPAddr).Port
}

// Run a real, isolated modified sing-box process. No externally routable
// listener is needed; the main node, remote node and client share loopback.
func startNodeRouteLiveCore(t *testing.T, binary string, cfg *singbox.Config, tcpReadyPort int) func() {
	t.Helper()
	cfg.Log = map[string]any{"level": "warn"}
	// Config.Marshal installs a default Clash API listener on 127.0.0.1:10090
	// unless explicitly disabled. Three simultaneous cores must not compete
	// for that panel-only socket during the isolated cascade test.
	cfg.Experimental = map[string]any{"clash_api": map[string]any{"external_controller": ""}}
	raw, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cascade.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	check := exec.Command(binary, "check", "-c", path)
	check.Dir = filepath.Dir(binary)
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("modified sing-box rejected cascade config: %v: %s", err, output)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, "srun", "-c", path)
	cmd.Dir = filepath.Dir(binary)
	log, err := os.CreateTemp(t.TempDir(), "cascade-core-*.log")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		cancel()
		_ = log.Close()
		t.Fatal(err)
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			_ = cmd.Wait()
			_ = log.Close()
		})
	}
	t.Cleanup(stop)
	if tcpReadyPort == 0 {
		time.Sleep(500 * time.Millisecond)
		return stop
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpReadyPort))
	for until := time.Now().Add(10 * time.Second); ; {
		conn, dialErr := net.DialTimeout("tcp", addr, 150*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return stop
		}
		if time.Now().After(until) {
			data, _ := os.ReadFile(log.Name())
			t.Fatalf("modified sing-box listener %s did not start: %v\n%s", addr, dialErr, data)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func nodeRouteLiveHTTP(socksPort int, target string) (string, error) {
	deadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(socksPort)), nil, &net.Dialer{})
	if err != nil {
		return "", err
	}
	conn, err := dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", target)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"); err != nil {
		return "", err
	}
	reply, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return "", err
	}
	defer reply.Body.Close()
	body, err := io.ReadAll(reply.Body)
	if err != nil {
		return "", err
	}
	if reply.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status %d", reply.StatusCode)
	}
	return string(body), nil
}

// A test-only subscription renderer keeps the production managed-outbound
// resolver and injector in the request path, with the remote link pinned to
// an isolated loopback listener.
type nodeRouteLiveLinkProvider struct {
	inboundID int
	email     string
	link      string
}

func (p *nodeRouteLiveLinkProvider) SubLinksForSubId(_, _ string) ([]string, error) {
	return nil, nil
}

func (p *nodeRouteLiveLinkProvider) LinksForClient(_ string, inbound *model.Inbound, email string) []string {
	if inbound == nil || inbound.Id != p.inboundID || email != p.email {
		return nil
	}
	return []string{p.link}
}

func (p *nodeRouteLiveLinkProvider) LinksForInbounds(_ string, _ []*model.Inbound) []string {
	return nil
}

// This checks real client -> panel-managed inbound -> node-route -> remote
// proxy -> HTTP traffic, rather than only probing the destination port or
// testing an outbound in a separate instance without the routing rule.
func TestNodeRouteLiveLocalInboundToRemoteCore(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_CONFIG_CHECK_BINARY to run a real two-node cascade")
	}
	for _, tc := range []struct {
		name, local, remote string
	}{
		{"vless-to-vless", "vless", "vless"},
		{"vless-to-shadowsocks", "vless", "shadowsocks"},
		{"tuic-to-vless", "tuic", "vless"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupBulkDB(t)
			t.Setenv("XUI_BIN_FOLDER", t.TempDir())
			localPort := nodeRouteLivePort(t, map[bool]string{true: "udp", false: "tcp"}[tc.local == "tuic"])
			remotePort := nodeRouteLivePort(t, "tcp")
			socksPort := nodeRouteLivePort(t, "tcp")

			protocol := model.VLESS
			settings := `{"decryption":"none"}`
			stream := `{"network":"tcp","security":"none"}`
			if tc.local == "tuic" {
				protocol = model.TUIC
				cert, key := writeTestCertificate(t)
				settings = fmt.Sprintf(`{"server":{"certificate":%q,"private_key":%q,"alpn":["h3"],"congestion_control":"bbr","authentication_timeout":5}}`, cert, key)
				stream = `{}`
			}
			source := seedHiddifyConfigInbound(t, protocol, settings, stream)
			if err := database.GetDB().Model(source).Update("port", localPort).Error; err != nil {
				t.Fatal(err)
			}

			remoteNode := &model.Node{Name: "cascade-live-node", Address: "127.0.0.1", Enable: true, Status: "online"}
			if err := database.GetDB().Create(remoteNode).Error; err != nil {
				t.Fatal(err)
			}
			remoteProtocol := model.VLESS
			link := fmt.Sprintf("vless://%s@127.0.0.1:%d?security=none&type=tcp#remote", nodeRouteLiveUUID, remotePort)
			if tc.remote == "shadowsocks" {
				remoteProtocol = model.Shadowsocks
				identity := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:cascade-test-password"))
				link = fmt.Sprintf("ss://%s@127.0.0.1:%d#remote", identity, remotePort)
			}
			foreignInbound := &model.Inbound{
				Enable: true, Protocol: remoteProtocol, Tag: "remote-" + tc.remote,
				Port: remotePort, NodeID: &remoteNode.Id, Settings: "{}", StreamSettings: "{}",
			}
			if err := database.GetDB().Create(foreignInbound).Error; err != nil {
				t.Fatal(err)
			}
			email := nodeRouteClientEmail(foreignInbound.Id)
			identity := &model.ClientRecord{Email: email, UUID: nodeRouteLiveUUID, Password: "cascade-test-password", Enable: true, SubID: "cascade-live"}
			if err := database.GetDB().Create(identity).Error; err != nil {
				t.Fatal(err)
			}
			previousProvider := subLinkProvider()
			RegisterSubLinkProvider(&nodeRouteLiveLinkProvider{inboundID: foreignInbound.Id, email: email, link: link})
			t.Cleanup(func() { RegisterSubLinkProvider(previousProvider) })
			destination := nodeRouteTag(foreignInbound.Id)
			template := fmt.Sprintf(`{"route":{"final":"blocked","rules":[{"comment":"panel-quick:inbound:live","enabled":true,"inbound":[%q],"action":"route","outbound":%q}]}}`, source.Tag, destination)
			if err := singBoxSettingService.SetSingBoxConfigTemplate(template); err != nil {
				t.Fatal(err)
			}
			main, err := (&HiddifyConfigGenerator{}).Generate()
			if err != nil {
				t.Fatal(err)
			}
			if main.Route["final"] != "blocked" {
				t.Fatalf("panel fallback is not fail-closed: %#v", main.Route)
			}
			routed := false
			for _, item := range singBoxRouteRules(main.Route) {
				rule, ok := item.(map[string]any)
				if !ok || rule["outbound"] != destination {
					continue
				}
				for _, tag := range readInboundTags(rule["inbound"]) {
					if tag == source.Tag {
						routed = true
					}
				}
			}
			if !routed {
				t.Fatalf("the generated local inbound %s is not routed to %s", source.Tag, destination)
			}

			foreign := singbox.NewConfig()
			foreign.Outbounds = []map[string]any{{"type": "direct", "tag": "direct"}}
			foreign.Route["final"] = "direct"
			var remoteInbound, managedOutbound map[string]any
			if tc.remote == "vless" {
				remoteInbound = map[string]any{"type": "vless", "tag": "foreign", "listen": "127.0.0.1", "listen_port": remotePort, "users": []map[string]any{{"name": "service", "uuid": nodeRouteLiveUUID}}}
				managedOutbound = map[string]any{"type": "vless", "tag": destination, "server": "127.0.0.1", "server_port": remotePort, "uuid": nodeRouteLiveUUID}
			} else {
				remoteInbound = map[string]any{"type": "shadowsocks", "tag": "foreign", "listen": "127.0.0.1", "listen_port": remotePort, "method": "aes-128-gcm", "password": "cascade-test-password"}
				managedOutbound = map[string]any{"type": "shadowsocks", "tag": destination, "server": "127.0.0.1", "server_port": remotePort, "method": "aes-128-gcm", "password": "cascade-test-password"}
			}
			foreign.Inbounds = []map[string]any{remoteInbound}
			found := false
			for _, ob := range main.Outbounds {
				if ob["tag"] != destination {
					continue
				}
				if ob["type"] != managedOutbound["type"] ||
					ob["server"] != "127.0.0.1" ||
					fmt.Sprint(ob["server_port"]) != strconv.Itoa(remotePort) {
					t.Fatalf("managed outbound was not generated from the remote service link: %#v", ob)
				}
				found = true
			}
			if !found {
				t.Fatal("panel did not resolve the foreign inbound as an outbound")
			}
			stopRemote := startNodeRouteLiveCore(t, binary, foreign, remotePort)
			ready := localPort
			if tc.local == "tuic" {
				ready = 0
			}
			startNodeRouteLiveCore(t, binary, main, ready)

			client := singbox.NewConfig()
			client.Inbounds = []map[string]any{{"type": "mixed", "tag": "client", "listen": "127.0.0.1", "listen_port": socksPort}}
			var entry map[string]any
			if tc.local == "tuic" {
				entry = map[string]any{"type": "tuic", "tag": "to-main", "server": "127.0.0.1", "server_port": localPort, "uuid": nodeRouteLiveUUID, "password": "test-password", "congestion_control": "bbr", "tls": map[string]any{"enabled": true, "insecure": true, "alpn": []string{"h3"}}}
			} else {
				entry = map[string]any{"type": "vless", "tag": "to-main", "server": "127.0.0.1", "server_port": localPort, "uuid": nodeRouteLiveUUID}
			}
			client.Outbounds = []map[string]any{entry}
			client.Route["final"] = "to-main"
			startNodeRouteLiveCore(t, binary, client, socksPort)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, "remote-cascade-ok")
			}))
			defer server.Close()
			target := strings.TrimPrefix(server.URL, "http://")
			got, err := nodeRouteLiveHTTP(socksPort, target)
			if err != nil || got != "remote-cascade-ok" {
				t.Fatalf("client -> local %s -> remote %s -> HTTP failed: body=%q err=%v", tc.local, tc.remote, got, err)
			}
			// A succeeding HTTP request must depend on the foreign node.
			// If it succeeds after that process has stopped, a direct leak
			// bypassed the managed destination.
			stopRemote()
			if body, err := nodeRouteLiveHTTP(socksPort, target); err == nil && body == "remote-cascade-ok" {
				t.Fatal("traffic bypassed the stopped remote node")
			}
		})
	}
}
