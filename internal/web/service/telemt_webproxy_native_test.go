package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestNativeTelemtWebCarrierConfig(t *testing.T) {
	binary := os.Getenv("TELEMT_TEST_BINARY")
	if binary == "" {
		t.Skip("set TELEMT_TEST_BINARY for native WEB transport validation")
	}
	dir := t.TempDir()
	freePort := func() int {
		t.Helper()
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
		return port
	}
	rawPort, webPort := freePort(), freePort()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("web-decoy"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("[general]\ndata_path = %q\nuse_middle_proxy = false\n\n[general.modes]\nclassic = true\nsecure = true\ntls = false\n\n[network]\nipv4 = true\nipv6 = false\n\n[server]\nport = %d\n\n[[server.listeners]]\nip = \"127.0.0.1\"\n\n[access.users]\nxui = \"0123456789abcdef0123456789abcdef\"\n\n[[upstreams]]\ntype = \"direct\"\n", dir, rawPort)
	config, err := appendTelemtWebProxyConfig(base, TelemtWebProxyState{Enabled: true, Domain: "example.com", Secret: "abcdef0123456789abcdef0123456789", PublicAddr: "203.0.113.1:443", ListenPort: webPort, DecoyDir: dir, Carrier: "https", Carriers: []string{"websocket-lanes", "websocket", "https-lanes", "https"}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "telemt.toml")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "run", path)
	cmd.Dir = dir
	log, err := os.Create(filepath.Join(dir, "telemt.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	defer func() { _ = cmd.Process.Kill(); <-exited }()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(25 * time.Second)
	for {
		select {
		case <-exited:
			logs, _ := os.ReadFile(log.Name())
			t.Fatalf("Telemt exited before WEB startup: %s", logs)
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, "http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(webPort))+"/", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "example.com"
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			logs, _ := os.ReadFile(log.Name())
			t.Fatalf("native Telemt rejected WEB carriers or did not start: %v\n%s", err, logs)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
