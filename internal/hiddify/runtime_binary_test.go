package hiddify

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Entirely loopback: validates the real process/API contract without a public
// IP lookup, VPN provider or privileged TUN device.
func TestPinnedBinaryRawRuntimeAndStatistics(t *testing.T) {
	binary := os.Getenv("HIDDIFY_RUNTIME_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_RUNTIME_BINARY to run the real loopback runtime test")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XUI_BIN_FOLDER", filepath.Dir(binary))
	reserve := func() string {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := listener.Addr().String()
		_ = listener.Close()
		return address
	}
	socksAddress, statsAddress, clashAddress := reserve(), reserve(), reserve()
	_, portText, _ := net.SplitHostPort(socksAddress)
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	logPath, configPath := filepath.Join(dir, "core.log"), filepath.Join(dir, "core.json")
	cfg := map[string]any{
		"log": map[string]any{"level": "info", "output": logPath},
		"inbounds": []any{map[string]any{
			"type": "socks", "tag": "vpn", "listen": "127.0.0.1", "listen_port": port,
			"users": []any{map[string]any{"username": "alice", "password": "secret"}},
		}},
		"outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}},
		"experimental": map[string]any{
			"v2ray_api": map[string]any{"listen": statsAddress, "stats": map[string]any{"enabled": true, "users": []string{"alice"}, "inbounds": []string{"vpn"}}},
			"clash_api": map[string]any{"external_controller": clashAddress, "secret": "secret"},
		},
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRuntime(configPath, true)
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if !r.IsRunning() {
		t.Fatal("core is not running")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) { _, _ = io.WriteString(w, "loopback-ok") }))
	defer upstream.Close()
	dialer, err := proxy.SOCKS5("tcp", socksAddress, &proxy.Auth{User: "alice", Password: "secret"}, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{DialContext: dialer.(proxy.ContextDialer).DialContext}
	defer transport.CloseIdleConnections()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(body) != "loopback-ok" {
		t.Fatalf("loopback body: %q", body)
	}
	stats := singbox.NewV2RayStatsClient(statsAddress, StatsQueryMethod)
	defer stats.Close()
	values, err := stats.Query(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var userUp, inboundUp int64
	for _, value := range values {
		if value.Name == "user>>>alice>>>traffic>>>uplink" {
			userUp += value.Value
		}
		if value.Name == "inbound>>>vpn>>>traffic>>>uplink" {
			inboundUp += value.Value
		}
	}
	if userUp <= 0 || inboundUp != userUp {
		t.Fatalf("user/inbound metering: %d/%d; %#v", userUp, inboundUp, values)
	}
	clashRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+clashAddress+"/connections", nil)
	clashRequest.Header.Set("Authorization", "Bearer secret")
	clashResponse, err := http.DefaultClient.Do(clashRequest)
	if err != nil {
		t.Fatal(err)
	}
	var connections struct {
		Connections []struct {
			Metadata struct {
				User string `json:"user"`
			} `json:"metadata"`
		} `json:"connections"`
	}
	err = json.NewDecoder(clashResponse.Body).Decode(&connections)
	_ = clashResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	foundUser := false
	for _, connection := range connections.Connections {
		foundUser = foundUser || connection.Metadata.User == "alice"
	}
	if !foundUser {
		t.Fatal("Clash metadata has no authenticated user")
	}
	logs, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(logs), "started") {
		t.Fatalf("missing runtime log: %v: %s", err, logs)
	}
	if err := r.Restart(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if r.IsRunning() {
		t.Fatal("core did not stop")
	}
}
