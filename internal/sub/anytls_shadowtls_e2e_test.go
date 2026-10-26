package sub

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
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
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func anyTLSCertificate(t *testing.T, names ...string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: secret}), 0600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func anyTLSFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func runAnyTLSCore(t *testing.T, binary string, cfg map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	check := exec.Command(binary, "check", "-c", path)
	check.Dir = filepath.Dir(binary)
	if out, err := check.CombinedOutput(); err != nil {
		t.Fatalf("core check: %v %s", err, out)
	}
	logPath := filepath.Join(dir, "core.log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "srun", "-c", path)
	command.Dir = filepath.Dir(binary)
	command.Stdout, command.Stderr = log, log
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait(); _ = log.Close() })
	return logPath
}

// The exact subscription document -> release core -> ShadowTLS -> inner AnyTLS
// -> TCP/UDP echo. All services are local; no root, TUN or public TLS site needed.
func TestAnyTLSShadowTLSSubscriptionConnections(t *testing.T) {
	binary := os.Getenv("HIDDIFY_TEST_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_TEST_BINARY for real AnyTLS + ShadowTLS traffic checks")
	}
	initSubDB(t)
	certPath, keyPath := anyTLSCertificate(t, "inner.example")
	cover := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	cover.StartTLS()
	defer cover.Close()
	coverPort := cover.Listener.Addr().(*net.TCPAddr).Port
	serverPort, relayPort := anyTLSFreePort(t), anyTLSFreePort(t)
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	echoPort := echo.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			c, e := echo.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	udp, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", echoPort))
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, e := udp.ReadFrom(buf)
			if e != nil {
				return
			}
			_, _ = udp.WriteTo(buf[:n], addr)
		}
	}()
	settings := map[string]any{"tls": map[string]any{"certificatePath": certPath, "keyPath": keyPath}, "clients": []any{map[string]any{"email": "alice", "password": "inner-password", "enable": true}}, "shadowTls": map[string]any{"enabled": true, "password": "outer-password", "handshake": map[string]any{"server": "127.0.0.1", "serverPort": coverPort}}}
	raw := map[string]any{"protocol": "anytls", "tag": "anytls-test", "listen": "127.0.0.1", "port": serverPort, "settings": settings}
	outer, inner, err := singbox.TranslateShadowTLSWrappedInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	serverCfg := map[string]any{"log": map[string]any{"level": "debug"}, "dns": map[string]any{"servers": []any{map[string]any{"type": "local", "tag": "local"}}}, "inbounds": []any{outer, inner}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": map[string]any{"final": "direct"}}
	serverLog := runAnyTLSCore(t, binary, serverCfg)
	settingsRaw, _ := json.Marshal(settings)
	streamRaw, _ := json.Marshal(map[string]any{"externalProxy": []any{map[string]any{"dest": "127.0.0.1", "port": serverPort, "allowInsecure": true}}}) // Only the local camouflage TLS certificate is untrusted.
	ib := &model.Inbound{UserId: 1, Enable: true, Protocol: model.AnyTLS, Tag: "anytls-test", Port: serverPort, Settings: string(settingsRaw), StreamSettings: string(streamRaw)}
	db := database.GetDB()
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "alice", Password: "inner-password", Enable: true, SubID: "anytls-e2e"}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	sub, _, err := NewSubJsonService("", "", "", "", NewSubService("")).GetHiddifySingBoxJson(client.SubID, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(sub), &cfg); err != nil {
		t.Fatal(err)
	}
	visible := ""
	for _, item := range cfg["outbounds"].([]any) {
		ob := item.(map[string]any)
		if ob["type"] == "anytls" {
			visible = ob["tag"].(string)
		}
	}
	if visible == "" {
		t.Fatalf("subscription missing AnyTLS: %s", sub)
	}
	cfg["inbounds"] = []any{map[string]any{"type": "direct", "tag": "relay", "listen": "127.0.0.1", "listen_port": relayPort, "override_address": "127.0.0.1", "override_port": echoPort}}
	cfg["route"] = map[string]any{"final": visible}
	cfg["log"] = map[string]any{"level": "debug"}
	clientLog := runAnyTLSCore(t, binary, cfg)
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		data, _ := os.ReadFile(serverLog)
		if strings.Contains(string(data), "create netlink socket: operation not permitted") {
			if os.Getenv("CI") == "true" {
				t.Fatalf("CI must exercise real AnyTLS + ShadowTLS TCP/UDP traffic; core startup failed: %s", data)
			}
			t.Skip("core configurations accepted; runtime netlink sockets are unavailable on this host")
		}
		if strings.Contains(string(data), "sing-box started") {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	for _, network := range []string{"tcp", "udp"} {
		t.Run(network, func(t *testing.T) {
			deadline := time.Now().Add(8 * time.Second)
			var last error
			for time.Now().Before(deadline) {
				c, err := net.DialTimeout(network, fmt.Sprintf("127.0.0.1:%d", relayPort), time.Second)
				if err == nil {
					_ = c.SetDeadline(time.Now().Add(time.Second))
					payload := "anytls-shadowtls-echo"
					_, err = io.WriteString(c, payload)
					if err == nil {
						buf := make([]byte, len(payload))
						_, err = io.ReadFull(c, buf)
						if err == nil && string(buf) != payload {
							err = fmt.Errorf("wrong echo")
						}
					}
					_ = c.Close()
				}
				if err == nil {
					t.Logf("%s traffic passed through the generated AnyTLS + ShadowTLS subscription", network)
					return
				}
				last = err
				time.Sleep(100 * time.Millisecond)
			}
			server, _ := os.ReadFile(serverLog)
			client, _ := os.ReadFile(clientLog)
			t.Fatalf("%s roundtrip failed: %v\nserver: %s\nclient: %s", network, last, server, client)
		})
	}
	if strings.Contains(sub, "PRIVATE KEY") {
		t.Fatal("subscription leaked private certificate key")
	}
}
