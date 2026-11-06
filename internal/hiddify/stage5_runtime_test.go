package hiddify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Test real panel translation in both directions, not a manually patched
// native transport. Each accepted user reconnects; invalid and disabled users
// must never receive the echo payload.
func TestStage5ProtocolTraffic(t *testing.T) {
	binary := os.Getenv("HIDDIFY_RUNTIME_BINARY")
	if selected := os.Getenv("STAGE5_RUNTIME_BINARY"); selected != "" {
		binary = selected
	}
	if binary == "" {
		t.Skip("set HIDDIFY_RUNTIME_BINARY for native protocol traffic")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	translateIn, translateOut := TranslateInbound, TranslateOutbound
	networks := []string{"tcp", "raw", "ws", "grpc", "http", "httpupgrade", "xhttp", "kcp", "xdrive", "native-quic"}
	if os.Getenv("STAGE5_NATIVE_PROFILE") == "upstream" {
		translateIn, translateOut = singbox.TranslateXrayInbound, singbox.TranslateXrayOutbound
		networks = networks[:6] // upstream has neither XHTTP nor mKCP
	}
	xray := os.Getenv("STAGE5_NATIVE_PROFILE") == "xray"
	if xray {
		networks = []string{"tcp", "raw", "ws", "grpc", "httpupgrade", "xhttp", "kcp", "xdrive"}
		translateIn = func(raw map[string]any) (map[string]any, error) {
			out := maps.Clone(raw)
			settings := maps.Clone(raw["settings"].(map[string]any))
			var active []any
			for _, u := range settings["clients"].([]any) {
				if u.(map[string]any)["enable"] != false {
					active = append(active, u)
				}
			}
			settings["clients"] = active
			out["settings"] = settings
			return out, nil
		}
		translateOut = func(raw map[string]any) (map[string]any, error) {
			out := maps.Clone(raw)
			settings := raw["settings"].(map[string]any)
			if raw["protocol"] == "trojan" {
				out["settings"] = map[string]any{"servers": []any{map[string]any{"address": settings["address"], "port": settings["port"], "password": settings["password"]}}}
			} else {
				user := map[string]any{"id": settings["id"], "encryption": "none", "security": "auto"}
				if e := settings["encryption"]; e != nil {
					user["encryption"] = e
				}
				out["settings"] = map[string]any{"vnext": []any{map[string]any{"address": settings["address"], "port": settings["port"], "users": []any{user}}}}
			}
			if stream, ok := raw["streamSettings"].(map[string]any); ok && stream["network"] == "xdrive" {
				out["mux"] = map[string]any{"enabled": true, "concurrency": 8, "xudpConcurrency": 8, "xudpProxyUDP443": "reject"}
			}
			return out, nil
		}
	}
	for _, protocol := range []string{"vless", "vmess", "trojan"} {
		for _, network := range networks {
			if network == "xdrive" && protocol != "vless" {
				continue
			}
			t.Run(protocol+"/"+network, func(t *testing.T) {
				dir := t.TempDir()
				port := stage5Port(t)
				streamNetwork := network
				if network == "native-quic" {
					streamNetwork = "tcp"
				}
				serverStream := map[string]any{"network": streamNetwork, "security": "none"}
				clientStream := map[string]any{"network": streamNetwork, "security": "none"}
				for _, stream := range []map[string]any{serverStream, clientStream} {
					switch network {
					case "xhttp":
						stream["xhttpSettings"] = map[string]any{"path": "/stage5", "mode": "auto"}
					case "ws":
						stream["wsSettings"] = map[string]any{"path": "/stage5"}
					case "grpc":
						stream["grpcSettings"] = map[string]any{"serviceName": "stage5"}
					case "http":
						stream["httpSettings"] = map[string]any{"path": "/stage5"}
					case "httpupgrade":
						stream["httpupgradeSettings"] = map[string]any{"path": "/stage5"}
					}
				}
				if protocol == "trojan" || network == "native-quic" {
					cert, key := testCertificate(t, dir)
					serverStream["security"], clientStream["security"] = "tls", "tls"
					serverStream["tlsSettings"] = map[string]any{"certificates": []any{map[string]any{"certificateFile": cert, "keyFile": key}}}
					clientStream["tlsSettings"] = map[string]any{"serverName": "localhost", "certificates": []any{map[string]any{"usage": "verify", "certificateFile": cert}}}
					if network == "native-quic" {
						clientStream["tlsSettings"].(map[string]any)["fingerprint"] = "unsafe"
					}
				}
				var pair struct {
					Encryption string `json:"encryption"`
					Decryption string `json:"decryption"`
				}
				if network == "xdrive" {
					if protocol != "vless" {
						t.Fatal("XDrive matrix must only include VLESS")
					}
					encryptionBinary := binary
					if xray {
						encryptionBinary = os.Getenv("STAGE5_ENCRYPTION_BINARY")
						if encryptionBinary == "" {
							t.Fatal("Xray XDrive needs the pinned compatible key generator")
						}
					}
					data, err := exec.Command(encryptionBinary, "vlessenc", "--json").CombinedOutput()
					if err != nil {
						t.Fatalf("vlessenc: %v %s", err, data)
					}
					if err := json.Unmarshal(data, &pair); err != nil || pair.Encryption == "" || pair.Decryption == "" {
						t.Fatalf("invalid key pair: %v", err)
					}
					for _, stream := range []map[string]any{serverStream, clientStream} {
						stream["xdriveSettings"] = map[string]any{"service": "local", "remoteFolder": filepath.Join(dir, "storage"), "pollIntervalMs": float64(10)}
					}
				}
				users := []any{}
				for i := 1; i <= 3; i++ {
					users = append(users, map[string]any{"id": fmt.Sprintf("11111111-1111-4111-8111-%012d", i), "password": fmt.Sprintf("password%d", i), "email": fmt.Sprintf("user%d", i), "enable": i != 3})
				}
				serverSettings := map[string]any{"clients": users, "decryption": "none"}
				if network == "xdrive" {
					serverSettings["decryption"] = pair.Decryption
				}
				inbound, err := translateIn(map[string]any{"protocol": protocol, "tag": "server", "listen": "127.0.0.1", "port": port, "settings": serverSettings, "streamSettings": serverStream})
				if err != nil {
					t.Fatal(err)
				}
				if network == "native-quic" {
					inbound["transport"] = map[string]any{"type": "quic"}
				}
				serverConfig := map[string]any{"inbounds": []any{inbound}, "outbounds": []any{map[string]any{"type": "direct"}}}
				if xray {
					serverConfig["outbounds"] = []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"finalRules": []any{map[string]any{"action": "allow", "ip": []string{"127.0.0.0/8", "::1/128"}}}}}}
				}
				stage5Start(t, binary, dir, "server", serverConfig, port, network != "kcp" && network != "xdrive" && network != "native-quic")
				echoAddress := stage5Echo(t, "127.0.0.1:0")
				v6 := stage5Echo(t, "[::1]:0")
				_, echoPort, err := net.SplitHostPort(echoAddress)
				if err != nil {
					t.Fatal(err)
				}
				for _, user := range []int{1, 2, 3, 4} {
					t.Run(fmt.Sprintf("user%d", user), func(t *testing.T) {
						socksPort := stage5Port(t)
						settings := map[string]any{"address": "127.0.0.1", "port": port, "id": fmt.Sprintf("11111111-1111-4111-8111-%012d", user), "password": fmt.Sprintf("password%d", user)}
						if network == "xdrive" {
							settings["encryption"] = pair.Encryption
						}
						outbound, err := translateOut(map[string]any{"protocol": protocol, "tag": "proxy", "settings": settings, "streamSettings": clientStream})
						if err != nil {
							t.Fatal(err)
						}
						if network == "native-quic" {
							outbound["transport"] = map[string]any{"type": "quic"}
						}
						relayPort := stage5Port(t)
						udpTarget, udpPort := stage5UDPEcho(t)
						relay := map[string]any{"type": "direct", "listen": "127.0.0.1", "listen_port": relayPort, "network": "udp", "override_address": udpTarget, "override_port": udpPort}
						clientInbounds := []any{map[string]any{"type": "socks", "listen": "127.0.0.1", "listen_port": socksPort}, relay}
						if xray {
							clientInbounds = []any{map[string]any{"protocol": "socks", "listen": "127.0.0.1", "port": socksPort, "settings": map[string]any{"auth": "noauth", "udp": true}}, map[string]any{"protocol": "dokodemo-door", "listen": "127.0.0.1", "port": relayPort, "settings": map[string]any{"network": "udp", "address": udpTarget, "port": udpPort}}}
						}
						stage5Start(t, binary, dir, fmt.Sprint("client", user), map[string]any{"inbounds": clientInbounds, "outbounds": []any{outbound}}, socksPort, true)
						dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", socksPort), nil, proxy.Direct)
						if err != nil {
							t.Fatal(err)
						}
						stage5Data(t, "udp", fmt.Sprintf("127.0.0.1:%d", relayPort), user <= 2)

						for attempt := range 2 {
							ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
							target := echoAddress
							if attempt == 1 && user == 1 {
								target = v6
							}
							if attempt == 1 && user == 2 {
								target = net.JoinHostPort("localhost", echoPort)
							}
							conn, err := dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", target)
							if err == nil {
								_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
								payload := bytes.Repeat([]byte("stage5-roundtrip"), 64)
								_, err = conn.Write(payload)
								if err == nil {
									size := len(payload)
									if user >= 3 {
										size = 1
									} // Any returned byte is an authentication bypass.
									got := make([]byte, size)
									_, err = io.ReadFull(conn, got)
									if user <= 2 && err == nil && !bytes.Equal(got, payload) {
										err = fmt.Errorf("payload corrupted")
									}
								}
								_ = conn.Close()
							}
							cancel()
							if user <= 2 && err != nil {
								t.Fatal(err)
							}
							if user >= 3 && err == nil {
								t.Fatal("disabled or unknown user transferred data")
							}
							if user >= 3 {
								break
							}
						}
					})
				}
				if protocol == "trojan" && !xray {
					t.Run("wrong-SNI", func(t *testing.T) {
						badStream := maps.Clone(clientStream)
						tls := maps.Clone(clientStream["tlsSettings"].(map[string]any))
						tls["serverName"] = "wrong.invalid"
						badStream["tlsSettings"] = tls
						outbound, err := translateOut(map[string]any{"protocol": protocol, "tag": "proxy", "settings": map[string]any{"address": "127.0.0.1", "port": port, "password": "password1"}, "streamSettings": badStream})
						if err != nil {
							t.Fatal(err)
						}
						if network == "native-quic" {
							outbound["transport"] = map[string]any{"type": "quic"}
						}
						socksPort := stage5Port(t)
						stage5Start(t, binary, dir, "bad-sni", map[string]any{"inbounds": []any{map[string]any{"type": "socks", "listen": "127.0.0.1", "listen_port": socksPort}}, "outbounds": []any{outbound}}, socksPort, true)
						dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", socksPort), nil, proxy.Direct)
						if err != nil {
							t.Fatal(err)
						}
						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
						defer cancel()
						conn, err := dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", echoAddress)
						if err == nil {
							defer conn.Close()
							_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
							_, err = conn.Write([]byte("X"))
							if err == nil {
								var got [1]byte
								_, err = io.ReadFull(conn, got[:])
							}
						}
						if err == nil {
							t.Fatal("wrong certificate name allowed traffic")
						}
					})
				}
			})
		}
	}
}

func stage5Echo(t *testing.T, address string) string {
	t.Helper()
	l, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	return l.Addr().String()
}

func stage5Port(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func stage5Start(t *testing.T, binary, dir, name string, cfg map[string]any, port int, tcp bool) {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(dir, name+".log"))
	if err != nil {
		t.Fatal(err)
	}
	command := "srun"
	if os.Getenv("STAGE5_NATIVE_PROFILE") == "upstream" || os.Getenv("STAGE5_NATIVE_PROFILE") == "xray" {
		command = "run"
	}
	cmd := exec.Command(binary, command, "-c", path)
	cmd.Dir = filepath.Dir(binary)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		_ = log.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = log.Close()
		if t.Failed() {
			data, _ := os.ReadFile(log.Name())
			t.Log(string(data))
		}
	})
	if !tcp {
		time.Sleep(200 * time.Millisecond)
		return
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("native listener did not start")
}
