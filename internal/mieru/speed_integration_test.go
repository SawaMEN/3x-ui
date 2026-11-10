package mieru

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	client "github.com/enfein/mieru/v3/apis/client"
	model "github.com/enfein/mieru/v3/apis/model"
	pb "github.com/enfein/mieru/v3/pkg/appctl/appctlpb"
	"google.golang.org/protobuf/proto"

	"github.com/SawaMEN/3x-ui/v3/internal/clientlimit"
)

func TestMieruSpeedLimitLiveServer(t *testing.T) {
	binary := os.Getenv("MIERU_TEST_BINARY")
	if binary == "" {
		t.Skip("set MIERU_TEST_BINARY to the speed-enabled mita server")
	}
	dir := t.TempDir()
	probe, err := net.Listen("unix", filepath.Join(dir, "probe.sock"))
	if errors.Is(err, syscall.EPERM) {
		t.Skip("native mita CLI requires Unix-domain sockets")
	}
	if err != nil {
		t.Fatal(err)
	}
	_ = probe.Close()
	t.Setenv("XUI_BIN_FOLDER", dir)
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, GetBinaryName()), data, 0o700); err != nil {
		t.Fatal(err)
	}
	previous := clientlimit.Default
	clientlimit.Default = clientlimit.New("127.0.0.1:0", "127.0.0.1:0")
	defer func() { clientlimit.Default.Close(); clientlimit.Default = previous }()
	finish, err := clientlimit.Default.Apply(clientlimit.Plan{Routes: []clientlimit.Route{{ID: "mieru-speed", Email: "alice", UploadMbps: 1, DownloadMbps: 1, Aliases: []string{"mieru:alice"}}}})
	if err != nil {
		t.Fatal(err)
	}
	finish(true)
	defer clientlimit.Default.Close()
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := free.Addr().(*net.TCPAddr).Port
	_ = free.Close()
	cfg := map[string]any{"portBindings": []any{map[string]any{"port": port, "protocol": "TCP"}}, "users": []any{map[string]any{"name": "alice", "password": "secret", "allowLoopbackIP": true, "allowPrivateIP": true}}, "loggingLevel": "ERROR", "mtu": 1400}
	data, _ = json.Marshal(cfg)
	path := filepath.Join(dir, "server.json")
	if err = os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	server := newProcess(path, "speed-test", filepath.Join(dir, "mita.sock"))
	if err = server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	c := client.NewClient()
	if err = c.Store(&client.ClientConfig{Profile: &pb.ClientProfile{ProfileName: proto.String("speed"), User: &pb.User{Name: proto.String("alice"), Password: proto.String("secret")}, Servers: []*pb.ServerEndpoint{{IpAddress: proto.String("127.0.0.1"), PortBindings: []*pb.PortBinding{{Port: proto.Int32(int32(port)), Protocol: pb.TransportProtocol_TCP.Enum()}}}}}}); err != nil {
		t.Fatal(err)
	}
	if err = c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	target := model.NetAddrSpec{Net: "tcp", AddrSpec: model.AddrSpec{IP: net.IPv4(127, 0, 0, 1), Port: echo.Addr().(*net.TCPAddr).Port}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var conn net.Conn
	for {
		conn, err = c.DialContext(ctx, target)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	payload := bytes.Repeat([]byte("rate"), 65536)
	started := time.Now()
	sent := make(chan error, 1)
	go func() { _, err := conn.Write(payload); sent <- err }()
	got := make([]byte, len(payload))
	if _, err = io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if err = <-sent; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("native shaping corrupted payload")
	}
	if elapsed := time.Since(started); elapsed < time.Second || elapsed > 8*time.Second {
		t.Fatalf("native Mieru ignored 1 Mbps cap: %v", elapsed)
	}
}
