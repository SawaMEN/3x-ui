package externalvpn

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

type openFluxTestRelease struct {
	version string
	body    string
	digest  string
	url     string
	name    string
}

func componentTestRelease(version, body string) openFluxTestRelease {
	sum := sha256.Sum256([]byte(body))
	name := "openflux-linux-" + runtime.GOARCH
	return openFluxTestRelease{
		version: version, body: body, digest: fmt.Sprintf("sha256:%x", sum), name: name,
		url: "https://github.com/p1neappleXpress/OpenFlux/releases/download/v" + version + "/" + name,
	}
}

func TestOpenFluxOptionalOfficialComponentLifecycle(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("Linux component installer")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	// A fixture executable stands in for a ready upstream release artifact.
	// The installer must not invoke a compiler or build a source archive.
	first := componentTestRelease("9.8.7", "#!/bin/sh\necho '--role --config --negotiate'\n")
	var current atomic.Value
	current.Store(first)
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := current.Load().(openFluxTestRelease)
		if strings.HasPrefix(r.URL.Path, "/p1neappleXpress/OpenFlux/releases/download/") {
			downloads.Add(1)
			_, _ = w.Write([]byte(release.body))
			return
		}
		if r.URL.Path != "/repos/p1neappleXpress/OpenFlux/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName: "v" + release.version,
			Assets:  []releaseAsset{{Name: release.name, URL: release.url, Digest: release.digest}},
		})
	}))
	defer server.Close()
	previousAPI, previousDownload := externalVPNHTTPClient, externalVPNDownloadClient
	client := &http.Client{Transport: hysteriaTestTransport{target: server.URL}}
	externalVPNHTTPClient, externalVPNDownloadClient = client, client
	t.Cleanup(func() { externalVPNHTTPClient, externalVPNDownloadClient = previousAPI, previousDownload })

	status := OpenFluxBinaryStatus(context.Background())
	if status.Installed || status.Version != "" || status.LatestVersion != first.version || downloads.Load() != 0 {
		t.Fatalf("fresh optional component status: %+v, downloads=%d", status, downloads.Load())
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("checking status installed files: %v, %v", files, err)
	}
	if err := Update(context.Background(), model.OpenFlux); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "openflux"))
	if err != nil || string(data) != first.body || downloads.Load() != 1 {
		t.Fatalf("official ready binary was not installed: %v", err)
	}
	status = OpenFluxBinaryStatus(context.Background())
	if !status.Installed || status.Version != first.version || status.UpdateAvailable {
		t.Fatalf("installed status: %+v", status)
	}
	second := componentTestRelease("9.8.8", "#!/bin/sh\necho '--role --config --negotiate new-release'\n")
	current.Store(second)
	status = OpenFluxBinaryStatus(context.Background())
	if status.Version != first.version || status.LatestVersion != second.version || !status.UpdateAvailable {
		t.Fatalf("latest stable release not detected: %+v", status)
	}
	if err := Update(context.Background(), model.OpenFlux); err != nil {
		t.Fatal(err)
	}
	status = OpenFluxBinaryStatus(context.Background())
	if status.Version != second.version || status.UpdateAvailable {
		t.Fatalf("updated receipt/status: %+v", status)
	}
	for _, reason := range []string{"checksum", "missing digest", "wrong source", "incompatible binary"} {
		t.Run(reason, func(t *testing.T) {
			next := componentTestRelease("9.8.9", "#!/bin/sh\necho '--role --config'\n")
			switch reason {
			case "checksum":
				next.digest = "sha256:" + strings.Repeat("0", 64)
			case "missing digest":
				next.digest = ""
			case "wrong source":
				next.url = "https://example.invalid/" + next.name
			case "incompatible binary":
				next = componentTestRelease("9.8.9", "#!/bin/sh\necho 'unsupported artifact'\n")
			}
			current.Store(next)
			if err := Update(context.Background(), model.OpenFlux); err == nil {
				t.Fatal("invalid upstream artifact was installed")
			}
			data, err := os.ReadFile(filepath.Join(dir, "openflux"))
			if err != nil || string(data) != second.body {
				t.Fatalf("working binary was replaced after failure: %v", err)
			}
			if got := OpenFluxBinaryStatus(context.Background(), true); got.Version != second.version {
				t.Fatalf("working receipt was replaced: %+v", got)
			}
		})
	}
}

func TestOpenFluxComponentRollbackOnReceiptFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	target := filepath.Join(dir, "openflux")
	if err := os.WriteFile(target, []byte("previous working binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".openflux-install.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(dir, ".openflux-staged")
	if err := os.WriteFile(staged, []byte("new binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := commitOpenFluxBinary(staged, "9.8.7", "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatal("receipt commit unexpectedly succeeded")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "previous working binary" {
		t.Fatalf("previous executable was not restored: %v", err)
	}
}
