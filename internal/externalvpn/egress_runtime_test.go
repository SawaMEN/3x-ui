package externalvpn

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	stdbinary "encoding/binary"
	"golang.org/x/net/http2"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

func TestTrustTunnelPayloadUsesCoreSOCKS(t *testing.T) {
	endpoint := os.Getenv("TRUSTTUNNEL_TEST_BINARY")
	if endpoint == "" {
		t.Skip("set TRUSTTUNNEL_TEST_BINARY for the official endpoint test")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	socks, err := net.Listen("tcp", sidecaregress.Address(998))
	if err != nil {
		t.Fatal(err)
	}
	defer socks.Close()
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	seen := make(chan byte, 4)
	go func() {
		b := make([]byte, 65535)
		for {
			n, addr, e := udp.ReadFromUDP(b)
			if e != nil {
				return
			}
			udp.WriteToUDP(b[:n], addr)
		}
	}()
	go func() {
		for {
			c, e := socks.Accept()
			if e != nil {
				return
			}
			go func() {
				defer c.Close()
				c.SetDeadline(time.Now().Add(15 * time.Second))
				var hello [2]byte
				if _, e := io.ReadFull(c, hello[:]); e != nil {
					return
				}
				io.CopyN(io.Discard, c, int64(hello[1]))
				c.Write([]byte{5, 0})
				var header [4]byte
				if _, e := io.ReadFull(c, header[:]); e != nil {
					return
				}
				var n int
				switch header[3] {
				case 1:
					n = 6
				case 4:
					n = 18
				case 3:
					var length [1]byte
					io.ReadFull(c, length[:])
					n = int(length[0]) + 2
				default:
					return
				}
				if _, e := io.CopyN(io.Discard, c, int64(n)); e != nil {
					return
				}
				seen <- header[1]
				reply := []byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}
				stdbinary.BigEndian.PutUint16(reply[8:], uint16(udp.LocalAddr().(*net.UDPAddr).Port))
				c.Write(reply)
				if header[1] == 1 {
					io.Copy(c, c)
				} else {
					io.Copy(io.Discard, c)
				}
			}()
		}
	}()
	freePort := func() int {
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}
	port, metricsPort := freePort(), freePort()
	inst := Instance{ID: 998, Protocol: model.TrustTunnel, Listen: "127.0.0.1", Port: port, Settings: Settings{Hostname: "example.test", Clients: []Client{{Email: "alice", Password: "secret", Enable: true}}}}
	args, err := files(inst, net.JoinHostPort("127.0.0.1", fmtPort(metricsPort)))
	if err != nil {
		t.Fatal(err)
	}
	start := func(binary string, args []string, name string) *os.File {
		log, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, args...)
		cmd.Dir = filepath.Dir(binary)
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait(); log.Close() })
		return log
	}
	start(endpoint, args, "endpoint.log")
	address := net.JoinHostPort("127.0.0.1", fmtPort(port))
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(filepath.Join(dir, "endpoint.log"))
			t.Fatalf("endpoint did not listen: %v %s", err, log)
		}
		time.Sleep(50 * time.Millisecond)
	}
	transport := &http2.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialTLSContext: func(ctx context.Context, network, ignored string, cfg *tls.Config) (net.Conn, error) {
		raw, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		config := cfg.Clone()
		config.ServerName = "example.test"
		config.InsecureSkipVerify = true
		conn := tls.Client(raw, config)
		if err := conn.HandshakeContext(ctx); err != nil {
			raw.Close()
			return nil, err
		}
		return conn, nil
	}}
	defer transport.CloseIdleConnections()
	for _, network := range []string{"tcp", "udp"} {
		t.Run(network, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			authority := "8.8.8.8:53"
			if network == "udp" {
				authority = "_udp2"
			}
			request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Scheme: "https", Host: authority}, Host: authority, Header: make(http.Header), Body: reader}
			request = request.WithContext(ctx)
			request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:secret")))
			request.Header.Set("User-Agent", "linux panel-egress-test")
			response, err := transport.RoundTrip(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("CONNECT response: %s", response.Status)
			}
			message := []byte("routed-payload")
			if network == "tcp" {
				writer.Write(message)
				got := make([]byte, len(message))
				if _, err := io.ReadFull(response.Body, got); err != nil || string(got) != string(message) {
					t.Fatalf("TCP payload: %q %v", got, err)
				}
			} else {
				packet := make([]byte, 41+len(message))
				stdbinary.BigEndian.PutUint32(packet, uint32(len(packet)-4))
				copy(packet[16:20], []byte{10, 0, 0, 2})
				stdbinary.BigEndian.PutUint16(packet[20:22], 12345)
				copy(packet[34:38], []byte{8, 8, 8, 8})
				stdbinary.BigEndian.PutUint16(packet[38:40], 53)
				copy(packet[41:], message)
				if _, err := writer.Write(packet); err != nil {
					t.Fatal(err)
				}
				var length [4]byte
				if _, err := io.ReadFull(response.Body, length[:]); err != nil {
					t.Fatal(err)
				}
				n := int(stdbinary.BigEndian.Uint32(length[:]))
				if n < 36 || n > 65535 {
					t.Fatalf("invalid UDP reply size: %d", n)
				}
				got := make([]byte, n)
				if _, err := io.ReadFull(response.Body, got); err != nil || string(got[36:]) != string(message) {
					t.Fatalf("UDP payload: %q %v", got, err)
				}
			}
		})
	}

	// Both payload types must have reached the configured SOCKS router.
	var tcp, datagrams bool
	for len(seen) > 0 {
		switch <-seen {
		case 1:
			tcp = true
		case 3:
			datagrams = true
		}
	}
	if !tcp || !datagrams {
		t.Fatalf("missing SOCKS payload path: TCP=%v UDP=%v", tcp, datagrams)
	}
}

func fmtPort(port int) string { return strconv.Itoa(port) }
