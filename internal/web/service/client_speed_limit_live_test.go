package service

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	"github.com/SawaMEN/3x-ui/v3/internal/clientlimit"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Exercise the actual core -> limiter -> core path, including authenticated UDP associations.
func TestClientSpeedLimitLiveCore(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_CONFIG_CHECK_BINARY to run live traffic shaping")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	go func() {
		buf := make([]byte, 65536)
		for {
			n, addr, err := udp.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = udp.WriteTo(buf[:n], addr)
		}
	}()
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	_ = reserved.Close()
	cfg := singbox.NewConfig()
	cfg.Log = map[string]any{"level": "warn"}
	cfg.Inbounds = []map[string]any{{"type": "mixed", "tag": "public", "listen": "127.0.0.1", "listen_port": port, "users": []map[string]any{{"username": "alice", "password": "pass"}, {"username": "bob", "password": "pass"}}}}
	cfg.Route["rules"] = []map[string]any{{"port": []int{1}, "action": "reject"}}
	clients := map[int][]model.Client{1: {{Email: "alice", Enable: true, SpeedLimit: &model.ClientSpeedLimit{Enabled: true, UploadMbps: 1, DownloadMbps: 1}}}}
	if err = injectClientSpeedLimits(cfg, clients, []*model.Inbound{{Id: 1, Enable: true, Protocol: model.Mixed, Tag: "public"}}); err != nil {
		t.Fatal(err)
	}
	if err = hiddify.FinalizeServerConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Experimental = nil
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "speed.json")
	if err = os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
		t.Fatalf("core rejected generated config: %v\n%s", err, out)
	}
	finish, err := clientlimit.Default.Apply(cfg.SpeedLimitPlan)
	if err != nil {
		t.Fatal(err)
	}
	finish(true)
	defer clientlimit.Default.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log, err := os.CreateTemp(t.TempDir(), "core-log")
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, binary, "srun", "-c", path)
	cmd.Stdout = log
	cmd.Stderr = log
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for deadline := time.Now().Add(10 * time.Second); ; {
		c, e := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if e == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			out, _ := os.ReadFile(log.Name())
			t.Fatalf("core did not start: %s", out)
		}
		time.Sleep(20 * time.Millisecond)
	}
	data = bytes.Repeat([]byte{'t'}, 256*1024)
	for _, user := range []string{"alice", "bob"} {
		dial, e := proxy.SOCKS5("tcp", address, &proxy.Auth{User: user, Password: "pass"}, proxy.Direct)
		if e != nil {
			t.Fatal(e)
		}
		c, e := dial.Dial("tcp", ln.Addr().String())
		if e != nil {
			t.Fatal(e)
		}
		_ = c.SetDeadline(time.Now().Add(10 * time.Second))
		started := time.Now()
		go func() { _, _ = c.Write(data) }()
		received := make([]byte, len(data))
		_, e = io.ReadFull(c, received)
		_ = c.Close()
		if e != nil || !bytes.Equal(received, data) {
			t.Fatalf("live %s TCP transfer: %v", user, e)
		}
		elapsed := time.Since(started)
		t.Logf("%s: 256 KiB round trip in %v", user, elapsed)
		if user == "alice" && elapsed < 1400*time.Millisecond {
			t.Fatal("real core bypassed the client rate cap")
		}
		if user == "bob" && elapsed > time.Second {
			t.Fatal("unlimited client throttled")
		}
	}
	c, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	_, _ = c.Write([]byte{5, 1, 2})
	reply := make([]byte, 2)
	if _, err = io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	_, _ = c.Write([]byte{1, 5, 'a', 'l', 'i', 'c', 'e', 4, 'p', 'a', 's', 's'})
	if _, err = io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	_, _ = c.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0})
	reply = make([]byte, 10)
	if _, err = io.ReadFull(c, reply); err != nil || reply[1] != 0 {
		t.Fatalf("UDP associate failed: %v %v", reply, err)
	}
	u, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(reply[8])<<8 | int(reply[9])})
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	_ = u.SetDeadline(time.Now().Add(10 * time.Second))
	targetPort := udp.LocalAddr().(*net.UDPAddr).Port
	packet := append([]byte{0, 0, 0, 1, 127, 0, 0, 1, byte(targetPort >> 8), byte(targetPort)}, bytes.Repeat([]byte{'u'}, 16384)...)
	started := time.Now()
	buf := make([]byte, 65536)
	for range 16 {
		if _, err = u.Write(packet); err != nil {
			t.Fatal(err)
		}
		n, e := u.Read(buf)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(buf[:n], packet) {
			t.Fatal("live UDP payload changed")
		}
	}
	t.Logf("alice: 256 KiB UDP round trip in %v", time.Since(started))
	if time.Since(started) < 1400*time.Millisecond {
		t.Fatal("real UDP associations bypassed cap")
	}
}
