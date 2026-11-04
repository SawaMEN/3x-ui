package externalvpn

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"golang.org/x/net/proxy"
)

func TestStage8FPTNNativePayload(t *testing.T) {
	if os.Getenv("STAGE8_DOCKER") != "1" {
		t.Skip("requires disposable Docker runner and TUN")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	core := filepath.Join(os.Getenv("XUI_STAGE4_CORE_DIR"), "owner", "x-ui", "bin", "hiddify-core-linux-"+runtime.GOARCH)
	if err := os.Symlink(core, config.OwnerCoreBinaryPath()); err != nil {
		t.Fatal(err)
	}
	containers := []string{containerName(83)}
	t.Cleanup(func() {
		// Registered before process cleanups so this runs after both managers stop.
		for _, name := range containers {
			out, err := exec.Command("docker", "inspect", name).CombinedOutput()
			if err == nil || !strings.Contains(string(out), "No such") {
				t.Errorf("FPTN container survived cleanup: %s: %v %s", name, err, out)
			}
		}
	})
	port := stage8Port(t)
	raw, _ := json.Marshal(Settings{Hostname: "example.test", Image: DefaultFPTNImage, MetricsKey: strings.Repeat("a", 64), MaxSessions: 3, MTU: 1400, Clients: []Client{{Email: "alice", Password: "stage8-correct-secret-password-long", Enable: true}}})
	ib := &model.Inbound{Id: 83, Tag: "fptn-native", Protocol: model.FPTN, Listen: "0.0.0.0", Port: port, Settings: string(raw)}
	if err := PrepareAdditional(ib, ""); err != nil {
		t.Fatal(err)
	}
	inst, err := FromInbound(ib)
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{procs: map[int]*running{}, lastErrors: map[int]string{}}
	t.Cleanup(manager.StopAll)
	if err = manager.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(inst.Settings.CertificatePEM)) {
		t.Fatal("invalid server trust anchor")
	}
	block, _ := pem.Decode([]byte(inst.Settings.CertificatePEM))
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: inst.Settings.Hostname}); err != nil {
		t.Fatal("FPTN certificate hostname/trust", err)
	}
	// The client runs in its own namespace. Use the actual Docker bridge address,
	// not container loopback, as the public server/echo target in the fixture.
	gateway := os.Getenv("STAGE8_DOCKER_GATEWAY")
	if gateway == "" {
		t.Fatal("Docker gateway not supplied")
	}
	token, err := ExportAdditionalLink(inst, "alice", gateway, port, "stage8")
	if err != nil {
		t.Fatal(err)
	}
	oldPlans, oldProcesses := outboundPlans, outboundProcesses
	outboundPlans = map[string]*managedOutbound{}
	outboundProcesses = map[string]*managedOutbound{}
	t.Cleanup(func() { KeepOutbounds(nil); outboundPlans = oldPlans; outboundProcesses = oldProcesses })
	outbound := map[string]any{"tag": "fptn-native", "protocol": "fptn", "settings": map[string]any{"token": token, "mtu": 1400, "sni": "example.test"}}
	finish, err := ApplyOutbounds([]map[string]any{outbound})
	if err != nil {
		t.Fatal(err)
	}
	if err = finish(true); err != nil {
		t.Fatal(err)
	}
	proc := outboundProcesses["fptn-native"]
	containers = append(containers, proc.container)
	// Keep the echo distinct from the VPN server address and its bypass route.
	echoHost := stage8NonLoopbackIP(t)
	echo, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, e := echo.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	udp, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	go func() {
		b := make([]byte, 65535)
		for {
			n, a, e := udp.ReadFrom(b)
			if e != nil {
				return
			}
			udp.WriteTo(b[:n], a)
		}
	}()
	// The host side of this bridge is VLESS, exactly as rendered by PlanOutbound.
	bridge := outboundBridge("fptn-native", proc)
	if bridge["protocol"] != "vless" {
		t.Fatal("unexpected FPTN bridge", bridge["protocol"])
	}
	server := bridge["settings"].(map[string]any)["vnext"].([]any)[0].(map[string]any)
	user := server["users"].([]any)[0].(map[string]any)
	native := map[string]any{"type": "vless", "tag": bridge["tag"], "server": server["address"], "server_port": server["port"], "uuid": user["id"]}

	localPort, udpPort := stage8Port(t), stage8Port(t)
	path := filepath.Join(dir, "test-client.json")
	cfg := map[string]any{"inbounds": []any{map[string]any{"type": "socks", "tag": "test", "listen": "127.0.0.1", "listen_port": localPort}, map[string]any{"type": "direct", "tag": "test-udp", "listen": "127.0.0.1", "listen_port": udpPort, "network": "udp", "override_address": echoHost, "override_port": udp.LocalAddr().(*net.UDPAddr).Port}}, "outbounds": []any{native}, "route": map[string]any{"final": "fptn-native"}}
	data, _ := json.Marshal(cfg)
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := stage8StartCore(t, core, path, localPort)
	_ = cmd
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", fmtPort(localPort)), nil, &net.Dialer{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	payloadCtx, payloadCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer payloadCancel()
	c, err := dialer.(proxy.ContextDialer).DialContext(payloadCtx, "tcp", net.JoinHostPort(echoHost, fmtPort(echo.Addr().(*net.TCPAddr).Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(20 * time.Second))
	payload := bytes.Repeat([]byte("fptn-tunnel-payload"), 1024)
	if _, err = c.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	_, err = io.ReadFull(c, got)
	if err != nil || !bytes.Equal(payload, got) {
		t.Fatal("FPTN TCP payload", err)
	}
	c.Close()
	stage8FPTNUDPTrace(t, proc.container, udp.LocalAddr().(*net.UDPAddr).Port)
	stage8FPTNUDPTrace(t, containerName(inst.ID), udp.LocalAddr().(*net.UDPAddr).Port)
	packet, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", fmtPort(udpPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer packet.Close()
	packet.SetDeadline(time.Now().Add(15 * time.Second))
	udpPayload := []byte("stage8-native-udp")
	if _, err = packet.Write(udpPayload); err != nil {
		t.Fatal(err)
	}
	var udpReply [128]byte
	n, err := packet.Read(udpReply[:])
	if err != nil || !bytes.Equal(udpReply[:n], udpPayload) {
		t.Fatal("FPTN native UDP payload", n, err)
	}

	manager.Remove(inst.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	blocked, err := dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", net.JoinHostPort(echoHost, fmtPort(echo.Addr().(*net.TCPAddr).Port)))
	if err == nil {
		defer blocked.Close()
		blocked.SetDeadline(time.Now().Add(2 * time.Second))
		blocked.Write([]byte("forbidden"))
		var b [9]byte
		n, _ := io.ReadFull(blocked, b[:])
		if n > 0 {
			t.Fatal("dead FPTN tunnel escaped directly")
		}
	}

}

func TestStage8InstallFPTNClient(t *testing.T) {
	if os.Getenv("STAGE8_INSTALL_FPTN") != "1" {
		t.Skip("requires disposable Docker runner")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := InstallFPTNClient(ctx); err != nil {
		t.Fatal(err)
	}
}

// Optional packet metadata makes a native UDP failure diagnosable. No payload
// bytes are captured, and the root helper has its own fixed lifetime.
func stage8FPTNUDPTrace(t *testing.T, container string, port int) {
	t.Helper()
	if _, err := exec.LookPath("tcpdump"); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	pid, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Pid}}", container).Output()
	cancel()
	if err != nil {
		return
	}
	file, err := os.CreateTemp(t.TempDir(), "udp-metadata-")
	if err != nil {
		return
	}
	cmd := exec.Command("sudo", "-n", "timeout", "18", "nsenter", "-t", strings.TrimSpace(string(pid)), "-n", "tcpdump", "-nn", "-l", "-i", "any", "udp", "and", "port", strconv.Itoa(port))
	cmd.Stdout, cmd.Stderr = file, file
	if err := cmd.Start(); err != nil {
		file.Close()
		return
	}
	t.Cleanup(func() {
		_ = cmd.Wait()
		file.Close()
		if t.Failed() {
			output, _ := os.ReadFile(file.Name())
			t.Log("FPTN UDP metadata " + container + ":\n" + string(output))
		}
	})
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); {
		data, _ := os.ReadFile(file.Name())
		if bytes.Contains(data, []byte("listening on")) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}
