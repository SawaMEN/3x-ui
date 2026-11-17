package hiddify

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Keep a successful control transfer and the exact upstream error alongside
// the expected upstream defect. A dead listener is never evidence of this bug.
func TestStage5FragmentedTrojanContract(t *testing.T) {
	root := os.Getenv("XUI_STAGE4_CORE_DIR")
	if root == "" {
		t.Skip("requires pinned owner and official upstream binaries")
	}
	for _, profile := range []string{"owner", "upstream"} {
		t.Run(profile, func(t *testing.T) {
			t.Setenv("STAGE5_NATIVE_PROFILE", profile)
			coreBinary := filepath.Join(root, "owner", "x-ui", "bin", "hiddify-core-linux-"+runtime.GOARCH)
			if profile == "upstream" {
				coreBinary = filepath.Join(root, "upstream", "sing-box")
			}
			dir := t.TempDir()
			cert, key := testCertificate(t, dir)
			port := stage5Port(t)
			stage5Start(t, coreBinary, dir, "server", map[string]any{"inbounds": []any{map[string]any{"type": "trojan", "tag": "fragment-contract", "listen": "127.0.0.1", "listen_port": port, "users": []any{map[string]any{"name": "alice", "password": "secret"}}, "tls": map[string]any{"enabled": true, "certificate_path": cert, "key_path": key}}}, "outbounds": []any{map[string]any{"type": "direct"}}}, port, true)
			target := stage5Echo(t, "127.0.0.1:0")
			host, p, _ := net.SplitHostPort(target)
			var targetPort uint16
			if _, err := fmt.Sscan(p, &targetPort); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum224([]byte("secret"))
			var frame bytes.Buffer
			frame.WriteString(hex.EncodeToString(hash[:]))
			frame.WriteString("\r\n")
			frame.Write([]byte{1, 1})
			frame.Write(net.ParseIP(host).To4())
			_ = binary.Write(&frame, binary.BigEndian, targetPort)
			frame.WriteString("\r\nfragment-proof")
			pem, err := os.ReadFile(cert)
			if err != nil {
				t.Fatal(err)
			}
			roots := x509.NewCertPool()
			if !roots.AppendCertsFromPEM(pem) {
				t.Fatal("invalid fixture certificate")
			}
			transfer := func(fragment bool) error {
				c, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS12})
				if err != nil {
					return err
				}
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(3 * time.Second))
				if fragment {
					if _, err = c.Write(frame.Bytes()[:1]); err != nil {
						return err
					}
					time.Sleep(20 * time.Millisecond)
					_, err = c.Write(frame.Bytes()[1:])
				} else {
					_, err = c.Write(frame.Bytes())
				}
				if err != nil {
					return err
				}
				got := make([]byte, len("fragment-proof"))
				_, err = io.ReadFull(c, got)
				if err != nil {
					return err
				}
				if string(got) != "fragment-proof" {
					return fmt.Errorf("wrong payload: %q", got)
				}
				return nil
			}
			if err := transfer(false); err != nil {
				t.Fatalf("control failed: %v", err)
			}
			err = transfer(true)
			if profile == "owner" {
				if err != nil {
					t.Fatalf("owner fragmentation regression: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("upstream behavior changed: remove the known-defect classification and update its pinned version")
			}
			log, readErr := os.ReadFile(filepath.Join(dir, "server.log"))
			if readErr != nil || !strings.Contains(string(log), "bad request size") {
				t.Fatalf("failure is not the known upstream defect: %v %s", readErr, log)
			}
			t.Log("KNOWN_UPSTREAM_DEFECT: official sing-box 1.14.4 rejects a fragmented Trojan authentication key; control passed")
		})
	}
}
