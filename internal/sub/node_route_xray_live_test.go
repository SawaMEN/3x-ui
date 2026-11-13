package sub

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	_ "github.com/xtls/xray-core/app/dispatcher"
	_ "github.com/xtls/xray-core/app/log"
	_ "github.com/xtls/xray-core/app/policy"
	_ "github.com/xtls/xray-core/app/proxyman/inbound"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"
	_ "github.com/xtls/xray-core/app/router"
	_ "github.com/xtls/xray-core/app/stats"
	xraycore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
	_ "github.com/xtls/xray-core/proxy/blackhole"
	_ "github.com/xtls/xray-core/proxy/freedom"
	_ "github.com/xtls/xray-core/proxy/shadowsocks"
	_ "github.com/xtls/xray-core/proxy/socks"
	_ "github.com/xtls/xray-core/proxy/trojan"
	_ "github.com/xtls/xray-core/proxy/vless/inbound"
	_ "github.com/xtls/xray-core/proxy/vless/outbound"
	_ "github.com/xtls/xray-core/proxy/vmess/inbound"
	_ "github.com/xtls/xray-core/proxy/vmess/outbound"
	_ "github.com/xtls/xray-core/transport/internet/reality"
	_ "github.com/xtls/xray-core/transport/internet/tcp"
	_ "github.com/xtls/xray-core/transport/internet/tls"
	_ "github.com/xtls/xray-core/transport/internet/websocket"
	"golang.org/x/net/proxy"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	sharelink "github.com/SawaMEN/3x-ui/v3/internal/util/link"
	webruntime "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// Each process owns a separate core: production nodes never share Xray's
// process-wide log and transport registries. The executable is this Go test
// binary, using the exact xray-core version pinned by the panel's go.mod.
func TestNodeRouteXrayCoreProcess(t *testing.T) {
	configPath := os.Getenv("XUI_CASCADE_TEST_CONFIG")
	if configPath == "" {
		t.Skip("subprocess helper for real Xray cascades")
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var parsed conf.Config
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	parsed.LogConfig = &conf.LogConfig{LogLevel: "debug", AccessLog: "none"}
	built, err := parsed.Build()
	if err != nil {
		t.Fatal(err)
	}
	instance, err := xraycore.New(built)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	if err := instance.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath+".ready", []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	<-stop
}

func startNodeRouteXray(t *testing.T, config any) func() {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	configPath, logPath := filepath.Join(dir, "config.json"), filepath.Join(dir, "xray.log")
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNodeRouteXrayCoreProcess$", "-test.timeout=2m")
	cmd.Env = append(os.Environ(), "XUI_CASCADE_TEST_CONFIG="+configPath)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		cancel()
		log.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var once sync.Once
	stop := func() { once.Do(func() { cancel(); <-done; _ = log.Close() }) }
	t.Cleanup(func() {
		stop()
		if t.Failed() {
			data, _ := os.ReadFile(logPath)
			t.Logf("Xray log:\n%s", data)
		}
	})
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(configPath + ".ready"); err == nil {
			return stop
		}
		select {
		case err := <-done:
			// Preserve the result for stop(), the sole cleanup owner.
			done <- err
			data, _ := os.ReadFile(logPath)
			t.Fatalf("Xray process exited before ready: %v\n%s", err, data)
		case <-deadline.C:
			t.Fatal("Xray process did not become ready")
		case <-tick.C:
		}
	}
}

func nodeRouteXrayHTTP(ctx context.Context, socksPort int, target string) (string, error) {
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(socksPort)), nil, &net.Dialer{})
	if err != nil {
		return "", err
	}
	transport := &http.Transport{
		DialContext:       dialer.(proxy.ContextDialer).DialContext,
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	response, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024))
	return string(body), err
}

// Exercise DB credentials -> actual subscription renderer -> managed outbound
// resolver -> full panel configuration -> three real cores -> HTTP. A default
// block outbound and a stopped-foreign-core control expose direct-route leaks.
func TestNodeRouteXrayLiveSubscriptionCascade(t *testing.T) {
	for _, tc := range []struct {
		name             string
		protocol         model.Protocol
		network          string
		tls              bool
		reality          bool
		vision           bool
		provision        bool
		newService       bool
		disabledIdentity bool
	}{
		{name: "vless-tcp", protocol: model.VLESS, network: "tcp"},
		{name: "vmess-tcp", protocol: model.VMESS, network: "tcp"},
		{name: "vless-websocket", protocol: model.VLESS, network: "ws"},
		{name: "vless-tls", protocol: model.VLESS, network: "tcp", tls: true},
		{name: "trojan-tls", protocol: model.Trojan, network: "tcp", tls: true},
		{name: "shadowsocks", protocol: model.Shadowsocks, network: "tcp"},
		{name: "disabled-service", protocol: model.VLESS, network: "tcp", disabledIdentity: true},
		{name: "vless-reality-managed", protocol: model.VLESS, network: "tcp", reality: true, provision: true},
		{name: "vless-reality-new-managed", protocol: model.VLESS, network: "tcp", reality: true, provision: true, newService: true},
		{name: "vless-reality-vision", protocol: model.VLESS, network: "tcp", reality: true, vision: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seedSubDB(t)
			db := database.GetDB()
			settings := &service.SettingService{}
			if err := settings.SetCoreType(service.CoreTypeXray); err != nil {
				t.Fatal(err)
			}
			provider := NewLinkProvider()
			service.RegisterSubLinkProvider(provider)
			t.Cleanup(func() { service.RegisterSubLinkProvider(nil) })
			localPort, foreignPort, socksPort := anyTLSFreePort(t), anyTLSFreePort(t), anyTLSFreePort(t)
			local := seedSubInbound(t, "cascade-entry", "entry", localPort, 0, `{"network":"tcp","security":"none"}`)
			local.Listen = "127.0.0.1"
			if err := db.Save(local).Error; err != nil {
				t.Fatal(err)
			}
			node := &model.Node{Name: "foreign", Address: "127.0.0.1", Enable: true, Status: "online"}
			if err := db.Create(node).Error; err != nil {
				t.Fatal(err)
			}
			stream := map[string]any{"network": tc.network, "security": "none"}
			if tc.network == "ws" {
				stream["wsSettings"] = map[string]any{"path": "/cascade"}
			}
			if tc.tls {
				cert, key := anyTLSCertificate(t, "cascade.test")
				certPEM, err := os.ReadFile(cert)
				if err != nil {
					t.Fatal(err)
				}
				block, _ := pem.Decode(certPEM)
				hash := sha256.Sum256(block.Bytes)
				stream["security"] = "tls"
				stream["tlsSettings"] = map[string]any{
					"serverName":   "cascade.test",
					"settings":     map[string]any{"pinnedPeerCertSha256": []string{fmt.Sprintf("%x", hash)}},
					"certificates": []any{map[string]any{"certificateFile": cert, "keyFile": key}},
				}
			}
			if tc.reality {
				cover := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
				defer cover.Close()
				private, err := ecdh.X25519().GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				stream["security"] = "reality"
				stream["realitySettings"] = map[string]any{
					"dest": cover.Listener.Addr().String(), "serverNames": []string{"cascade.test"},
					"privateKey": base64.RawURLEncoding.EncodeToString(private.Bytes()), "shortIds": []string{"0123456789abcdef"},
					"settings": map[string]any{"publicKey": base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()), "fingerprint": "chrome"},
				}
			}
			const uuid = "22222222-2222-4222-8222-222222222222"
			const password = "cascade-service-password"
			remoteSettings := map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": uuid, "password": password, "email": "service", "enable": true}}}
			flow := ""
			if tc.vision {
				flow = "xtls-rprx-vision"
				remoteSettings["clients"].([]any)[0].(map[string]any)["flow"] = flow
			}
			if tc.protocol == model.Shadowsocks {
				remoteSettings = map[string]any{"method": "aes-128-gcm", "password": password, "network": "tcp,udp"}
			}
			streamRaw, _ := json.Marshal(stream)
			settingsRaw, _ := json.Marshal(remoteSettings)
			foreign := &model.Inbound{
				UserId: 1, Tag: "foreign", Enable: true, Listen: "127.0.0.1", Port: foreignPort,
				Protocol: tc.protocol, NodeID: &node.Id, Settings: string(settingsRaw), StreamSettings: string(streamRaw),
			}
			if err := db.Create(foreign).Error; err != nil {
				t.Fatal(err)
			}
			email := fmt.Sprintf("node-route-%d@internal", foreign.Id)
			if tc.protocol != model.Shadowsocks {
				remoteSettings["clients"].([]any)[0].(map[string]any)["email"] = email
				if tc.newService {
					remoteSettings["clients"].([]any)[0].(map[string]any)["email"] = "foreign-user"
				}
				data, _ := json.Marshal(remoteSettings)
				if err := db.Model(foreign).Update("settings", string(data)).Error; err != nil {
					t.Fatal(err)
				}
			}
			identity := &model.ClientRecord{Email: email, UUID: uuid, Password: password, Enable: !tc.disabledIdentity, SubID: "cascade-service"}
			if !tc.newService {
				if err := db.Create(identity).Error; err != nil {
					t.Fatal(err)
				}
				// GORM's default:true applies to a zero bool during Create.
				if tc.disabledIdentity {
					if err := db.Model(identity).Update("enable", false).Error; err != nil {
						t.Fatal(err)
					}
				}
				if err := db.Create(&model.ClientInbound{ClientId: identity.Id, InboundId: foreign.Id, FlowOverride: flow}).Error; err != nil {
					t.Fatal(err)
				}
			}
			destination := fmt.Sprintf("node-route-%d", foreign.Id)
			if tc.provision {
				// The destination panel RPC is isolated, but identity creation,
				// repair, link rendering and both traffic engines are real.
				panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == "/panel/api/inbounds/list" {
						_, _ = w.Write([]byte(`{"success":true,"obj":[{"id":7,"tag":"foreign"}]}`))
						return
					}
					if r.URL.Path == "/panel/api/inbounds/update/7" {
						if err := json.Unmarshal([]byte(r.FormValue("settings")), &remoteSettings); err != nil {
							t.Error(err)
						}
					}
					_, _ = w.Write([]byte(`{"success":true}`))
				}))
				defer panel.Close()
				node.Port = panel.Listener.Addr().(*net.TCPAddr).Port
				node.Scheme, node.BasePath, node.ApiToken, node.AllowPrivateAddress = "http", "/", "test-token", true
				if err := db.Save(node).Error; err != nil {
					t.Fatal(err)
				}
				previous := webruntime.GetManager()
				webruntime.SetManager(webruntime.NewManager(webruntime.LocalDeps{APIPort: func() int { return 0 }, SetNeedRestart: func() {}}))
				t.Cleanup(func() { webruntime.SetManager(previous) })
				var credential string
				for attempt := 0; attempt < 2; attempt++ {
					if tag, _, err := service.EnsureNodeRoute(1, foreign.Id, service.CoreTypeXray); err != nil || tag != destination {
						t.Fatalf("prepare managed Reality route: tag=%q err=%v", tag, err)
					}
					var current model.ClientRecord
					if err := db.Where("email = ?", email).First(&current).Error; err != nil {
						t.Fatal(err)
					}
					if attempt > 0 && credential != current.UUID {
						t.Fatal("repeated preparation rotated service credential")
					}
					credential = current.UUID
				}
				// The destination's ordinary Xray config generator canonicalizes
				// the subscription-only UDP443 suffix on the server-side account.
				for _, raw := range remoteSettings["clients"].([]any) {
					user := raw.(map[string]any)
					if value, _ := user["flow"].(string); value != "" {
						user["flow"] = strings.TrimSuffix(value, "-udp443")
					}
				}
			}
			template := fmt.Sprintf(`{"log":{"loglevel":"none"},"outbounds":[{"protocol":"blackhole","tag":"blocked"}],"routing":{"rules":[{"type":"field","ruleTag":"panel-quick:inbound:live","inboundTag":[%q],"outboundTag":%q}]}}`, local.Tag, destination)
			if err := settings.SetXrayConfigTemplate(template); err != nil {
				t.Fatal(err)
			}
			main, err := (&service.CoreService{}).GetXrayConfig()
			if err != nil {
				t.Fatal(err)
			}
			var outbounds []map[string]any
			if err := json.Unmarshal(main.OutboundConfigs, &outbounds); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, outbound := range outbounds {
				if outbound["tag"] == destination {
					expected := string(tc.protocol)
					if tc.disabledIdentity {
						expected = "blackhole"
					}
					if outbound["protocol"] != expected {
						t.Fatalf("managed outbound is not the foreign protocol: %v", outbound["protocol"])
					}
					found = true
				}
			}
			if !found {
				t.Fatal("managed outbound not generated")
			}
			foreignConfig := map[string]any{
				"log":      map[string]any{"loglevel": "none"},
				"inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": foreignPort, "protocol": string(tc.protocol), "settings": remoteSettings, "streamSettings": stream}},
				// The pinned Xray blocks private destinations by default. Permit
				// only this test's loopback HTTP server on the fixture exit.
				"outbounds": []any{map[string]any{
					"protocol": "freedom", "tag": "direct",
					"settings": map[string]any{"finalRules": []any{map[string]any{"action": "allow", "ip": []string{"127.0.0.1/32"}}}},
				}},
			}
			stopForeign := startNodeRouteXray(t, foreignConfig)
			startNodeRouteXray(t, main)
			links := provider.LinksForClient("127.0.0.1", local, local.Tag+"@e")
			if len(links) != 1 {
				t.Fatalf("local subscription has %d links", len(links))
			}
			entry, err := sharelink.ParseLink(links[0])
			if err != nil {
				t.Fatal(err)
			}
			startNodeRouteXray(t, map[string]any{
				"log":       map[string]any{"loglevel": "none"},
				"inbounds":  []any{map[string]any{"listen": "127.0.0.1", "port": socksPort, "protocol": "socks", "settings": map[string]any{"auth": "noauth"}}},
				"outbounds": []any{entry.Outbound},
			})
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				_, _ = io.WriteString(w, "foreign-cascade-ok")
			}))
			defer server.Close()
			request := func() (string, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return nodeRouteXrayHTTP(ctx, socksPort, server.URL)
			}
			if tc.disabledIdentity {
				if _, err := request(); err == nil || hits.Load() != 0 {
					t.Fatal("disabled service identity forwarded traffic")
				}
				return
			}
			if body, err := request(); err != nil || body != "foreign-cascade-ok" {
				t.Fatalf("client -> local -> foreign -> HTTP: body=%q err=%v", body, err)
			}
			stopForeign()
			before := hits.Load()
			if body, err := request(); err == nil || strings.Contains(body, "foreign-cascade-ok") {
				t.Fatal("traffic bypassed stopped foreign node")
			}
			if hits.Load() != before {
				t.Fatal("stopped foreign node still delivered an HTTP request")
			}
		})
	}
}
