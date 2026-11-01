package hiddify

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPinnedProtocolTransportMatrix(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_CONFIG_CHECK_BINARY for modified Sing-box contract checks")
	}
	binary, _ = filepath.Abs(binary)
	dir := t.TempDir()
	cert, key := testCertificate(t, dir)
	for _, protocol := range []string{"vless", "vmess", "trojan"} {
		for _, network := range []string{"tcp", "ws", "grpc", "httpupgrade", "http", "xhttp", "raw", "quic", "kcp"} {
			for _, inbound := range []bool{false, true} {
				direction := "outbound"
				if inbound {
					direction = "inbound"
				}
				t.Run(protocol+"/"+network+"/"+direction, func(t *testing.T) {
					settings := map[string]any{}
					user := map[string]any{"id": "2a547fe4-5b0f-4a84-845c-131464875bf8", "password": "secret", "email": "alice", "enable": true}
					if inbound {
						settings["clients"] = []any{user}
						settings["decryption"] = "none"
					} else {
						settings["vnext"] = []any{map[string]any{"address": "127.0.0.1", "port": 443, "users": []any{user}}}
						settings["servers"] = []any{map[string]any{"address": "127.0.0.1", "port": 443, "password": "secret"}}
					}
					if protocol != "vless" {
						delete(settings, "decryption")
					}
					if protocol == "trojan" {
						delete(settings, "vnext")
					} else {
						delete(settings, "servers")
					}
					streamNetwork := network
					if network == "raw" || network == "quic" {
						streamNetwork = "tcp"
					}
					stream := map[string]any{"network": streamNetwork, "security": "tls", "tlsSettings": map[string]any{"serverName": "localhost"}}
					if inbound {
						stream["tlsSettings"] = map[string]any{"certificates": []any{map[string]any{"certificateFile": cert, "keyFile": key}}}
					}
					switch network {
					case "ws":
						stream["wsSettings"] = map[string]any{"path": "/ws"}
					case "grpc":
						stream["grpcSettings"] = map[string]any{"serviceName": "relay"}
					case "httpupgrade":
						stream["httpupgradeSettings"] = map[string]any{"path": "/upgrade"}
					case "http":
						stream["httpSettings"] = map[string]any{"path": "/h2"}
					case "xhttp":
						stream["xhttpSettings"] = map[string]any{"path": "/x", "mode": "auto"}
					}
					raw := map[string]any{"protocol": protocol, "tag": direction, "settings": settings, "streamSettings": stream, "listen": "127.0.0.1", "port": 8443}
					var translated map[string]any
					var err error
					var cfg map[string]any
					if inbound {
						translated, err = TranslateInbound(raw)
						cfg = map[string]any{"inbounds": []any{translated}}
					} else {
						translated, err = TranslateOutbound(raw)
						cfg = map[string]any{"outbounds": []any{translated}}
					}
					if err != nil {
						t.Fatal(err)
					}
					if network == "raw" || network == "quic" {
						if !inbound {
							translated["type"] = protocol
							translated["transport"] = map[string]any{"type": network}
							translated, err = TranslateOutbound(translated)
							if err != nil {
								t.Fatal(err)
							}
							cfg["outbounds"] = []any{translated}
						} else {
							translated["transport"] = map[string]any{"type": network}
						}
					}
					data, err := json.Marshal(cfg)
					if err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(t.TempDir(), "config.json")
					if err := os.WriteFile(path, data, 0o600); err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					command := exec.CommandContext(ctx, binary, "check", "-c", path)
					command.Dir = filepath.Dir(binary)
					if output, err := command.CombinedOutput(); err != nil {
						t.Fatalf("modified Sing-box check: %v: %s\n%s", err, output, data)
					}
				})
			}
		}
	}
}
