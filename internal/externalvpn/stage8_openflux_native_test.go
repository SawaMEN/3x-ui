package externalvpn

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"golang.org/x/net/proxy"
)

func TestStage8OpenFluxNativePayload(t *testing.T) {
	executable := os.Getenv("STAGE8_OPENFLUX_BINARY")
	if executable == "" {
		t.Skip("requires pinned OpenFlux binary")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	if err := os.Symlink(executable, filepath.Join(dir, "openflux")); err != nil {
		t.Fatal(err)
	}
	port := stage8Port(t)
	inst := Instance{ID: 84, Tag: "openflux-native", Protocol: model.OpenFlux, Listen: "127.0.0.1", Port: port, Settings: Settings{Context: "stage8", Transports: []OpenFluxTransport{{Type: "direct", Priority: 100}}, Clients: []Client{{Email: "alice", Password: "stage8-real-openflux-secret-1234567890", Enable: true}}}}
	m := &Manager{procs: map[int]*running{}, lastErrors: map[int]string{}}
	t.Cleanup(m.StopAll)
	if err := m.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	previousPlans, previousProcesses := outboundPlans, outboundProcesses
	outboundPlans = map[string]*managedOutbound{}
	outboundProcesses = map[string]*managedOutbound{}
	t.Cleanup(func() { KeepOutbounds(nil); outboundPlans = previousPlans; outboundProcesses = previousProcesses })
	raw := map[string]any{"tag": "openflux-native", "protocol": "openflux", "settings": map[string]any{"secret": inst.Settings.Clients[0].Password, "context": inst.Settings.Context, "transports": []any{map[string]any{"type": "direct", "dial": inst.Bind(), "priority": 100}}}}
	finish, err := ApplyOutbounds([]map[string]any{raw})
	if err != nil {
		t.Fatal(err)
	}
	if err = finish(true); err != nil {
		t.Fatal(err)
	}
	proc := outboundProcesses["openflux-native"]
	address := net.JoinHostPort("127.0.0.1", fmtPort(proc.port))
	for end := time.Now().Add(10 * time.Second); ; {
		c, e := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if e == nil {
			c.Close()
			break
		}
		if time.Now().After(end) {
			t.Fatal("OpenFlux SOCKS not ready", e)
		}
		time.Sleep(30 * time.Millisecond)
	}
	echo, err := net.Listen("tcp", "127.0.0.1:0")
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
	d, err := proxy.SOCKS5("tcp", address, nil, &net.Dialer{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	c, err := d.Dial("tcp", echo.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(15 * time.Second))
	data := bytes.Repeat([]byte("encrypted-openflux-payload"), 1024)
	if _, err = c.Write(data); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(data))
	_, err = io.ReadFull(c, got)
	if err != nil || !bytes.Equal(data, got) {
		t.Fatal("OpenFlux TCP payload", err)
	}
}
