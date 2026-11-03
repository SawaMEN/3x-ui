package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func stage7KernelOnly(t *testing.T) {
	t.Helper()
	if os.Getenv("STAGE7_NETNS") != "1" {
		t.Skip("requires isolated root network/mount namespace")
	}
	self, e1 := os.Readlink("/proc/self/ns/net")
	host, e2 := os.Readlink("/proc/1/ns/net")
	if os.Geteuid() != 0 || e1 != nil || e2 != nil || self == host {
		t.Fatal("refusing kernel mutations outside a private network namespace")
	}
}

func stage7Cmd(t *testing.T, name string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v %s", name, args, err, out)
	}
	return string(out)
}

func stage7World(t *testing.T) (string, string) {
	t.Helper()
	stage7KernelOnly(t)
	client, target := fmt.Sprintf("s7c%d", os.Getpid()), fmt.Sprintf("s7t%d", os.Getpid())
	for _, ns := range []string{client, target} {
		stage7Cmd(t, "ip", "netns", "add", ns)
		t.Cleanup(func() { _ = exec.Command("ip", "netns", "del", ns).Run() })
	}
	for _, v := range []struct{ host, peer, ns, hostIP, peerIP string }{{"gw0", "c0", client, "10.60.0.1/24", "10.60.0.2/24"}, {"wan0", "t0", target, "198.18.0.1/24", "198.18.0.2/24"}} {
		stage7Cmd(t, "ip", "link", "add", v.host, "type", "veth", "peer", "name", v.peer)
		t.Cleanup(func() { _ = exec.Command("ip", "link", "del", v.host).Run() })
		stage7Cmd(t, "ip", "link", "set", v.peer, "netns", v.ns)
		stage7Cmd(t, "ip", "addr", "add", v.hostIP, "dev", v.host)
		stage7Cmd(t, "ip", "link", "set", v.host, "up")
		stage7Cmd(t, "ip", "-n", v.ns, "addr", "add", v.peerIP, "dev", v.peer)
		stage7Cmd(t, "ip", "-n", v.ns, "link", "set", v.peer, "up")
		stage7Cmd(t, "ip", "-n", v.ns, "link", "set", "lo", "up")
		gateway, _, _ := strings.Cut(v.hostIP, "/")
		stage7Cmd(t, "ip", "-n", v.ns, "route", "add", "default", "via", gateway)
	}
	stage7Cmd(t, "sysctl", "-w", "net.ipv4.ip_forward=0")
	stage7Cmd(t, "ip", "-n", target, "addr", "add", "203.0.114.2/32", "dev", "lo")
	dir := t.TempDir()
	script := filepath.Join(dir, "echo.py")
	ready := filepath.Join(dir, "ready")
	code := `import socket,threading,pathlib,sys
tcp=socket.socket();tcp.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);tcp.bind(('0.0.0.0',18080));tcp.listen()

def connection(c):
 try:
  while True:
   b=c.recv(65535)
   if not b: break
   c.sendall(b)
 finally: c.close()
def datagrams(udp):
 while True:
  b,a=udp.recvfrom(65535);udp.sendto(b,a)
# A connected UDP client only accepts replies from its destination address.
for address in ('198.18.0.2','203.0.114.2'):
 udp=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);udp.bind((address,18080))
 threading.Thread(target=datagrams,args=(udp,),daemon=True).start()
pathlib.Path(sys.argv[1]).write_text('ready')
while True:
 c,a=tcp.accept();threading.Thread(target=connection,args=(c,),daemon=True).start()
`
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("ip", "netns", "exec", target, "python3", script, ready)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	for end := time.Now().Add(3 * time.Second); ; {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(end) {
			t.Fatal("namespace echo server not ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	l, err := net.Listen("tcp", "10.60.0.1:2222")
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
			go func() { defer c.Close(); _ = c.SetDeadline(time.Now().Add(3 * time.Second)); _, _ = io.Copy(c, c) }()
		}
	}()
	return client, target
}

func stage7Packet(t *testing.T, ns, network, address string, port int, allowed bool) {
	t.Helper()
	code := `import socket,sys
s=socket.socket(socket.AF_INET,socket.SOCK_STREAM if sys.argv[1]=='tcp' else socket.SOCK_DGRAM);s.settimeout(2)
try:
 s.connect((sys.argv[2],int(sys.argv[3])));s.sendall(b'stage7-packet');b=s.recv(100);print(b.decode(),flush=True);assert b==b'stage7-packet'
finally: s.close()
`
	args := []string{"-c", code, network, address, strconv.Itoa(port)}
	name := "python3"
	if ns != "" {
		name = "ip"
		args = append([]string{"netns", "exec", ns, "python3"}, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if allowed && (err != nil || !strings.Contains(string(out), "stage7-packet")) {
		t.Fatalf("packet failed %s %s:%d: %v %s", network, address, port, err, out)
	}
	if !allowed && strings.Contains(string(out), "stage7-packet") {
		t.Fatalf("direct fallback leaked payload: %s", out)
	}
}

func stage7KernelCore(t *testing.T, cfg map[string]any) func() {
	t.Helper()
	root := os.Getenv("XUI_STAGE4_CORE_DIR")
	if root == "" {
		t.Fatal("missing pinned native core")
	}
	binary := filepath.Join(root, "owner", "x-ui", "bin", "hiddify-core-linux-"+runtime.GOARCH)
	dir := t.TempDir()
	path := filepath.Join(dir, "kernel.json")
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(dir, "core.log"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "srun", "-c", path)
	cmd.Dir = filepath.Dir(binary)
	cmd.Stdout, cmd.Stderr = log, log
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Error("core did not clean up kernel state on shutdown")
			}
			_ = log.Close()
		})
	}
	t.Cleanup(func() {
		stop()
		if t.Failed() {
			data, _ := os.ReadFile(log.Name())
			t.Log(string(data))
		}
	})
	return stop
}

func TestStage7KernelGateway(t *testing.T) {
	client, _ := stage7World(t)
	shim := t.TempDir()
	fakeFirewallBinary(t, shim, "systemctl", "exit 0")
	t.Setenv("PATH", shim+":"+os.Getenv("PATH"))
	dir := t.TempDir()
	paths := &gatewayNetworkPaths{filepath.Join(dir, "state"), filepath.Join(dir, "rp"), filepath.Join(dir, "rules"), filepath.Join(dir, "routing.service"), filepath.Join(dir, "firewall.service"), filepath.Join(dir, "restore")}
	s := &GatewayNetworkService{paths: paths}
	ctx := context.Background()
	stage7Cmd(t, "nft", "add", "table", "inet", "stage7_foreign")
	stage7Cmd(t, "nft", "add", "chain", "inet", "stage7_foreign", "untouched")
	foreign := stage7Cmd(t, "nft", "-s", "list", "table", "inet", "stage7_foreign")
	t.Cleanup(func() { _ = exec.Command("nft", "delete", "table", "inet", "stage7_foreign").Run() })
	stage7Cmd(t, "ip", "rule", "add", "priority", "1234", "fwmark", "0x200/0x200", "table", "200")
	t.Cleanup(func() { _ = exec.Command("ip", "rule", "del", "priority", "1234").Run() })
	baselineRules := stage7Cmd(t, "ip", "-4", "rule", "show")
	baselineForward := stage7Cmd(t, "sysctl", "-n", "net.ipv4.ip_forward")
	stop := stage7KernelCore(t, map[string]any{"inbounds": []any{map[string]any{"type": "tproxy", "tag": "in-tproxy", "listen": "0.0.0.0", "listen_port": 52345}}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": map[string]any{"final": "direct"}})
	cfg := GatewayNetworkConfig{LANInterface: "gw0", LANIP: "10.60.0.1", LANPrefix: 24, WANInterface: "wan0"}
	if err := s.Enable(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Disable(ctx); err != nil {
			t.Error(err)
		}
	})
	for _, network := range []string{"tcp", "udp"} {
		stage7Packet(t, client, network, "198.18.0.2", 18080, true)
	}
	rules := stage7Cmd(t, "nft", "list", "table", "inet", "xui_gateway")
	if !strings.Contains(rules, "counter packets") || strings.Contains(rules, "counter packets 0 bytes 0 tproxy") {
		t.Fatal("TPROXY did not intercept real packets", rules)
	}
	if err := s.Suspend(ctx); err != nil {
		t.Fatal(err)
	}
	for _, network := range []string{"tcp", "udp"} {
		stage7Packet(t, client, network, "198.18.0.2", 18080, false)
	}
	stage7Packet(t, client, "tcp", "10.60.0.1", 2222, true)
	if err := s.Enable(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	// Simulate a fresh kernel while keeping the durable boot intent/files.
	if err := s.removeGatewayTables(ctx); err != nil {
		t.Fatal(err)
	}
	stage7Cmd(t, "ip", "-4", "rule", "del", "fwmark", "0x40/0xc0", "table", "100")
	stage7Cmd(t, "ip", "-4", "route", "del", "local", "default", "dev", "lo", "table", "100")
	stage7Cmd(t, "bash", paths.restoreScript, "routing")
	stage7Cmd(t, "bash", paths.restoreScript, "firewall")
	if status := s.Status(ctx); !status.PolicyRoute || !status.NFTables || !status.NAT {
		t.Fatalf("boot recovery incomplete: %+v", status)
	}
	for _, network := range []string{"tcp", "udp"} {
		stage7Packet(t, client, network, "198.18.0.2", 18080, true)
	}
	stop()
	if err := s.removeGatewayTables(ctx); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := exec.CommandContext(short, "bash", paths.restoreScript, "firewall").Run(); err == nil {
		t.Fatal("restored interception without a listening core")
	}
	for _, network := range []string{"tcp", "udp"} {
		stage7Packet(t, client, network, "198.18.0.2", 18080, false)
	}
	stage7Packet(t, client, "tcp", "10.60.0.1", 2222, true)
	if err := s.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	if got := stage7Cmd(t, "ip", "-4", "rule", "show"); got != baselineRules {
		t.Fatalf("foreign policy rules changed: %s / %s", baselineRules, got)
	}
	if got := stage7Cmd(t, "sysctl", "-n", "net.ipv4.ip_forward"); got != baselineForward {
		t.Fatal("forwarding not restored")
	}
	if got := stage7Cmd(t, "nft", "-s", "list", "table", "inet", "stage7_foreign"); got != foreign {
		t.Fatal("foreign nftables rules changed")
	}
	if tables := stage7Cmd(t, "nft", "list", "tables"); strings.Contains(tables, "xui_gateway") {
		t.Fatal("gateway left kernel tables behind", tables)
	}
	t.Log("real TCP/UDP TPROXY counters, fail-closed pause, local management, boot script replay and exact foreign-rule restoration passed")
}

func TestStage7KernelTun(t *testing.T) {
	stage7World(t)
	stage7Cmd(t, "ip", "route", "add", "default", "via", "198.18.0.2", "dev", "wan0")
	beforeRules := stage7Cmd(t, "ip", "-4", "rule", "show")
	for _, blocked := range []bool{false, true} {
		t.Run(fmt.Sprint("blocked=", blocked), func(t *testing.T) {
			route := map[string]any{"default_interface": "wan0", "final": "direct"}
			if blocked {
				route["rules"] = []any{map[string]any{"action": "reject"}}
			}
			stop := stage7KernelCore(t, map[string]any{"inbounds": []any{map[string]any{"type": "tun", "tag": "tun-test", "interface_name": "s7tun", "address": []string{"172.31.200.1/30"}, "auto_route": true, "strict_route": true, "stack": "system"}}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": route})
			for end := time.Now().Add(5 * time.Second); ; {
				if _, err := net.InterfaceByName("s7tun"); err == nil && strings.Contains(stage7Cmd(t, "ip", "-4", "rule", "show"), "lookup 2022") {
					break
				}
				if time.Now().After(end) {
					t.Fatal("native TUN interface never appeared")
				}
				time.Sleep(20 * time.Millisecond)
			}
			for _, network := range []string{"tcp", "udp"} {
				stage7Packet(t, "", network, "203.0.114.2", 18080, !blocked)
			}
			var packets uint64
			for _, name := range []string{"rx_packets", "tx_packets"} {
				b, err := os.ReadFile(filepath.Join("/sys/class/net/s7tun/statistics", name))
				if err != nil {
					t.Fatal(err)
				}
				v, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
				packets += v
			}
			if packets == 0 {
				t.Fatal("payload bypassed TUN")
			}
			stop()
			if _, err := net.InterfaceByName("s7tun"); err == nil {
				t.Fatal("TUN interface survived shutdown")
			}
			if after := stage7Cmd(t, "ip", "-4", "rule", "show"); after != beforeRules {
				t.Fatalf("TUN left policy rules: %s", after)
			}
		})
	}
}

func TestStage7KernelFirewall(t *testing.T) {
	client, _ := stage7World(t)
	ctx := context.Background()
	stage7Cmd(t, "nft", "add", "table", "inet", "stage7_foreign")
	t.Cleanup(func() { _ = exec.Command("nft", "delete", "table", "inet", "stage7_foreign").Run() })
	stage7Cmd(t, "nft", "add", "chain", "inet", "stage7_foreign", "input")
	stage7Cmd(t, "nft", "add", "rule", "inet", "stage7_foreign", "input", "ip", "saddr", "192.0.2.0/24", "counter", "drop")
	b := firewallBackend{name: "nftables", binary: "nft"}
	external, err := listExternalFirewallRules(ctx, b)
	if err != nil || len(external) != 1 {
		t.Fatal(external, err)
	}
	if err := deleteExternalFirewallRule(ctx, b, external[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := deleteExternalFirewallRule(ctx, b, external[0].ID); err == nil {
		t.Fatal("deleted stale selection")
	}
	foreign := stage7Cmd(t, "nft", "-s", "list", "table", "inet", "stage7_foreign")
	rules := []FirewallRule{{Port: 2222, Protocol: "tcp", Source: "session"}}
	if err := applyManagedNftables(ctx, "nft", rules); err != nil {
		t.Fatal(err)
	}
	stage7Packet(t, client, "tcp", "10.60.0.1", 2222, true)
	before := stage7Cmd(t, "nft", "-s", "list", "table", "inet", managedNftTable)
	dir := t.TempDir()
	realNft, err := exec.LookPath("nft")
	if err != nil {
		t.Fatal(err)
	}
	fakeFirewallBinary(t, dir, "nft", "if [ \"$1\" = -c ]; then cat >/dev/null; exit 1; fi\nexec "+realNft+" \"$@\"")
	if err := applyManagedNftables(ctx, filepath.Join(dir, "nft"), rules); err == nil {
		t.Fatal("injected nft rejection ignored")
	}
	if after := stage7Cmd(t, "nft", "-s", "list", "table", "inet", managedNftTable); after != before {
		t.Fatal("failed replacement altered live firewall")
	}
	stage7Packet(t, client, "tcp", "10.60.0.1", 2222, true)
	stage7Cmd(t, "nft", "delete", "table", "inet", managedNftTable)
	if after := stage7Cmd(t, "nft", "-s", "list", "table", "inet", "stage7_foreign"); after != foreign {
		t.Fatal("foreign nft table changed")
	}
	stage7Cmd(t, "iptables", "-N", "STAGE7_ADMIN")
	stage7Cmd(t, "iptables", "-A", "STAGE7_ADMIN", "-j", "RETURN")
	stage7Cmd(t, "iptables", "-A", "INPUT", "-j", "STAGE7_ADMIN")
	prior := stage7Cmd(t, "iptables", "-S")
	if err := applyManagedIPTablesSafe(ctx, "iptables", rules); err != nil {
		t.Fatal(err)
	}
	stage7Packet(t, client, "tcp", "10.60.0.1", 2222, true)
	if err := removeManagedIPTables(ctx, "iptables"); err != nil {
		t.Fatal(err)
	}
	if after := stage7Cmd(t, "iptables", "-S"); after != prior {
		t.Fatal("iptables removal changed administrator rules", after)
	}
	t.Log("nft transaction failure retained live rules; real namespace management traffic and iptables/nft foreign-rule preservation passed")
}
