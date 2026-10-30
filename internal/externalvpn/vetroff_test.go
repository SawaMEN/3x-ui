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
	inst.Settings.Clients[0].ExpiryTime = time.Now().Add(-time.Hour).UnixMilli()
	if _, err := exportVetroffLink(inst, "alice", "vpn.example.com", 8443); err == nil {
		t.Fatal("expired user exported")
	}
	inst.Settings.Clients[0].ExpiryTime = 0
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
		nativeOptions := DefaultVetroffSettings()
		nativeOptions.AutoTLS = false
		nativeOptions.Padding, nativeOptions.AllowPrivate = true, true
		inst.Settings.Vetroff = &nativeOptions
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
		previousPID := manager.procs[inst.ID].cmd.Process.Pid
		renewal := inst
		renewal.Settings.Certificate, renewal.Settings.PrivateKey = "", ""
		renewedCert, renewedKey, err := certificate(renewal, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		for target, source := range map[string]string{cert: renewedCert, key: renewedKey} {
			contents, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, contents, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := manager.Ensure(inst); err != nil {
			t.Fatal(err)
		}
		if manager.procs[inst.ID].cmd.Process.Pid == previousPID {
			t.Fatal("server did not reload a renewed TLS certificate at the same path")
		}
		if err := manager.Ensure(inst); err != nil {
			t.Fatal(err)
		}
		currentPID := manager.procs[inst.ID].cmd.Process.Pid
		if err := manager.Ensure(inst); err != nil {
			t.Fatal(err)
		}
		if manager.procs[inst.ID].cmd.Process.Pid != currentPID {
			t.Fatal("unchanged TLS certificate caused repeated restarts")
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
	for _, attack := range []string{"", "vetroff-necko/../../escape", "/tmp/escape", "vetroff-necko/link"} {
		t.Run(attack, func(t *testing.T) {
			folder := t.TempDir()
			archive := filepath.Join(folder, "release.tar.gz")
			f, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(f)
			tw := tar.NewWriter(gz)
			files := []string{"vetroff-necko/vetroff", "vetroff-necko/vetroff-necko", "vetroff-necko/runtime/xpcshell", "vetroff-necko/lib/libnss3.so"}
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
				if _, err := os.Stat(filepath.Join(folder, "out/vetroff-necko/lib/libnss3.so")); err != nil {
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
	if err := verifyReleaseDigest(archive, "sha256:492d07be1b430d8fe8cfbe486dd4d3f006e4dcd04399ada2c7091f45380998d3"); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	if err := extractVetroffBundle(archive, folder); err != nil {
		t.Fatal(err)
	}
	version, err := binaryVersion(t.Context(), filepath.Join(folder, "vetroff-necko/vetroff"), []string{"version"})
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
		for _, name := range []string{"vetroff", "vetroff-necko", "runtime/xpcshell", "lib/libnss3.so"} {
			data := []byte("#!/bin/sh\nprintf 'VetroffProxy v" + version + "\\n'\n")
			if err := tw.WriteHeader(&tar.Header{Name: "vetroff-necko/" + name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(data))}); err != nil {
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
	if _, err := os.Stat(filepath.Join(dir, "vetroff-bundle/vetroff-necko/lib/libnss3.so")); err != nil {
		t.Fatal("full binary bundle was not preserved:", err)
	}
}

func TestVetroffAllTransportProfilesAndServerOptions(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	inst := Instance{ID: 831, Protocol: model.VetroffProxy, Listen: "127.0.0.1", Port: 18443, Settings: Settings{Hostname: "vpn.example.com", Clients: []Client{{Email: "alice", Password: strings.Repeat("v", 48), Enable: true}}}}
	folder := filepath.Join(directory(), "test-tls")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	cert, key, err := certificate(inst, folder)
	if err != nil {
		t.Fatal(err)
	}
	inst.Settings.Certificate, inst.Settings.PrivateKey = cert, key
	binary := os.Getenv("VETROFF_TEST_BINARY")
	for _, transport := range []string{"h2-connect", "h2-post", "h3-connect", "h3-post", "wss", "xhttp-stream-one", "xhttp-stream-up", "xhttp-packet-up"} {
		t.Run(transport, func(t *testing.T) {
			options := DefaultVetroffSettings()
			options.AutoTLS = false
			options.Transport = transport
			options.ProtocolPolicy = "strict"
			options.MaxConnections = 123
			options.MaxTunnels = 61
			options.XHTTPPath = "/custom-carrier/"
			options.XHTTPHost = "front.example.com"
			options.Balance = "least-active"
			options.Pipeline = "4"
			options.ConnectTimeout = "13s"
			options.AttemptTimeout = "4s"
			options.EgressProxy = "socks5://127.0.0.1:19080"
			options.EgressDNS = "9.9.9.9:53"
			options.UserPolicies = []VetroffUserPolicy{{Email: "alice", MaxRequests: 17, MaxTunnels: 9, RequestsPerSecond: 30, Burst: 60, QuotaBytes: 4096}}
			options.FallbackServers = []string{"https://backup.example.com:18443"}
			inst.Settings.Vetroff = &options
			if err := inst.Validate(); err != nil {
				t.Fatal(err)
			}
			args, err := vetroffFiles(inst)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(args[2])
			if err != nil {
				t.Fatal(err)
			}
			var conf map[string]any
			if err := json.Unmarshal(data, &conf); err != nil {
				t.Fatal(err)
			}
			if conf["max_connections"] != float64(123) || conf["max_tunnels"] != float64(61) || conf["egress_proxy"] != options.EgressProxy || conf["xhttp_host"] != options.XHTTPHost {
				t.Fatalf("server options lost: %s", data)
			}
			data, err = os.ReadFile(conf["users_file"].(string))
			if err != nil {
				t.Fatal(err)
			}
			var policy struct {
				Users []struct {
					MaxTunnels int `json:"max_tunnels"`
					Quota      int `json:"quota_bytes"`
				}
			}
			if err := json.Unmarshal(data, &policy); err != nil {
				t.Fatal(err)
			}
			if len(policy.Users) != 1 || policy.Users[0].Quota != 4096 || policy.Users[0].MaxTunnels != 9 {
				t.Fatalf("native policy lost: %s", data)
			}
			link, err := exportVetroffLink(inst, "alice", "vpn.example.com", 18443)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "connection.txt")
			if err := os.WriteFile(file, []byte(link), 0o600); err != nil {
				t.Fatal(err)
			}
			if binary != "" {
				for _, command := range [][]string{{"check-config", "--config", args[2]}, {"check-config", "--link-file", file, "--ca-cert", cert}} {
					if out, err := exec.Command(binary, command...).CombinedOutput(); err != nil {
						t.Fatalf("official release rejected %s: %v: %s", transport, err, out)
					}
				}
			}
		})
	}
}

func TestVetroffBondClusterGeneratedAndChecked(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	options := DefaultVetroffSettings()
	options.AutoTLS = false
	options.Bond, options.Discover, options.BondHub = true, true, true
	inst := Instance{ID: 832, Protocol: model.VetroffProxy, Port: 18443, Settings: Settings{Hostname: "vpn.example.com", Vetroff: &options, Clients: []Client{{Email: "alice", Password: strings.Repeat("v", 48), Enable: true}}}}
	folder := filepath.Join(directory(), "test-tls")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	cert, key, err := certificate(inst, folder)
	if err != nil {
		t.Fatal(err)
	}
	inst.Settings.Certificate, inst.Settings.PrivateKey = cert, key
	if err := inst.Validate(); err != nil {
		t.Fatal(err)
	}
	args, err := vetroffFiles(inst)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(directory(), "832", "vetroff-cluster.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cluster struct {
		Nodes []struct {
			URL    string
			Weight int
		}
	}
	if err := json.Unmarshal(raw, &cluster); err != nil {
		t.Fatal(err)
	}
	if len(cluster.Nodes) != 1 || cluster.Nodes[0].URL != "https://vpn.example.com:18443" || cluster.Nodes[0].Weight != 100 {
		t.Fatalf("automatic cluster invalid: %s", raw)
	}
	if binary := os.Getenv("VETROFF_TEST_BINARY"); binary != "" {
		if out, err := exec.Command(binary, "check-config", "--config", args[2]).CombinedOutput(); err != nil {
			t.Fatalf("official release rejected generated hub: %v: %s", err, out)
		}
	}
	options.ProtocolPolicy = "strict"
	if err := inst.Validate(); err == nil || !strings.Contains(err.Error(), "strict policy") {
		t.Fatalf("incompatible Bond policy accepted: %v", err)
	}
}

func TestVetroffRejectsNativeProfileAliases(t *testing.T) {
	options := DefaultVetroffSettings()
	options.AutoTLS, options.AutoPin = false, false
	inst := Instance{Protocol: model.VetroffProxy, Port: 443, Settings: Settings{Hostname: "vpn.example.com", Certificate: "/cert.pem", PrivateKey: "/key.pem", Vetroff: &options, Clients: []Client{{Email: "alice", Password: strings.Repeat("v", 48), Enable: true}}}}
	for _, fallback := range []string{"https://VPN.example.com", "https://vpn.example.com:0443/"} {
		options.FallbackServers = []string{fallback}
		if err := inst.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate VetroffProxy server") {
			t.Fatalf("native client would reject duplicate %q: %v", fallback, err)
		}
	}
	options.FallbackServers = nil
	inst.Settings.Hostname = "-invalid.example.com"
	if err := inst.Validate(); err == nil || !strings.Contains(err.Error(), "requires a hostname") {
		t.Fatalf("invalid native SNI accepted: %v", err)
	}
}

func TestVetroffMigratesSavedProtocolPolicy(t *testing.T) {
	var settings VetroffSettings
	if err := json.Unmarshal([]byte(`{"protocolPolicy":"legacy"}`), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.ProtocolPolicy != "builtin" {
		t.Fatalf("saved policy was not migrated: %q", settings.ProtocolPolicy)
	}
	inst := Instance{Settings: Settings{Vetroff: &VetroffSettings{ProtocolPolicy: "legacy"}}}
	if got := vetroffOptions(inst).ProtocolPolicy; got != "builtin" {
		t.Fatalf("runtime policy = %q, want builtin", got)
	}
}
