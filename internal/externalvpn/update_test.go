package externalvpn

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestParseExternalVersion(t *testing.T) {
	for input, want := range map[string]string{
		"pingtunnel 2.10 (linux/amd64)":     "2.10",
		"trusttunnel_endpoint v1.1.0":       "1.1.0",
		"TrustTunnel endpoint 1.2.3-beta.1": "1.2.3-beta.1",
	} {
		if got := parseExternalVersion(input); got != want {
			t.Fatalf("parseExternalVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPickReleaseAsset(t *testing.T) {
	pingSpec, err := releaseSpecFor(model.Pingtunnel)
	if err != nil {
		t.Fatal(err)
	}
	pingRelease := githubRelease{TagName: "2.10", Assets: []releaseAsset{{Name: "pingtunnel_linux_amd64.zip", URL: "https://example.invalid/ping.zip"}}}
	asset, err := pickReleaseAsset(pingSpec, pingRelease, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != "pingtunnel_linux_amd64.zip" {
		t.Fatalf("Pingtunnel asset = %q", asset.Name)
	}

	trustSpec, err := releaseSpecFor(model.TrustTunnel)
	if err != nil {
		t.Fatal(err)
	}
	trustRelease := githubRelease{TagName: "v1.1.0", Assets: []releaseAsset{{Name: "trusttunnel-v1.1.0-linux-aarch64.tar.gz", URL: "https://example.invalid/trust.tar.gz"}}}
	asset, err = pickReleaseAsset(trustSpec, trustRelease, "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != "trusttunnel-v1.1.0-linux-aarch64.tar.gz" {
		t.Fatalf("TrustTunnel asset = %q", asset.Name)
	}
}

func TestVerifyReleaseDigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asset")
	if err := os.WriteFile(path, []byte("3x-ui"), 0o600); err != nil {
		t.Fatal(err)
	}
	const digest = "sha256:1343462835cbb1f7631c393c1090169aee14f90129e0922c7ec1d58fd40b0647"
	if err := verifyReleaseDigest(path, digest); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseDigest(path, "sha256:0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("digest mismatch was accepted")
	}
}

func TestInstallStandaloneHysteriaFromOfficialAsset(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("standalone Linux installer")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	const executable = "#!/bin/sh\nprintf 'Version: v2.13.0\\n'\n"
	digest := sha256.Sum256([]byte(executable))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/binary" {
			_, _ = io.WriteString(w, executable)
			return
		}
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "app/v2.13.0",
			Assets: []releaseAsset{{
				Name:   "hysteria-linux-" + runtime.GOARCH,
				URL:    "https://example.invalid/binary",
				Digest: fmt.Sprintf("sha256:%x", digest),
			}},
		})
	}))
	defer server.Close()
	originalAPI, originalDownload := externalVPNHTTPClient, externalVPNDownloadClient
	client := &http.Client{Transport: hysteriaTestTransport{target: server.URL}}
	externalVPNHTTPClient, externalVPNDownloadClient = client, client
	t.Cleanup(func() { externalVPNHTTPClient, externalVPNDownloadClient = originalAPI, originalDownload })

	if err := Update(context.Background(), model.Hysteria); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(filepath.Join(dir, "hysteria"))
	if err != nil || string(installed) != executable {
		t.Fatalf("installed binary = %q, error = %v", installed, err)
	}
	status := binaryStatus(context.Background(), model.Hysteria)
	if !status.Installed || status.Version != "2.13.0" || status.LatestVersion != "2.13.0" || status.UpdateAvailable || status.Error != "" {
		t.Fatalf("status after install: %+v", status)
	}
}

type hysteriaTestTransport struct{ target string }

func (transport hysteriaTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	forward := request.Clone(request.Context())
	target, err := url.Parse(transport.target)
	if err != nil {
		return nil, err
	}
	forward.URL.Scheme, forward.URL.Host = target.Scheme, target.Host
	return http.DefaultTransport.RoundTrip(forward)
}

func TestLocalComponentStatusDoesNotQueryReleasesOrRunBinaries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	marker := filepath.Join(dir, "executed")
	script := fmt.Sprintf("#!/bin/sh\ntouch %q\nprintf '2.10\\n'\n", marker)
	if err := os.WriteFile(filepath.Join(dir, "pingtunnel"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected upstream request", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	old := externalVPNHTTPClient
	externalVPNHTTPClient = &http.Client{Transport: hysteriaTestTransport{target: server.URL}}
	t.Cleanup(func() { externalVPNHTTPClient = old })
	status := GetUpdateStatus(context.Background(), true)
	if !status.Pingtunnel.Installed || calls.Load() != 0 {
		t.Fatalf("local status: %+v, upstream requests: %d", status, calls.Load())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("local status executed a binary: %v", err)
	}
}
