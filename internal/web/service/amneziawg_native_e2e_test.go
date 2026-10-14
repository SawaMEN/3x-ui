package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	"github.com/amnezia-vpn/amneziawg-go/v3/tun/netstack"

	"github.com/SawaMEN/3x-ui/v3/internal/amneziawg"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/util/wireguard"
)

// Native outbound -> real release core -> AWG v3 server -> TCP/UDP echo.
// No root privileges, public network or system TUN are needed.
func TestNativeAWGConnections(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_CONFIG_CHECK_BINARY for native AWG TCP/UDP checks")
	}
	serverPrivate, serverPublic, err := wireguard.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	clientPrivate, clientPublic, err := wireguard.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	protectionKey, _, err := wireguard.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	serverHex, _ := wireguard.KeyToHex(serverPrivate)
	clientHex, _ := wireguard.KeyToHex(clientPublic)
	protectionHex, _ := wireguard.KeyToHex(protectionKey)
	tun, stack, err := netstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.8.1.1")}, nil, 1408)
	if err != nil {
		t.Fatal(err)
	}
	server := device.NewDevice(tun, nativeAWGTestBind{conn.NewDefaultBind()}, device.NewLogger(device.LogLevelSilent, ""))
	t.Cleanup(server.Close)
	ipc := fmt.Sprintf("private_key=%s\nlisten_port=0\ns1=20\ns2=30\ns3=12\ns4=12\nh1=101\nh2=102\nh3=103\nh4=104\nheader_protection_key=%s\ncontent_padding_addition=8-16\npublic_key=%s\nallowed_ip=10.8.1.2/32\n", serverHex, protectionHex, clientHex)
	if err := server.IpcSet(ipc); err != nil {
		t.Fatal(err)
	}
	if err := server.Up(); err != nil {
		t.Fatal(err)
	}
	dump, err := server.IpcGet()
	if err != nil {
		t.Fatal(err)
	}
	port := ""
	for _, line := range strings.Split(dump, "\n") {
		if value, ok := strings.CutPrefix(line, "listen_port="); ok {
			port = value
		}
	}
	if port == "" || port == "0" {
		t.Fatal("AWG server did not bind a UDP port")
	}
	tcpEcho, err := stack.ListenTCPAddrPort(netip.MustParseAddrPort("10.8.1.1:8080"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tcpEcho.Close() })
	go func() {
		for {
			c, err := tcpEcho.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	udpEcho, err := stack.ListenUDPAddrPort(netip.MustParseAddrPort("10.8.1.1:8080"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = udpEcho.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := udpEcho.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = udpEcho.WriteTo(buf[:n], from)
		}
	}()
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	relayPort := free.Addr().(*net.TCPAddr).Port
	_ = free.Close()
	raw, _ := json.Marshal(map[string]any{"protocol": "amneziawg", "tag": "native-awg", "settings": map[string]any{
		"native": true, "secretKey": clientPrivate, "address": []string{"10.8.1.2/32"}, "s1": 20, "s2": 30, "s3": 12, "s4": 12,
		"h1": "101", "h2": "102", "h3": "103", "h4": "104", "i1": "<r 32>", "headerProtectionKey": protectionKey, "contentPaddingAddition": "8-16",
		"peers": []any{map[string]any{"publicKey": serverPublic, "endpoint": "127.0.0.1:" + port, "allowedIPs": []string{"0.0.0.0/0"}}},
	}})
	native, err := amneziawg.NativeOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, _, err := singbox.TranslatePanelEndpoint(native)
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"log": map[string]any{"level": "warn"}, "endpoints": []any{endpoint}, "inbounds": []any{map[string]any{"type": "direct", "tag": "relay", "listen": "127.0.0.1", "listen_port": relayPort, "override_address": "10.8.1.1", "override_port": 8080}}, "route": map[string]any{"final": "native-awg"}}
	data, _ := json.Marshal(config)
	path := filepath.Join(t.TempDir(), "client.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.OpenFile(filepath.Join(t.TempDir(), "core.log"), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logFile.Close() })
	command := exec.Command(binary, "srun", "-c", path)
	command.Dir = filepath.Dir(binary)
	command.Stdout, command.Stderr = logFile, logFile
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	for _, network := range []string{"tcp", "udp"} {
		t.Run(network, func(t *testing.T) {
			deadline := time.Now().Add(12 * time.Second)
			var last error
			for time.Now().Before(deadline) {
				last = nativeAWGEcho(network, net.JoinHostPort("127.0.0.1", strconv.Itoa(relayPort)))
				if last == nil {
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			log, _ := os.ReadFile(logFile.Name())
			t.Fatalf("native AWG %s round-trip failed: %v; core log: %s", network, last, log)
		})
	}
}

func nativeAWGEcho(network, address string) error {
	c, err := net.DialTimeout(network, address, time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	payload := []byte("native-awg-echo")
	if _, err := c.Write(payload); err != nil {
		return err
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(c, got); err != nil {
		return err
	}
	if !bytes.Equal(got, payload) {
		return fmt.Errorf("echo payload mismatch")
	}
	return nil
}

// The echo server only needs UDP sockets. Wrapping the interface avoids the
// optional Linux sticky-route listener, which needs a system netlink socket.
type nativeAWGTestBind struct{ conn.Bind }
