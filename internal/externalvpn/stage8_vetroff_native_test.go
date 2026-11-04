package externalvpn

import (
	"bytes"
	"encoding/base64"
	stdbinary "encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"golang.org/x/net/proxy"
)

func stage8Port(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestStage8VetroffNativePayload(t *testing.T) {
	bundle := os.Getenv("STAGE8_VETROFF_BUNDLE")
	if bundle == "" {
		t.Skip("requires verified official Vetroff bundle")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	if err := os.Symlink(bundle, filepath.Join(dir, "vetroff-bundle")); err != nil {
		t.Fatal(err)
	}
	port := stage8Port(t)
	inst := Instance{ID: 82, Tag: "stage8-vetroff", Protocol: model.VetroffProxy, Listen: "127.0.0.1", Port: port, Settings: Settings{Hostname: "example.test", Clients: []Client{{Email: "alice", Password: "stage8-correct-secret-with-32-bytes", Enable: true}}}}
	certDir := filepath.Join(dir, "cert")
	_ = os.MkdirAll(certDir, 0700)
	cert, key, err := certificate(inst, certDir)
	if err != nil {
		t.Fatal(err)
	}
	inst.Settings.Certificate = cert
	inst.Settings.PrivateKey = key
	options := DefaultVetroffSettings()
	options.AllowPrivate = true
	inst.Settings.Vetroff = &options
	manager := &Manager{procs: map[int]*running{}, lastErrors: map[int]string{}}
	t.Cleanup(manager.StopAll)
	if err := manager.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(10 * time.Second); ; {
		c, e := net.DialTimeout("tcp", inst.Bind(), 100*time.Millisecond)
		if e == nil {
			c.Close()
			break
		}
		if time.Now().After(end) {
			t.Fatal("server not ready", e)
		}
		time.Sleep(30 * time.Millisecond)
	}
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { echo.Close() })
	go func() {
		for {
			c, e := echo.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { udp.Close() })
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
	oldPlans, oldProcesses := outboundPlans, outboundProcesses
	outboundPlans = map[string]*managedOutbound{}
	outboundProcesses = map[string]*managedOutbound{}
	t.Cleanup(func() { KeepOutbounds(nil); outboundPlans = oldPlans; outboundProcesses = oldProcesses })
	for _, transport := range []string{"h2-connect", "wss", "h3-connect"} {
		t.Run(transport, func(t *testing.T) {
			exportInst := inst
			copyOptions := options
			exportInst.Settings.Vetroff = &copyOptions
			copyOptions.Transport = transport
			copyOptions.TLSBackend = "auto"
			if transport == "h3-connect" {
				copyOptions.TLSBackend = "go"
			}
			link, err := exportVetroffLink(exportInst, "alice", "127.0.0.1", port)
			if err != nil {
				t.Fatal(err)
			}
			raw := map[string]any{"tag": "stage8-client", "protocol": "vetroff", "settings": map[string]any{"link": link, "enableUDP": true, "caCert": cert}}
			finish, err := ApplyOutbounds([]map[string]any{raw})
			if err != nil {
				t.Fatal(err)
			}
			if err = finish(true); err != nil {
				t.Fatal(err)
			}
			proc := outboundProcesses["stage8-client"]
			address := net.JoinHostPort("127.0.0.1", strconv.Itoa(proc.port))
			dialer, err := proxy.SOCKS5("tcp", address, nil, &net.Dialer{Timeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			c, err := dialer.Dial("tcp", echo.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			payload := bytes.Repeat([]byte("stage8-verified-payload"), 1024)
			c.SetDeadline(time.Now().Add(15 * time.Second))
			if _, err = c.Write(payload); err != nil {
				c.Close()
				t.Fatal(err)
			}
			got := make([]byte, len(payload))
			_, err = io.ReadFull(c, got)
			c.Close()
			if err != nil || !bytes.Equal(payload, got) {
				t.Fatal("TCP payload", err)
			}
			stage8SocksUDP(t, address, udp.LocalAddr().(*net.UDPAddr).Port)
			// Reconciliation with identical settings must preserve the native server.
			previous := manager.procs[inst.ID]
			if err := manager.Ensure(inst); err != nil {
				t.Fatal(err)
			}
			if manager.procs[inst.ID] != previous {
				t.Fatal("unchanged server was restarted")
			}
		})
	}
	for _, field := range []string{"sni", "pin_sha256", "password"} {
		t.Run("reject-"+field, func(t *testing.T) {
			link, err := exportVetroffLink(inst, "alice", "127.0.0.1", port)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(link, "vetroff://"))
			if err != nil {
				t.Fatal(err)
			}
			var profile map[string]any
			if err = json.Unmarshal(decoded, &profile); err != nil {
				t.Fatal(err)
			}
			profile["tls"] = "go"
			profile["transport"] = "h2-connect"
			switch field {
			case "sni":
				profile[field] = "wrong.example.test"
			case "pin_sha256":
				profile[field] = "sha256/" + base64.StdEncoding.EncodeToString(make([]byte, 32))
			default:
				profile[field] = "incorrect-authentication-password"
			}
			data, _ := json.Marshal(profile)
			badLink := "vetroff://" + base64.RawURLEncoding.EncodeToString(data)
			raw := map[string]any{"tag": "stage8-client", "protocol": "vetroff", "settings": map[string]any{"link": badLink, "caCert": cert}}
			finish, err := ApplyOutbounds([]map[string]any{raw})
			if err != nil {
				t.Fatal(err)
			}
			if err = finish(true); err != nil {
				t.Fatal(err)
			}
			proc := outboundProcesses["stage8-client"]
			d, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", fmtPort(proc.port)), nil, &net.Dialer{Timeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			c, err := d.Dial("tcp", echo.Addr().String())
			if err == nil {
				defer c.Close()
				c.SetDeadline(time.Now().Add(time.Second))
				c.Write([]byte("forbidden"))
				var b [9]byte
				n, _ := io.ReadFull(c, b[:])
				if n > 0 {
					t.Fatal("invalid " + field + " forwarded payload")
				}
			}
		})
	}

}

func stage8SocksUDP(t *testing.T, address string, target int) {
	stage8SocksUDPTo(t, address, "127.0.0.1", target)
}

func stage8SocksUDPTo(t *testing.T, address, host string, target int) {
	t.Helper()
	c, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(15 * time.Second))
	c.Write([]byte{5, 1, 0})
	var hello [2]byte
	if _, err = io.ReadFull(c, hello[:]); err != nil || hello != [2]byte{5, 0} {
		t.Fatal("SOCKS greeting", hello, err)
	}
	c.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0})
	var reply [10]byte
	if _, err = io.ReadFull(c, reply[:]); err != nil || reply[1] != 0 || reply[3] != 1 {
		t.Fatal("UDP associate", reply, err)
	}
	relay := net.JoinHostPort(net.IP(reply[4:8]).String(), fmt.Sprint(stdbinary.BigEndian.Uint16(reply[8:])))
	u, err := net.Dial("udp", relay)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	u.SetDeadline(time.Now().Add(15 * time.Second))
	payload := []byte("stage8-native-udp")
	ip := net.ParseIP(host).To4()
	if ip == nil {
		t.Fatal("IPv4 UDP target required")
	}
	packet := append([]byte{0, 0, 0, 1}, ip...)
	packet = append(packet, byte(target>>8), byte(target))
	packet = append(packet, payload...)
	if _, err = u.Write(packet); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 2048)
	n, err := u.Read(b)
	if err != nil || n < 10+len(payload) || !bytes.Equal(b[n-len(payload):n], payload) {
		t.Fatal("UDP payload", n, err)
	}
}

func stage8StartCore(t *testing.T, binary, path string, port int) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(binary, "srun", "-c", path)
	cmd.Dir = filepath.Dir(binary)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("core did not exit")
		}
	})
	for end := time.Now().Add(10 * time.Second); ; {
		c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", fmtPort(port)), 100*time.Millisecond)
		if e == nil {
			c.Close()
			return cmd
		}
		if time.Now().After(end) {
			t.Fatal("core listener not ready", e)
		}
		time.Sleep(30 * time.Millisecond)
	}
}
