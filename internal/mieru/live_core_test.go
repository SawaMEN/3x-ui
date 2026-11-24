package mieru

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	client "github.com/enfein/mieru/v3/apis/client"
	model "github.com/enfein/mieru/v3/apis/model"
	pb "github.com/enfein/mieru/v3/pkg/appctl/appctlpb"
	"golang.org/x/net/proxy"
	"google.golang.org/protobuf/proto"
)

func TestMieruAndNativeCoreConcurrentTraffic(t *testing.T) {
	coreBinary, mitaBinary := os.Getenv("SINGBOX_TEST_BINARY"), os.Getenv("MIERU_TEST_BINARY")
	if coreBinary == "" || mitaBinary == "" {
		t.Skip("requires native core and Mita release binaries")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	mita, err := os.ReadFile(mitaBinary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, GetBinaryName()), mita, 0o700); err != nil {
		t.Fatal(err)
	}
	reserve := func(network string) int {
		t.Helper()
		if network == "udp" {
			l, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			return l.LocalAddr().(*net.UDPAddr).Port
		}
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}
	id := 0
	for candidate := 1; candidate <= 1000; candidate++ {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", SOCKSPortForInbound(candidate)))
		if err == nil {
			l.Close()
			id = candidate
			break
		}
	}
	if id == 0 {
		t.Fatal("no available Mieru relay slot")
	}
	relay := SOCKSPortForInbound(id)
	ordinaryPort := reserve("tcp")
	for ordinaryPort == relay {
		ordinaryPort = reserve("tcp")
	}
	tcpPort := reserve("tcp")
	for tcpPort == relay || tcpPort == ordinaryPort {
		tcpPort = reserve("tcp")
	}
	udpPort := reserve("udp")
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			conn, err := echo.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg := map[string]any{
		"log": map[string]any{"level": "warn"},
		"inbounds": []any{
			map[string]any{"type": "socks", "tag": "mieru", "listen": "127.0.0.1", "listen_port": relay, "users": []any{map[string]any{"username": SocksUsername(), "password": SocksPassword()}}},
			map[string]any{"type": "socks", "tag": "ordinary", "listen": "127.0.0.1", "listen_port": ordinaryPort},
		},
		"outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}},
		"route":     map[string]any{"final": "direct", "rules": []any{map[string]any{"inbound": []string{"mieru"}, "action": "sniff", "sniffer": []string{"http", "tls", "quic"}}}},
	}
	save := func(name string, value any) string {
		t.Helper()
		path := filepath.Join(dir, name)
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	coreConfig := save("core.json", cfg)
	check := exec.CommandContext(ctx, coreBinary, "check", "-c", coreConfig)
	if out, err := check.CombinedOutput(); err != nil {
		t.Fatalf("native config check failed: %v: %s", err, out)
	}
	logFile, err := os.Create(filepath.Join(dir, "core.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	core := exec.CommandContext(ctx, coreBinary, "srun", "-c", coreConfig)
	core.Stdout, core.Stderr = logFile, logFile
	if err := core.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = core.Wait(); close(done) }()
	defer func() {
		_ = core.Process.Kill()
		<-done
		if t.Failed() {
			data, _ := os.ReadFile(logFile.Name())
			t.Logf("core log: %s", data)
		}
	}()
	ordinaryAddr := fmt.Sprintf("127.0.0.1:%d", ordinaryPort)
	for {
		conn, err := net.DialTimeout("tcp", ordinaryAddr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		select {
		case <-done:
			t.Fatal("native core exited during startup")
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	inst := Instance{Id: id, Tag: "mixed-traffic", Listen: "127.0.0.1", PortBindings: []PortBinding{{Port: tcpPort, Protocol: "TCP"}, {Port: udpPort, Protocol: "UDP"}}, Users: []User{{Name: "alice", Password: "secret"}}, MTU: 1400, LoggingLevel: "ERROR"}
	mitaConfig := renderConfig(inst)
	// Permit the test's loopback echo destination only in this fixture.
	mitaConfig["users"] = []any{map[string]any{"name": "alice", "password": "secret", "allowLoopbackIP": true, "allowPrivateIP": true}}
	p := newProcess(save("mita.json", mitaConfig), inst.Tag, filepath.Join(dir, "mita.sock"))
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	target := model.NetAddrSpec{Net: "tcp", AddrSpec: model.AddrSpec{IP: net.IPv4(127, 0, 0, 1), Port: echo.Addr().(*net.TCPAddr).Port}}
	tcpClient, udpClient := client.NewClient(), client.NewClient()
	profile := func(port int, protocol pb.TransportProtocol) *client.ClientConfig {
		return &client.ClientConfig{Profile: &pb.ClientProfile{ProfileName: proto.String("mixed"), User: &pb.User{Name: proto.String("alice"), Password: proto.String("secret")}, Servers: []*pb.ServerEndpoint{{IpAddress: proto.String("127.0.0.1"), PortBindings: []*pb.PortBinding{{Port: proto.Int32(int32(port)), Protocol: protocol.Enum()}}}}}}
	}
	if err := tcpClient.Store(profile(tcpPort, pb.TransportProtocol_TCP)); err != nil {
		t.Fatal(err)
	}
	if err := tcpClient.Start(); err != nil {
		t.Fatal(err)
	}
	defer tcpClient.Stop()
	if err := udpClient.Store(profile(udpPort, pb.TransportProtocol_UDP)); err != nil {
		t.Fatal(err)
	}
	if err := udpClient.Start(); err != nil {
		t.Fatal(err)
	}
	defer udpClient.Stop()
	socks, err := proxy.SOCKS5("tcp", ordinaryAddr, nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	ordinary := func() (net.Conn, error) {
		return socks.(proxy.ContextDialer).DialContext(ctx, "tcp", echo.Addr().String())
	}
	exchange := func(dial func() (net.Conn, error)) error {
		conn, err := dial()
		if err != nil {
			return err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
		payload := bytes.Repeat([]byte("mixed-payload"), 1024)
		if _, err := conn.Write(payload); err != nil {
			return err
		}
		got := make([]byte, len(payload))
		if _, err := io.ReadFull(conn, got); err != nil {
			return err
		}
		if !bytes.Equal(got, payload) {
			return fmt.Errorf("payload corrupted")
		}
		return nil
	}
	dials := []func() (net.Conn, error){
		func() (net.Conn, error) { return tcpClient.DialContext(ctx, target) },
		func() (net.Conn, error) { return udpClient.DialContext(ctx, target) },
		ordinary,
	}
	// Warm up both Mieru transports before concurrent payloads.
	for _, dial := range dials {
		if err := exchange(dial); err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan error, len(dials)*4)
	for _, dial := range dials {
		for worker := 0; worker < 4; worker++ {
			go func() { results <- exchange(dial) }()
		}
	}
	for i := 0; i < cap(results); i++ {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := exchange(ordinary); err != nil {
		t.Fatalf("ordinary connection failed after stopping Mieru: %v", err)
	}
	select {
	case <-done:
		t.Fatal("native core exited while Mieru was stopped")
	default:
	}
}
