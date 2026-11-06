package dnstt

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateDNSTT(t *testing.T) {
	for _, domain := range []string{"t.example.com", "t.sub.example.com", "t-example.example.com"} {
		c := Defaults()
		c.Domain = domain
		if err := Validate(c); err != nil {
			t.Errorf("rejected %s: %v", domain, err)
		}
		if !strings.Contains(serviceUnit(c), domain+" 127.0.0.1:22") {
			t.Fatal("unit dropped target")
		}
	}
	for _, tc := range []Config{
		{Domain: ""},
		{Domain: "example"},
		{Domain: "bad space.example.com"},
		{Domain: "-t.example.com"},
		{Domain: "t.example.com", ListenPort: 65536, TargetPort: 22},
		{Domain: "t.example.com", ListenPort: 53, TargetPort: 65536},
		{Domain: "t.example.com", ListenPort: 53, TargetPort: 22, ServerIP: "not-an-ip"},
	} {
		if err := Validate(tc); err == nil {
			t.Errorf("accepted invalid settings: %+v", tc)
		}
	}
}

func TestDNSTTDefaultsAndUnitSafety(t *testing.T) {
	c := Defaults()
	if c.ListenPort != 53 || c.TargetPort != 22 || c.Enabled {
		t.Fatalf("unsafe defaults: %+v", c)
	}
	c.Domain = "t.example.com"
	unit := serviceUnit(c)
	for _, key := range []string{"-udp :53", "-privkey-file " + privatePath, "127.0.0.1:22", "NoNewPrivileges=true", "CAP_NET_BIND_SERVICE"} {
		if !strings.Contains(unit, key) {
			t.Fatalf("unit missing %s", key)
		}
	}
	if strings.Contains(unit, "0.0.0.0:22") {
		t.Fatal("forward target must always be localhost")
	}
}

func TestOfficialDNSTTTunnelRoundTrip(t *testing.T) {
	server := os.Getenv("DNSTT_SERVER_BINARY")
	client := os.Getenv("DNSTT_CLIENT_BINARY")
	if server == "" || client == "" {
		t.Skip("requires pinned DNSTT server and client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	temp := t.TempDir()
	private, pub := filepath.Join(temp, "private.key"), filepath.Join(temp, "public.key")
	key := exec.CommandContext(ctx, server, "-gen-key", "-privkey-file", private, "-pubkey-file", pub)
	if output, err := key.CombinedOutput(); err != nil {
		t.Fatalf("official key generation: %v: %s", err, output)
	}
	publicKey, err := os.ReadFile(pub)
	if err != nil || len(strings.TrimSpace(string(publicKey))) != 64 {
		t.Fatalf("invalid generated public key: %v", err)
	}
	// Echo service represents an existing authenticated local TCP target.
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		conn, err := echo.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udpAddress := udp.LocalAddr().String()
	udp.Close()
	local, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clientAddress := local.Addr().String()
	local.Close()
	srv := exec.CommandContext(ctx, server, "-udp", udpAddress, "-privkey-file", private, "t.example.com", echo.Addr().String())
	serverLog, err := os.Create(filepath.Join(temp, "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer serverLog.Close()
	srv.Stdout = serverLog
	srv.Stderr = serverLog
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Process.Kill(); _ = srv.Wait() }()
	cli := exec.CommandContext(ctx, client, "-udp", udpAddress, "-pubkey-file", pub, "t.example.com", clientAddress)
	clientLog, err := os.Create(filepath.Join(temp, "client.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer clientLog.Close()
	cli.Stdout = clientLog
	cli.Stderr = clientLog
	if err := cli.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cli.Process.Kill(); _ = cli.Wait() }()
	var conn net.Conn
	for i := 0; i < 28; i++ {
		conn, err = net.DialTimeout("tcp", clientAddress, 250*time.Millisecond)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(150 * time.Millisecond):
		}
	}
	if err != nil {
		t.Fatalf("DNSTT client did not listen: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(22 * time.Second)); err != nil {
		t.Fatal(err)
	}
	payload := []byte("dnstt-round-trip-2026")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, response); err != nil {
		sl, _ := os.ReadFile(serverLog.Name())
		cl, _ := os.ReadFile(clientLog.Name())
		t.Fatalf("real DNSTT tunnel did not transmit stream: %v\nserver: %s\nclient: %s", err, sl, cl)
	}
	if string(payload) != string(response) {
		t.Fatalf("corrupted DNSTT data: got %q", response)
	}
}

func TestBinaryReadiness(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary")
	if binaryReady(path) || binaryReady(dir) {
		t.Fatal("missing file or directory accepted")
	}
	for _, tc := range []struct {
		content string
		mode    os.FileMode
		ready   bool
	}{
		{"", 0o755, false}, {"binary", 0o644, false}, {"binary", 0o755, true},
	} {
		if err := os.WriteFile(path, []byte(tc.content), tc.mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, tc.mode); err != nil {
			t.Fatal(err)
		}
		if got := binaryReady(path); got != tc.ready {
			t.Fatalf("mode %o, contents %q: ready=%v", tc.mode, tc.content, got)
		}
	}
}

func TestSetupRequiresInstalledBinary(t *testing.T) {
	if binaryReady(binaryPath) {
		t.Skip("component already installed")
	}
	err := Setup(context.Background(), Config{Domain: "t.example.com", ListenPort: 53, TargetPort: 22})
	if err == nil || !strings.Contains(err.Error(), "сначала установите DNSTT") {
		t.Fatalf("expected installation prerequisite, got %v", err)
	}
}
