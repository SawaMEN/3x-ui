package externalvpn

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestVetroffConfigurationAndLink(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	inst := Instance{ID: 320, Protocol: model.VetroffProxy, Port: 8443, Settings: Settings{Hostname: "vpn.example.com", Clients: []Client{{Email: "alice", Password: strings.Repeat("s", 32), Enable: true}, {Email: "bob", Password: strings.Repeat("o", 32), Enable: false}}}}
	folder := filepath.Join(directory(), "cert-test")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	cert, key, err := certificate(inst, folder)
	if err != nil {
		t.Fatal(err)
	}
	inst.Settings.Certificate, inst.Settings.PrivateKey = cert, key
	args, err := vetroffFiles(inst)
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != "server" || args[1] != "--config" {
		t.Fatalf("args: %v", args)
	}
	raw, err := os.ReadFile(args[2])
	if err != nil {
		t.Fatal(err)
	}
	var conf map[string]any
	if err := json.Unmarshal(raw, &conf); err != nil {
		t.Fatal(err)
	}
	if conf["listen"] != "0.0.0.0:8443" || conf["h3_listen"] != conf["listen"] {
		t.Fatalf("listeners: %s", raw)
	}
	usersPath := conf["users_file"].(string)
	users, err := os.ReadFile(usersPath)
	if err != nil {
		t.Fatal(err)
	}
	var policy struct {
		Users []struct {
			Username string
			Disabled bool
		}
	}
	if err := json.Unmarshal(users, &policy); err != nil {
		t.Fatal(err)
	}
	if len(policy.Users) != 2 || !policy.Users[1].Disabled {
		t.Fatalf("policy: %s", users)
	}
	info, err := os.Stat(usersPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private policy mode: %v, %v", info, err)
	}
	link, err := exportVetroffLink(inst, "alice", "2001:db8::1", 8443)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(link, "vetroff://"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Servers                 []string
		Username, Password, SNI string
	}
	if err := json.Unmarshal(profile, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Servers[0] != "https://[2001:db8::1]:8443" || parsed.Username != FPTNUsername("alice") || parsed.Password != strings.Repeat("s", 32) || parsed.SNI != "vpn.example.com" {
		t.Fatalf("unexpected profile: %s", profile)
	}
	if _, err := exportVetroffLink(inst, "bob", "vpn.example.com", 8443); err == nil {
		t.Fatal("disabled user exported")
	}
	if binary := os.Getenv("VETROFF_TEST_BINARY"); binary != "" {
		cmd := exec.Command(binary, "check-config", "--config", args[2])
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("official binary rejected server config: %v: %s", err, out)
		}

		releaseRoot := filepath.Dir(filepath.Dir(binary))
		if err := os.Symlink(releaseRoot, filepath.Join(os.Getenv("XUI_BIN_FOLDER"), "vetroff-bundle")); err != nil {
			t.Fatal(err)
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		inst.Listen, inst.Port = "127.0.0.1", listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		manager := &Manager{procs: map[int]*running{}, lastErrors: map[int]string{}}
		t.Cleanup(func() { manager.Remove(inst.ID) })
		if err := manager.Ensure(inst); err != nil {
			t.Fatal(err)
		}
		pem, err := os.ReadFile(cert)
		if err != nil {
			t.Fatal(err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			t.Fatal("test certificate rejected")
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 250 * time.Millisecond}, "tcp", inst.Bind(), &tls.Config{RootCAs: roots, ServerName: inst.Settings.Hostname, MinVersion: tls.VersionTLS13})
			if err == nil {
				conn.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("official server failed TLS startup: %v", err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		inst.Settings.Clients[0].Enable = false
		if err := manager.Ensure(inst); err != nil {
			t.Fatal(err)
		}
		if len(manager.procs) != 0 {
			t.Fatal("server kept running after disabling all clients")
		}
	}
}

func TestVetroffBundleExtraction(t *testing.T) {
	for _, attack := range []string{"", "foxtunnel-necko/../../escape", "/tmp/escape", "foxtunnel-necko/link"} {
		t.Run(attack, func(t *testing.T) {
			folder := t.TempDir()
			archive := filepath.Join(folder, "release.tar.gz")
			f, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(f)
			tw := tar.NewWriter(gz)
			files := []string{"foxtunnel-necko/vetroff", "foxtunnel-necko/foxtunnel-necko", "foxtunnel-necko/runtime/xpcshell", "foxtunnel-necko/lib/libnss3.so"}
			if attack != "" {
				files = append(files, attack)
			}
			for _, name := range files {
				h := &tar.Header{Name: name, Mode: 0o755, Typeflag: tar.TypeReg, Size: 1}
				if strings.HasSuffix(name, "/link") {
					h.Typeflag, h.Size, h.Linkname = tar.TypeSymlink, 0, "/etc/passwd"
				}
				if err := tw.WriteHeader(h); err != nil {
					t.Fatal(err)
				}
				if h.Size > 0 {
					if _, err := tw.Write([]byte("x")); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			err = extractVetroffBundle(archive, filepath.Join(folder, "out"))
			if attack == "" && err != nil {
				t.Fatal(err)
			}
			if attack != "" && err == nil {
				t.Fatal("unsafe member accepted")
			}
			if attack == "" {
				if _, err := os.Stat(filepath.Join(folder, "out/foxtunnel-necko/lib/libnss3.so")); err != nil {
					t.Fatal("bundle library lost:", err)
				}
			}
		})
	}
}

func TestOfficialVetroffBundle(t *testing.T) {
	archive := os.Getenv("VETROFF_TEST_ARCHIVE")
	if archive == "" {
		t.Skip("set VETROFF_TEST_ARCHIVE to verify the complete official binary release")
	}
	if err := verifyReleaseDigest(archive, "sha256:69c72c1966ba4714fd2f3df3675ee1e8e5b4cbf500780bc1d90c6fd3bab5e041"); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	if err := extractVetroffBundle(archive, folder); err != nil {
		t.Fatal(err)
	}
	version, err := binaryVersion(t.Context(), filepath.Join(folder, "foxtunnel-necko/vetroff"), []string{"version"})
	if err != nil || version != "0.1.0" {
		t.Fatalf("official release: %s, %v", version, err)
	}
}

func TestVetroffBinaryInstallUpdateAndDigestFailure(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("Linux binary installer")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	version := "0.1.0"
	var archive []byte
	badDigest := false
	makeArchive := func() {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		for _, name := range []string{"vetroff", "foxtunnel-necko", "runtime/xpcshell", "lib/libnss3.so"} {
			data := []byte("#!/bin/sh\nprintf 'VetroffProxy v" + version + "\\n'\n")
			if err := tw.WriteHeader(&tar.Header{Name: "foxtunnel-necko/" + name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(data))}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		archive = buf.Bytes()
	}
	makeArchive()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/binary" {
			_, _ = w.Write(archive)
			return
		}
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(archive))
		if badDigest {
			digest = "sha256:" + strings.Repeat("0", 64)
		}
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v" + version, Assets: []releaseAsset{{Name: "vetroff_" + version + "_linux_" + runtime.GOARCH + ".tar.gz", URL: "https://example.invalid/binary", Digest: digest}}})
	}))
	defer server.Close()
	oldAPI, oldDownload := externalVPNHTTPClient, externalVPNDownloadClient
	client := &http.Client{Transport: hysteriaTestTransport{target: server.URL}}
	externalVPNHTTPClient, externalVPNDownloadClient = client, client
	t.Cleanup(func() { externalVPNHTTPClient, externalVPNDownloadClient = oldAPI, oldDownload })
	if err := Update(t.Context(), model.VetroffProxy); err != nil {
		t.Fatal(err)
	}
	status := binaryStatus(t.Context(), model.VetroffProxy)
	if !status.Installed || status.Version != version || status.UpdateAvailable || status.Error != "" {
		t.Fatalf("installed status: %+v", status)
	}
	version = "0.2.0"
	makeArchive()
	status = binaryStatus(t.Context(), model.VetroffProxy)
	if !status.UpdateAvailable || status.LatestVersion != version {
		t.Fatalf("update not offered: %+v", status)
	}
	badDigest = true
	if err := Update(t.Context(), model.VetroffProxy); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("bad digest accepted: %v", err)
	}
	status = binaryStatus(t.Context(), model.VetroffProxy)
	if status.Version != "0.1.0" {
		t.Fatalf("working installation lost: %+v", status)
	}
	badDigest = false
	if err := Update(t.Context(), model.VetroffProxy); err != nil {
		t.Fatal(err)
	}
	status = binaryStatus(t.Context(), model.VetroffProxy)
	if status.Version != version || status.UpdateAvailable {
		t.Fatalf("update failed: %+v", status)
	}
	if _, err := os.Stat(filepath.Join(dir, "vetroff-bundle/foxtunnel-necko/lib/libnss3.so")); err != nil {
		t.Fatal("full binary bundle was not preserved:", err)
	}
}
