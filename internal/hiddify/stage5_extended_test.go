package hiddify

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func stage5UDPEcho(t *testing.T) (string, int) {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			n, a, e := c.ReadFrom(buf)
			if e != nil {
				return
			}
			_, _ = c.WriteTo(buf[:n], a)
		}
	}()
	return "127.0.0.1", c.LocalAddr().(*net.UDPAddr).Port
}

func stage5Data(t *testing.T, network, address string, allowed bool) {
	t.Helper()
	c, err := net.DialTimeout(network, address, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	payload := []byte("stage5-protocol-echo")
	if !allowed {
		payload = []byte("X")
	}
	if _, err = c.Write(payload); err != nil {
		if !allowed {
			return
		}
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	var n int
	if network == "tcp" {
		n, err = io.ReadFull(c, got)
	} else {
		n, err = c.Read(got)
	}
	if !allowed {
		if n > 0 {
			t.Fatalf("unauthorized client received %q", got[:n])
		}
		return
	}
	if err != nil || !bytes.Equal(got[:n], payload) {
		t.Fatalf("%s echo: %q %v", network, got[:n], err)
	}
}

// The server options match the translated/database fixtures. Each compatible
// native client is passed through the production native outbound translator.
func TestStage5ExtendedProtocolTraffic(t *testing.T) {
	binary := os.Getenv("HIDDIFY_RUNTIME_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_RUNTIME_BINARY")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"socks", "http", "shadowsocks-aead", "shadowsocks-2022", "hysteria", "hysteria2", "tuic", "anytls", "naive", "snell5", "snell6"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			cert, key := testCertificate(t, dir)
			port := stage5Port(t)
			protocol := kind
			if strings.HasPrefix(kind, "shadowsocks-") {
				protocol = "shadowsocks"
			}
			if strings.HasPrefix(kind, "snell") {
				protocol = "snell"
			}
			tlsServer := map[string]any{"enabled": true, "certificate_path": cert, "key_path": key}
			tlsClient := map[string]any{"enabled": true, "server_name": "localhost", "certificate_path": cert}
			server := map[string]any{"type": protocol, "tag": "server", "listen": "127.0.0.1", "listen_port": port}
			client := map[string]any{"type": protocol, "tag": "proxy", "server": "127.0.0.1", "server_port": port}
			password := "secret"
			tcpOnly := kind == "http" || kind == "naive"
			switch kind {
			case "socks", "http":
				server["users"] = []any{map[string]any{"username": "alice", "password": password}}
				client["username"], client["password"] = "alice", password
			case "shadowsocks-aead", "shadowsocks-2022":
				method := "aes-128-gcm"
				if kind == "shadowsocks-2022" {
					method = "2022-blake3-aes-128-gcm"
					password = "MDEyMzQ1Njc4OWFiY2RlZg=="
				}
				server["method"], client["method"] = method, method
				server["password"], client["password"] = password, password
			case "snell5", "snell6":
				v := 5
				if kind == "snell6" {
					v = 6
					server["mode"], client["mode"] = "unshaped", "unshaped"
				}
				server["version"], client["version"] = v, v
				server["psk"], client["psk"] = "test-psk-12345678", "test-psk-12345678"
				userkey := strings.Repeat("11", 32)
				server["users"] = []any{map[string]any{"name": "alice", "userkey": userkey}}
				client["userkey"] = userkey
			default:
				server["tls"], client["tls"] = tlsServer, tlsClient
				user := map[string]any{"name": "alice", "password": password}
				client["password"] = password
				switch kind {
				case "hysteria":
					delete(user, "password")
					user["auth_str"] = password
					delete(client, "password")
					client["auth_str"] = password
					server["up_mbps"], server["down_mbps"], client["up_mbps"], client["down_mbps"] = 100, 100, 100, 100
				case "tuic":
					user["uuid"] = "11111111-1111-4111-8111-111111111111"
					client["uuid"] = user["uuid"]
					tlsServer["alpn"], tlsClient["alpn"] = []string{"h3"}, []string{"h3"}
				case "naive":
					delete(user, "name")
					user["username"] = "alice"
					client["username"] = "alice"
					tlsServer["alpn"], tlsClient["alpn"] = []string{"h2"}, []string{"h2"}
				}
				server["users"] = []any{user}
			}
			udpProtocol := kind == "hysteria" || kind == "hysteria2" || kind == "tuic"
			stage5Start(t, binary, dir, "server", map[string]any{"inbounds": []any{server}, "outbounds": []any{map[string]any{"type": "direct"}}}, port, !udpProtocol)
			echo := stage5Echo(t, "127.0.0.1:0")
			host, p, _ := net.SplitHostPort(echo)
			echoPort, _ := strconv.Atoi(p)
			udpHost, udpPort := stage5UDPEcho(t)
			for _, valid := range []bool{true, false} {
				t.Run(fmt.Sprint("authorized=", valid), func(t *testing.T) {
					outbound := maps.Clone(client)
					if !valid {
						switch kind {
						case "hysteria":
							outbound["auth_str"] = "wrong"
						case "snell5", "snell6":
							outbound["userkey"] = strings.Repeat("22", 32)
						case "shadowsocks-2022":
							outbound["password"] = "YWJjZGVmZ2hpamtsbW5vcA=="
						default:
							outbound["password"] = "wrong"
						}
					}
					translated, err := TranslateOutbound(outbound)
					if err != nil {
						t.Fatal(err)
					}
					tcpPort, udpRelay := stage5Port(t), stage5Port(t)
					ins := []any{map[string]any{"type": "direct", "listen": "127.0.0.1", "listen_port": tcpPort, "network": "tcp", "override_address": host, "override_port": echoPort}}
					if !tcpOnly {
						ins = append(ins, map[string]any{"type": "direct", "listen": "127.0.0.1", "listen_port": udpRelay, "network": "udp", "override_address": udpHost, "override_port": udpPort})
					}
					stage5Start(t, binary, dir, fmt.Sprint("client", valid), map[string]any{"inbounds": ins, "outbounds": []any{translated}}, tcpPort, true)
					stage5Data(t, "tcp", fmt.Sprintf("127.0.0.1:%d", tcpPort), valid)
					if valid {
						stage5Data(t, "tcp", fmt.Sprintf("127.0.0.1:%d", tcpPort), true)
					}
					if !tcpOnly {
						stage5Data(t, "udp", fmt.Sprintf("127.0.0.1:%d", udpRelay), valid)
					}
				})
			}
		})
	}
}
