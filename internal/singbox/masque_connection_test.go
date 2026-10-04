package singbox

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the generated server and an outgoing endpoint, not just JSON parsing.
// Direct relay inbounds avoid relying on a platform TUN or external network.
func TestMASQUEConnections(t *testing.T) {
	binary := os.Getenv("SINGBOX_TEST_BINARY")
	if binary == "" {
		t.Skip("set SINGBOX_TEST_BINARY for CONNECT-IP TCP/UDP checks")
	}
	certPath, keyPath := masqueTestCertificate(t)
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	targetPort := echo.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { defer conn.Close(); _, _ = io.Copy(conn, conn) }()
		}
	}()
	udpEcho, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", targetPort))
	if err != nil {
		t.Fatal(err)
	}
	defer udpEcho.Close()
	go func() {
		buffer := make([]byte, 2048)
		for {
			n, addr, err := udpEcho.ReadFrom(buffer)
			if err != nil {
				return
			}
			_, _ = udpEcho.WriteTo(buffer[:n], addr)
		}
	}()
	for _, version := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("HTTP%d", version), func(t *testing.T) {
			port, relayPort := masqueFreePort(t), masqueFreePort(t)
			path := "/.well-known/masque/ip/{target}/{ipproto}/"
			if version == 2 {
				path = "/tunnel{?target,ipproto}"
			}
			server, err := TranslateMASQUEEndpoint(map[string]any{"protocol": "masque", "tag": "server", "listen": "127.0.0.1", "port": port, "settings": map[string]any{"version": []int{version}, "path": path, "tls": map[string]any{"certificatePath": certPath, "keyPath": keyPath}, "clients": []any{map[string]any{"email": "alice", "password": "secret", "enable": true}}}})
			if err != nil {
				t.Fatal(err)
			}
			serverLog := startMASQUETestProcess(t, binary, "server", map[string]any{"endpoints": []any{server}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": map[string]any{"final": "direct"}})
			// Explicit TLS trust and no version fallback ensure each transport is tested.
			clientLog := startMASQUETestProcess(t, binary, "client", map[string]any{
				"endpoints": []any{map[string]any{"type": "masque-client", "tag": "proxy", "server": "127.0.0.1", "server_port": port, "path": path, "version": version, "disable_version_fallback": true, "username": "alice", "password": "secret", "tls": map[string]any{"enabled": true, "server_name": "localhost", "certificate_path": certPath}}},
				"inbounds":  []any{map[string]any{"type": "direct", "tag": "relay", "listen": "127.0.0.1", "listen_port": relayPort, "override_address": "127.0.0.1", "override_port": targetPort}},
				"route":     map[string]any{"final": "proxy"},
			})
			address := fmt.Sprintf("127.0.0.1:%d", relayPort)
			deadline := time.Now().Add(15 * time.Second)
			for {
				err = masqueEcho(address, "tcp")
				if err == nil {
					break
				}
				serverOutput, _ := os.ReadFile(serverLog)
				clientOutput, _ := os.ReadFile(clientLog)
				for _, output := range [][]byte{serverOutput, clientOutput} {
					if strings.Contains(string(output), "create netlink socket: operation not permitted") {
						t.Skip("runtime connection check requires a platform allowing sing-box's netlink network monitor")
					}
					if strings.Contains(string(output), "FATAL") {
						t.Fatalf("MASQUE runtime failed: %s", output)
					}
				}
				if time.Now().After(deadline) {
					t.Fatalf("TCP through MASQUE: %v\nserver: %s\nclient: %s", err, serverOutput, clientOutput)
				}
				time.Sleep(50 * time.Millisecond)
			}
			if err := masqueEcho(address, "udp"); err != nil {
				t.Fatalf("UDP through MASQUE: %v", err)
			}
		})
	}
}

func masqueEcho(address, network string) error {
	conn, err := net.DialTimeout(network, address, time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	payload := "MASQUE CONNECT-IP echo"
	if _, err := io.WriteString(conn, payload); err != nil {
		return err
	}
	buffer := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, buffer); err != nil {
		return err
	}
	if string(buffer) != payload {
		return fmt.Errorf("unexpected echo %q", buffer)
	}
	return nil
}

func masqueFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func startMASQUETestProcess(t *testing.T, binary, name string, cfg map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, name+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "run", "-c", path)
	cmd.Dir = filepath.Dir(binary)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait(); _ = logFile.Close() })
	return logPath
}
