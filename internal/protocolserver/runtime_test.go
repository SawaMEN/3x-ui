package protocolserver

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func testPorts(t *testing.T, n int) []int {
	t.Helper()
	var listeners []net.Listener
	ports := make([]int, n)
	for i := range ports {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listeners = append(listeners, listener)
		ports[i] = listener.Addr().(*net.TCPAddr).Port
	}
	for _, listener := range listeners {
		_ = listener.Close()
	}
	return ports
}

func TestPresenceExcludesIdleListeners(t *testing.T) {
	state.Lock()
	previous := state.processes
	state.processes = map[string]*process{}
	state.Unlock()
	t.Cleanup(func() {
		state.Lock()
		state.processes = previous
		state.Unlock()
	})

	for tag, payload := range map[string]string{
		"idle":   `{"alice":0}`,
		"active": `{"alice":2}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/online" {
				_, _ = io.WriteString(w, payload)
			} else {
				_, _ = io.WriteString(w, `{}`)
			}
		}))
		t.Cleanup(server.Close)
		state.Lock()
		state.processes[tag] = &process{
			plan: Plan{Protocol: model.Hysteria, Tag: tag, StatsListen: strings.TrimPrefix(server.URL, "http://")},
			done: make(chan struct{}), epoch: "test",
		}
		state.Unlock()
	}
	users, tags, err := Presence(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users["alice"] == nil {
		t.Fatalf("online users = %v, want alice", users)
	}
	if len(tags) != 1 || tags[0] != "active" {
		t.Fatalf("active inbound tags = %v, want [active]", tags)
	}
}

func testCertificate(t *testing.T) (string, string, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	private, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: private}), 0o600); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(encoded)
	return certPath, keyPath, roots
}

func echoSOCKS(t *testing.T, port int) <-chan string {
	t.Helper()
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	users := make(chan string, 8)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
				header := make([]byte, 2)
				if _, err := io.ReadFull(conn, header); err != nil {
					return
				}
				methods := make([]byte, int(header[1]))
				if _, err := io.ReadFull(conn, methods); err != nil {
					return
				}
				_, _ = conn.Write([]byte{5, 2})
				if _, err := io.ReadFull(conn, header); err != nil {
					return
				}
				name := make([]byte, int(header[1]))
				if _, err := io.ReadFull(conn, name); err != nil {
					return
				}
				length := make([]byte, 1)
				if _, err := io.ReadFull(conn, length); err != nil {
					return
				}
				password := make([]byte, int(length[0]))
				if _, err := io.ReadFull(conn, password); err != nil {
					return
				}
				_, _ = conn.Write([]byte{1, 0})
				users <- string(name)
				request := make([]byte, 4)
				if _, err := io.ReadFull(conn, request); err != nil {
					return
				}
				addressLength := 0
				switch request[3] {
				case 1:
					addressLength = 4
				case 4:
					addressLength = 16
				case 3:
					if _, err := io.ReadFull(conn, length); err != nil {
						return
					}
					addressLength = int(length[0])
				default:
					return
				}
				if _, err := io.CopyN(io.Discard, conn, int64(addressLength+2)); err != nil {
					return
				}
				_, _ = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 1})
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return users
}

func TestHysteriaAuthenticationRejectsUnknownTokens(t *testing.T) {
	handler := authHandler(Plan{AuthPath: "/secret", Users: map[string]string{"token": "alice"}})
	for _, tc := range []struct {
		token string
		ok    bool
	}{{"token", true}, {"invalid", false}, {"", false}} {
		request := httptest.NewRequest(http.MethodPost, "http://localhost/secret", strings.NewReader(fmt.Sprintf(`{"auth":%q}`, tc.token)))
		reply := httptest.NewRecorder()
		handler.ServeHTTP(reply, request)
		var result struct {
			OK bool   `json:"ok"`
			ID string `json:"id"`
		}
		if err := json.Unmarshal(reply.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if reply.Code != http.StatusOK || result.OK != tc.ok || (tc.ok && result.ID != "alice") {
			t.Fatalf("auth result: %d %+v", reply.Code, result)
		}
	}
}

func TestOfficialNaiveRoutesEachUserThroughAuthenticatedBridge(t *testing.T) {
	binaryPath := os.Getenv("CADDY_NAIVE_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("set CADDY_NAIVE_TEST_BINARY for official binary integration")
	}
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	t.Cleanup(func() { _ = StopAll() })
	cert, key, roots := testCertificate(t)
	ports := testPorts(t, 4)
	raw := map[string]any{"type": "naive", "tag": "naive-test", "network": "tcp", "listen": "127.0.0.1", "listen_port": ports[0], "tls": map[string]any{"certificate_path": cert, "key_path": key}, "users": []map[string]any{{"username": "alice", "password": "one"}, {"username": "bob", "password": "two"}}}
	plan, bridge, err := Build(raw, [3]int{ports[1], ports[2], ports[3]})
	if err != nil {
		t.Fatal(err)
	}
	plan.Binary = binaryPath
	if bridge["tag"] != "naive-test" || bridge["type"] != "socks" {
		t.Fatalf("invalid bridge: %#v", bridge)
	}
	users := echoSOCKS(t, ports[1])
	if err := Check(context.Background(), []Plan{plan}); err != nil {
		t.Fatal(err)
	}
	if err := Start(context.Background(), []Plan{plan}); err != nil {
		t.Fatal(err)
	}
	for name, password := range map[string]string{"alice": "one", "bob": "two"} {
		conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", ports[0]), &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS12})
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		auth := base64.StdEncoding.EncodeToString([]byte(name + ":" + password))
		_, _ = fmt.Fprintf(conn, "CONNECT example.test:443 HTTP/1.1\r\nHost: example.test:443\r\nProxy-Authorization: Basic %s\r\n\r\n", auth)
		reader := bufio.NewReader(conn)
		response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("CONNECT: %s", response.Status)
		}
		payload := "roundtrip-" + name
		_, _ = io.WriteString(conn, payload)
		data := make([]byte, len(payload))
		if _, err := io.ReadFull(reader, data); err != nil {
			t.Fatal(err)
		}
		if string(data) != payload {
			t.Fatalf("echo: %q", data)
		}
		_ = conn.Close()
		select {
		case got := <-users:
			if got != name {
				t.Fatalf("bridge authenticated %q, want %q", got, name)
			}
		case <-time.After(time.Second):
			t.Fatal("no upstream authentication")
		}
	}
}

func TestOfficialHysteriaAuthenticatesRoutesAndReportsClientTraffic(t *testing.T) {
	binaryPath := os.Getenv("HYSTERIA_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("set HYSTERIA_TEST_BINARY for official binary integration")
	}
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	t.Cleanup(func() { _ = StopAll() })
	cert, key, _ := testCertificate(t)
	ports := testPorts(t, 5)
	raw := map[string]any{"type": "hysteria2", "tag": "hysteria-test", "listen": "127.0.0.1", "listen_port": ports[0], "tls": map[string]any{"certificate_path": cert, "key_path": key}, "users": []map[string]any{{"name": "alice", "password": "secret-token"}}}
	plan, _, err := Build(raw, [3]int{ports[1], ports[2], ports[3]})
	if err != nil {
		t.Fatal(err)
	}
	plan.Binary = binaryPath
	users := echoSOCKS(t, ports[1])
	if err := Start(context.Background(), []Plan{plan}); err != nil {
		t.Fatal(err)
	}
	client := map[string]any{"server": fmt.Sprintf("127.0.0.1:%d", ports[0]), "auth": "secret-token", "tls": map[string]any{"sni": "localhost", "insecure": true}, "socks5": map[string]any{"listen": fmt.Sprintf("127.0.0.1:%d", ports[4])}}
	data, _ := json.Marshal(client)
	path := filepath.Join(t.TempDir(), "client.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "client", "--disable-update-check", "--config", path)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	var conn net.Conn
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		conn, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", ports[4]), 100*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = conn.Write([]byte{5, 1, 0})
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	address := "example.test"
	request := append([]byte{5, 1, 0, 3, byte(len(address))}, []byte(address)...)
	request = binary.BigEndian.AppendUint16(request, 443)
	_, _ = conn.Write(request)
	response := make([]byte, 10)
	if _, err := io.ReadFull(conn, response); err != nil {
		t.Fatal(err)
	}
	if response[1] != 0 {
		t.Fatalf("SOCKS response %v", response)
	}
	payload := "hysteria-roundtrip"
	_, _ = io.WriteString(conn, payload)
	echo := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, echo); err != nil {
		t.Fatal(err)
	}
	if string(echo) != payload {
		t.Fatalf("echo %q", echo)
	}
	select {
	case user := <-users:
		if !strings.HasPrefix(user, InternalUserPrefix) {
			t.Fatalf("bridge user %q", user)
		}
	case <-time.After(time.Second):
		t.Fatal("no Hysteria upstream")
	}
	snapshots, err := Snapshots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("snapshots: %+v", snapshots)
	}
	traffic := snapshots[0].Counters["alice"]
	if traffic.Tx < int64(len(payload)) || traffic.Rx < int64(len(payload)) || snapshots[0].Online["alice"] < 1 {
		t.Fatalf("traffic: %+v online: %v", traffic, snapshots[0].Online)
	}
	if !Running(model.Hysteria) {
		t.Fatal("Hysteria is not managed")
	}
}

func TestInternalPortsAvoidReservedListeners(t *testing.T) {
	occupied := map[int]bool{}
	first, err := Port("same-tag", occupied)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Port("same-tag", occupied)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !occupied[first] || !occupied[second] {
		t.Fatalf("colliding ports %s/%s", strconv.Itoa(first), strconv.Itoa(second))
	}
}

func TestOfficialNaiveQUICOnlyDoesNotOccupyTCP(t *testing.T) {
	binaryPath := os.Getenv("CADDY_NAIVE_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("set CADDY_NAIVE_TEST_BINARY for QUIC listener integration")
	}
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	t.Cleanup(func() { _ = StopAll() })
	cert, key, _ := testCertificate(t)
	ports := testPorts(t, 4)
	certPEM, err := os.ReadFile(cert)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, err := os.ReadFile(key)
	if err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{"type": "naive", "tag": "naive-quic", "network": "udp", "listen": "127.0.0.1", "listen_port": ports[0], "tls": map[string]any{"certificate": strings.Split(string(certPEM), "\n"), "key": strings.Split(string(keyPEM), "\n")}, "users": []map[string]any{{"username": "alice", "password": "secret"}}}
	plan, _, err := Build(raw, [3]int{ports[1], ports[2], ports[3]})
	if err != nil {
		t.Fatal(err)
	}
	plan.Binary = binaryPath
	if err := Check(context.Background(), []Plan{plan}); err != nil {
		t.Fatal(err)
	}
	if err := Start(context.Background(), []Plan{plan}); err != nil {
		log, _ := os.ReadFile(filepath.Join(os.Getenv("XUI_BIN_FOLDER"), "protocol-servers", identifier(plan.Tag), "server.log"))
		t.Fatalf("%v\n%s", err, log)
	}
	listener, err := net.Listen("tcp", plan.ReadyListen)
	if err != nil {
		t.Fatalf("QUIC-only Naive occupied TCP: %v", err)
	}
	_ = listener.Close()
	if !Running(model.NaiveProxy) {
		t.Fatal("QUIC-only Naive is not running")
	}
}

func TestInstalledSelectionMustBeAppliedEvenWithoutStandalonePlans(t *testing.T) {
	if !Supported(model.Hysteria, runtime.GOOS, runtime.GOARCH) {
		t.Skip("unsupported platform")
	}
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	t.Cleanup(func() { _ = StopAll() })
	if err := Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(BinaryPath(model.Hysteria), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !Changed() {
		t.Fatal("installation did not request apply")
	}
	if Applied(nil) {
		t.Fatal("new binary selection was treated as already applied")
	}
	if err := Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if Changed() {
		t.Fatal("selection keeps requesting redundant applies")
	}
}
