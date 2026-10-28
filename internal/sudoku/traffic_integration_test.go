package sudoku

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/clientlimit"

	"golang.org/x/net/proxy"
)

// Exercise the public CLI with the unmodified official server AND client.
// Global mode ensures even loopback echo targets traverse the Sudoku tunnel.
func TestOfficialSudokuTraffic(t *testing.T) { testSudokuTraffic(t, false) }

func TestSudokuSpeedLimits(t *testing.T) { testSudokuTraffic(t, true) }

func testSudokuTraffic(t *testing.T, limited bool) {
	executable := os.Getenv("SUDOKU_TEST_BINARY")
	if executable == "" {
		t.Skip("set SUDOKU_TEST_BINARY to an official release executable")
	}
	public, private, err := GenerateMasterKey(context.Background(), executable)
	if err != nil {
		t.Fatal(err)
	}
	key, err := GenerateSplitKey(context.Background(), executable, private)
	if err != nil {
		t.Fatal(err)
	}
	if limited {
		previous := clientlimit.Default
		clientlimit.Default = clientlimit.New("127.0.0.1:0", "127.0.0.1:0")
		defer func() { clientlimit.Default.Close(); clientlimit.Default = previous }()

		raw, err := hex.DecodeString(key)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		finish, err := clientlimit.Default.Apply(clientlimit.Plan{Routes: []clientlimit.Route{{ID: "sudoku-speed", Email: "alice", UploadMbps: 1, DownloadMbps: 1, Aliases: []string{"sudoku:" + hex.EncodeToString(digest[:8])}}}})
		if err != nil {
			t.Fatal(err)
		}
		finish(true)
		defer clientlimit.Default.Close()
	}
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tcp.Close() })
	go func() {
		for {
			c, e := tcp.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = udp.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			n, a, e := udp.ReadFrom(buf)
			if e != nil {
				return
			}
			_, _ = udp.WriteTo(buf[:n], a)
		}
	}()

	for _, tc := range []struct {
		name, mux string
		mask      bool
	}{
		{"TCP", "off", false}, {"MUX", "on", false}, {"HTTPMask", "on", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{Mode: "server", Transport: "tcp", LocalPort: freeSudokuPort(t), Key: public, AEAD: "chacha20-poly1305", SuspiciousAction: "silent", PaddingMin: 5, PaddingMax: 15, ASCII: "prefer_entropy", EnablePureDownlink: true, Multiplex: tc.mux, HTTPMask: HTTPMaskConfig{Disable: !tc.mask, Mode: "auto", Multiplex: tc.mux}}
			dir := t.TempDir()
			path, err := writeCandidateConfig(dir, 1, cfg)
			if err != nil {
				t.Fatal(err)
			}
			server := NewProcess(path, "official-traffic-server")
			t.Cleanup(func() {
				if err := server.Stop(); err != nil {
					t.Error(err)
				}
			})
			if err := server.Start(executable); err != nil {
				t.Fatal(err)
			}
			clientCfg := cfg
			clientCfg.Mode = "client"
			clientCfg.LocalPort = freeSudokuPort(t)
			clientCfg.ServerAddress = net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.LocalPort))
			clientCfg.Key = key
			data, err := json.Marshal(struct {
				Config
				RuleURLs []string `json:"rule_urls"`
			}{clientCfg, []string{"global"}})
			if err != nil {
				t.Fatal(err)
			}
			clientPath := filepath.Join(dir, "client.json")
			if err := os.WriteFile(clientPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			client := NewProcess(clientPath, "official-traffic-client")
			t.Cleanup(func() {
				if err := client.Stop(); err != nil {
					t.Error(err)
				}
			})
			if err := client.Start(executable); err != nil {
				t.Fatal(err)
			}
			socksAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(clientCfg.LocalPort))
			dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, &net.Dialer{Timeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ { // The second connection reuses the mux session when enabled.
				conn, err := dialer.Dial("tcp", tcp.Addr().String())
				if err != nil {
					t.Fatal(err)
				}
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				payload := []byte("official Sudoku TCP payload")
				if limited {
					payload = bytes.Repeat([]byte("rate"), 65536)
				}
				started := time.Now()
				sent := make(chan error, 1)
				go func() { _, err := conn.Write(payload); sent <- err }()
				got := make([]byte, len(payload))
				_, err = io.ReadFull(conn, got)
				if sendErr := <-sent; sendErr != nil {
					t.Fatal(sendErr)
				}
				if limited && time.Since(started) < time.Second {
					t.Fatal("Sudoku ignored its shared 1 Mbps cap")
				}
				conn.Close()
				if err != nil || !bytes.Equal(got, payload) {
					t.Fatalf("TCP echo: %q %v", got, err)
				}
			}
			officialSudokuUDPEcho(t, socksAddr, udp.LocalAddr().(*net.UDPAddr).Port)
		})
	}
}

func freeSudokuPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func officialSudokuUDPEcho(t *testing.T, socksAddr string, targetPort int) {
	t.Helper()
	ctrl, err := net.DialTimeout("tcp", socksAddr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	_ = ctrl.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := ctrl.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(ctrl, greeting); err != nil || !bytes.Equal(greeting, []byte{5, 0}) {
		t.Fatalf("SOCKS greeting: %v %v", greeting, err)
	}
	if _, err := ctrl.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(ctrl, reply); err != nil || reply[1] != 0 || reply[3] != 1 {
		t.Fatalf("UDP associate: %v %v", reply, err)
	}
	addr := &net.UDPAddr{IP: net.IP(reply[4:8]), Port: int(binary.BigEndian.Uint16(reply[8:]))}
	if addr.IP.IsUnspecified() {
		addr.IP = ctrl.RemoteAddr().(*net.TCPAddr).IP
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	payload := []byte("official Sudoku UDP payload")
	packet := append([]byte{0, 0, 0, 1, 127, 0, 0, 1, byte(targetPort >> 8), byte(targetPort)}, payload...)
	if _, err := conn.Write(packet); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf[:n], packet) {
		t.Fatalf("UDP echo: %v, want %v", buf[:n], packet)
	}
}
