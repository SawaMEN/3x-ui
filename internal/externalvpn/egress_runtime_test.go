package externalvpn

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	stdbinary "encoding/binary"
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

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

func TestTrustTunnelPayloadUsesCoreSOCKS(t *testing.T) {
	for _, split := range []bool{false, true} {
		name := "combined"
		if split {
			name = "split"
		}
		t.Run(name, func(t *testing.T) { testTrustTunnelPayloadUsesCoreSOCKS(t, split) })
	}
}

func testTrustTunnelPayloadUsesCoreSOCKS(t *testing.T, split bool) {
	endpoint := os.Getenv("TRUSTTUNNEL_TEST_BINARY")
	if endpoint == "" {
		t.Skip("set TRUSTTUNNEL_TEST_BINARY for the official endpoint test")
	}
	if split {
		// Official upstream 1.1.0 (and 1.3.1) still binds TCP for pings
		// even in QUIC-only mode. Do not mistake a config table for an
		// independent UDP listener. A patched endpoint must pass the probe.
		if err := probeTrustTunnelQUICOnly(context.Background(), endpoint); err != nil {
			if os.Getenv("TRUSTTUNNEL_REQUIRE_SPLIT") == "1" {
				t.Fatalf("required independent QUIC endpoint is unavailable: %v", err)
			}
			t.Skipf("independent QUIC listener unavailable; production migration is blocked: %v", err)
		}
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
	seen := make(chan byte, 16)
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
	// Hold all TCP reservations simultaneously to avoid reusing an ephemeral
	// port between listener and metrics allocations.
	reserved := make([]net.Listener, 0, 4)
	for range 4 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		reserved = append(reserved, l)
		defer l.Close()
	}
	port := reserved[0].Addr().(*net.TCPAddr).Port
	metricsPort := reserved[1].Addr().(*net.TCPAddr).Port
	quicPort := reserved[2].Addr().(*net.TCPAddr).Port
	quicMetrics := reserved[3].Addr().String()
	// A TCP reservation cannot protect UDP; hold the UDP address too.
	quicReservation, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: quicPort})
	if err != nil {
		t.Fatal(err)
	}
	defer quicReservation.Close()
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
	if split {
		original := inst
		original.Port = quicPort
		plan, err := planTrustTunnelSplit(original, port, net.JoinHostPort("127.0.0.1", fmtPort(metricsPort)), quicMetrics)
		if err != nil {
			t.Fatal(err)
		}
		tcpPath := filepath.Join(filepath.Dir(plan.HostsPath), "vpn-tcp.toml")
		quicPath := filepath.Join(filepath.Dir(plan.HostsPath), "vpn-quic.toml")
		if err := writePrivate(tcpPath, []byte(plan.TCPConfig)); err != nil {
			t.Fatal(err)
		}
		if err := writePrivate(quicPath, []byte(plan.QUICConfig)); err != nil {
			t.Fatal(err)
		}
		for _, l := range reserved {
			l.Close()
		}
		quicReservation.Close()
		start(endpoint, []string{tcpPath, plan.HostsPath}, "endpoint.log")
		start(endpoint, []string{quicPath, plan.HostsPath}, "quic.log")
		deadline := time.Now().Add(8 * time.Second)
		for {
			_, err := readTrustTunnelMetrics(quicMetrics)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				log, _ := os.ReadFile(filepath.Join(dir, "quic.log"))
				t.Fatalf("QUIC-only endpoint metrics not ready: %s", log)
			}
			time.Sleep(100 * time.Millisecond)
		}
		// The QUIC-only process must bind UDP, never TCP, on its public port.
		if conn, err := net.DialTimeout("tcp", plan.QUICBind, time.Second); err == nil {
			conn.Close()
			t.Fatal("QUIC-only endpoint unexpectedly listens on TCP")
		}
		host, _, _ := net.SplitHostPort(plan.QUICBind)
		probe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(host), Port: quicPort})
		if err == nil {
			probe.Close()
			t.Fatal("QUIC-only endpoint did not reserve UDP")
		}
	} else {
		for _, l := range reserved {
			l.Close()
		}
		quicReservation.Close()
		start(endpoint, args, "endpoint.log")
	}
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
	transport := &http.Transport{ForceAttemptHTTP2: true, TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}, DialTLSContext: func(ctx context.Context, network, ignored string) (net.Conn, error) {
		raw, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		config := &tls.Config{NextProtos: []string{"h2"}}
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

	// Verify a real HTTP/3 connection using the official endpoint, its
	// generated certificate, proxy authentication and a SOCKS echo payload.
	// No external target is contacted: the fixture's SOCKS server echoes.
	h3Port := port
	if split {
		h3Port = quicPort
	}
	t.Run("authenticated-quic", func(t *testing.T) {
		before := len(seen)
		testTrustTunnelHTTP3Payload(t, inst, h3Port)
		if len(seen) != before+1 {
			t.Fatal("authenticated QUIC CONNECT did not reach SOCKS exactly once")
		}
	})

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

func testTrustTunnelHTTP3Payload(t *testing.T, inst Instance, port int) {
	t.Helper()
	// Trust only the fixture endpoint's actual certificate; never disable
	// certificate checks in the HTTP/3 interoperability probe.
	certPath := filepath.Join(directory(), strconv.Itoa(inst.ID), "cert.pem")
	pemBytes, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		t.Fatal("invalid TrustTunnel fixture certificate")
	}
	address := net.JoinHostPort("127.0.0.1", fmtPort(port))
	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: inst.Settings.Hostname},
		Dial: func(ctx context.Context, _ string, tlsConfig *tls.Config, config *quic.Config) (*quic.Conn, error) {
			return quic.DialAddr(ctx, address, tlsConfig, config)
		},
	}
	defer transport.Close()
	request := func(ctx context.Context, password string, body io.Reader) *http.Request {
		const authority = "8.8.8.8:53"
		req := (&http.Request{
			Method: http.MethodConnect,
			URL:    &url.URL{Scheme: "https", Host: authority},
			Host:   authority,
			Header: make(http.Header),
		}).WithContext(ctx)
		if body != nil {
			req.Body = io.NopCloser(body)
			req.ContentLength = -1
		}
		req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:"+password)))
		req.Header.Set("User-Agent", "linux panel-egress-test")
		return req
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bad, err := transport.RoundTrip(request(ctx, "incorrect", nil))
	if err != nil {
		t.Fatalf("QUIC authentication rejection probe: %v", err)
	}
	bad.Body.Close()
	if bad.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("invalid QUIC credentials returned HTTP %d, want 407", bad.StatusCode)
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	response, err := transport.RoundTrip(request(ctx, "secret", reader))
	if err != nil {
		t.Fatalf("authenticated QUIC CONNECT: %v", err)
	}
	defer response.Body.Close()
	if response.ProtoMajor != 3 || response.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP/3 CONNECT success, got %s %s", response.Proto, response.Status)
	}
	message := []byte("authenticated-quic-payload")
	written := make(chan error, 1)
	go func() {
		_, err := writer.Write(message)
		written <- err
	}()
	got := make([]byte, len(message))
	if _, err := io.ReadFull(response.Body, got); err != nil || string(got) != string(message) {
		t.Fatalf("QUIC echo: %q %v", got, err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
}
