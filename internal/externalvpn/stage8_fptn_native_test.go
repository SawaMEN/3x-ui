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
	// A remote network namespace avoids Docker gateway hairpin SNAT on UDP replies.
	// It also verifies both protocols against the same independent echo host.
	echoHost, tcpEchoPort, udpEchoPort := stage8RemoteEcho(t)
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
	cfg := map[string]any{"inbounds": []any{map[string]any{"type": "socks", "tag": "test", "listen": "127.0.0.1", "listen_port": localPort}, map[string]any{"type": "direct", "tag": "test-udp", "listen": "127.0.0.1", "listen_port": udpPort, "network": "udp", "override_address": echoHost, "override_port": udpEchoPort}}, "outbounds": []any{native}, "route": map[string]any{"final": "fptn-native"}}
	data, _ := json.Marshal(cfg)
	if err = os.WriteFile(path, data, 0o600); err != nil {
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
	c, err := dialer.(proxy.ContextDialer).DialContext(payloadCtx, "tcp", net.JoinHostPort(echoHost, fmtPort(tcpEchoPort)))
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
	blocked, err := dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", net.JoinHostPort(echoHost, fmtPort(tcpEchoPort)))
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

// stage8RemoteEcho serves TCP and UDP from a separate network namespace instead
// of the runner host. Docker's host-to-container UDP hairpin would otherwise
// rewrite the source to the bridge gateway, which the FPTN client deliberately
// excludes from its tunnel to keep the transport connection alive.
func stage8RemoteEcho(t *testing.T) (string, int, int) {
	t.Helper()
	suffix := strconv.Itoa(os.Getpid())
	ns := "s8fptn-" + suffix
	hostLink, peerLink := "s8fh"+suffix, "s8fn"+suffix
	const remoteIP = "198.18.240.2" // RFC 2544 benchmark range; never public internet.
	runIP := func(args ...string) {
		t.Helper()
		output, err := exec.Command("sudo", append([]string{"ip"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("configure isolated echo namespace (%s): %v: %s", strings.Join(args, " "), err, output)
		}
	}
	runIP("netns", "add", ns)
	t.Cleanup(func() {
		if output, err := exec.Command("sudo", "ip", "netns", "delete", ns).CombinedOutput(); err != nil {
			t.Errorf("remove isolated echo namespace: %v: %s", err, output)
		}
	})
	runIP("link", "add", hostLink, "type", "veth", "peer", "name", peerLink)
	runIP("link", "set", peerLink, "netns", ns)
	runIP("addr", "add", "198.18.240.1/30", "dev", hostLink)
	runIP("link", "set", hostLink, "up")
	runIP("-n", ns, "addr", "add", remoteIP+"/30", "dev", peerLink)
	runIP("-n", ns, "link", "set", "lo", "up")
	runIP("-n", ns, "link", "set", peerLink, "up")

	tcpPort, udpPort := stage8Port(t), stage8Port(t)
	script := filepath.Join(t.TempDir(), "echo.py")
	program := strings.Join([]string{
		"import socket, sys, threading",
		"host, tcp_port, udp_port = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])",
		"tcp = socket.socket(socket.AF_INET, socket.SOCK_STREAM)",
		"tcp.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)",
		"tcp.bind((host, tcp_port))",
		"tcp.listen()",
		"udp = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)",
		"udp.bind((host, udp_port))",
		"def echo_tcp(conn):",
		"    with conn:",
		"        while data := conn.recv(65535):",
		"            conn.sendall(data)",
		"def accept_tcp():",
		"    while True:",
		"        conn, _ = tcp.accept()",
		"        threading.Thread(target=echo_tcp, args=(conn,), daemon=True).start()",
		"threading.Thread(target=accept_tcp, daemon=True).start()",
		"while True:",
		"    data, peer = udp.recvfrom(65535)",
		"    udp.sendto(data, peer)",
	}, "\n") + "\n"
	if err := os.WriteFile(script, []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sudo", "ip", "netns", "exec", ns, "python3", "-u", script, remoteIP, strconv.Itoa(tcpPort), strconv.Itoa(udpPort))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// sudo/ip can fork: terminate all tasks in the disposable namespace,
		// then reap the launched command before removing its veth pair.
		if output, err := exec.Command("sudo", "ip", "netns", "pids", ns).Output(); err == nil {
			if pids := strings.Fields(string(output)); len(pids) != 0 {
				_ = exec.Command("sudo", append([]string{"kill", "-TERM"}, pids...)...).Run()
			}
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	tcpAddress := net.JoinHostPort(remoteIP, strconv.Itoa(tcpPort))
	ready := false
	for attempt := 0; attempt < 30; attempt++ {
		conn, err := net.DialTimeout("tcp", tcpAddress, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("isolated TCP echo not ready: %s", stderr.String())
	}
	probe, err := net.Dial("udp", net.JoinHostPort(remoteIP, strconv.Itoa(udpPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	ready = false
	for attempt := 0; attempt < 15; attempt++ {
		_ = probe.SetDeadline(time.Now().Add(200 * time.Millisecond))
		if _, err = probe.Write([]byte("ready")); err == nil {
			var reply [16]byte
			var n int
			n, err = probe.Read(reply[:])
			if err == nil && bytes.Equal(reply[:n], []byte("ready")) {
				ready = true
				break
			}
		}
	}
	if !ready {
		t.Fatalf("isolated UDP echo not reachable directly: %v (%s)", err, stderr.String())
	}
	return remoteIP, tcpPort, udpPort
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
