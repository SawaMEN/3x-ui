package zapret2

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type zapret2RoundTrip func(*http.Request) (*http.Response, error)

func (f zapret2RoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func fixtureArchive(t *testing.T, tag string) []byte {
	t.Helper()
	arch := "x86_64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	files := map[string]string{
		"lua/zapret-lib.lua":                 "return {}",
		"lua/zapret-antidpi.lua":             "return {}",
		"blockcheck2.sh":                     "#!/bin/sh\nexit 0\n",
		"binaries/linux-" + arch + "/nfqws2": "#!/bin/sh\nexit 0\n",
		"binaries/linux-" + arch + "/mdig":   "#!/bin/sh\nexit 0\n",
		"binaries/linux-" + arch + "/ip2net": "#!/bin/sh\nexit 0\n",
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		entry := tag + "/" + name
		if err := tw.WriteHeader(&tar.Header{
			Name: entry, Mode: 0o755, Typeflag: tar.TypeReg, Size: int64(len(body)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func mockOfficialRelease(t *testing.T, archive []byte, digest string) {
	t.Helper()
	old := client
	tag := "v9.9.9"
	name := "zapret2-" + tag + ".tar.gz"
	asset := "https://github.com/bol-van/zapret2/releases/download/" + tag + "/" + name
	meta, err := json.Marshal(map[string]any{
		"tag_name": tag,
		"assets":   []map[string]string{{"name": name, "browser_download_url": asset, "digest": digest}},
	})
	if err != nil {
		t.Fatal(err)
	}
	client = &http.Client{
		Transport: zapret2RoundTrip(func(req *http.Request) (*http.Response, error) {
			var body []byte
			switch req.URL.String() {
			case releasesAPI:
				body = meta
			case asset:
				body = archive
			default:
				return nil, fmt.Errorf("unexpected endpoint %s", req.URL)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}
	t.Cleanup(func() { client = old })
}

func TestInstallOfficialReleaseAndRemove(t *testing.T) {
	if !supported() {
		t.Skip("requires supported Linux architecture")
	}
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	archive := fixtureArchive(t, "zapret2-v9.9.9")
	hash := sha256.Sum256(archive)
	mockOfficialRelease(t, archive, fmt.Sprintf("sha256:%x", hash))
	status, err := Update(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Installed || status.Version != "v9.9.9" || !Installed() {
		t.Fatalf("installed status mismatch: %+v", status)
	}
	for _, file := range []string{
		BinaryPath(),
		filepath.Join(Directory(), "nfq2", "nfqws2"),
		filepath.Join(Directory(), "mdig", "mdig"),
		filepath.Join(Directory(), "ip2net", "ip2net"),
		filepath.Join(Directory(), "blockcheck2.sh"),
	} {
		st, err := os.Stat(file)
		if err != nil || st.Mode().Perm()&0o111 == 0 {
			t.Fatalf("missing executable %s: %v", file, err)
		}
	}
	checked, err := GetStatus(context.Background())
	if err != nil || checked.Version != status.Version || checked.UpdateAvailable {
		t.Fatalf("invalid update status: %+v, %v", checked, err)
	}
	if err := Remove(); err != nil {
		t.Fatal(err)
	}
	if Installed() {
		t.Fatal("component still reported installed after removal")
	}
}

func TestInstallFromOfficialReleasePageWhenAPIIsRateLimited(t *testing.T) {
	if !supported() {
		t.Skip("requires supported Linux architecture")
	}
	const tag = "v9.9.9"
	name := releaseArchiveName(tag)
	archive := fixtureArchive(t, "zapret2-"+tag)
	hash := fmt.Sprintf("sha256:%x", sha256.Sum256(archive))
	for _, test := range []struct {
		name       string
		statusCode int
		digest     string
		wantError  string
	}{
		{"forbidden", http.StatusForbidden, hash, ""},
		{"too-many-requests", http.StatusTooManyRequests, hash, ""},
		{"missing-digest", http.StatusForbidden, "", "missing SHA-256 digest"},
		{"wrong-digest", http.StatusForbidden, "sha256:" + strings.Repeat("0", 64), "SHA-256 mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("XUI_BIN_FOLDER", t.TempDir())
			old := client
			t.Cleanup(func() { client = old })
			assetDownloaded := false
			client = &http.Client{Transport: zapret2RoundTrip(func(req *http.Request) (*http.Response, error) {
				resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: req}
				var contents string
				switch req.Method + " " + req.URL.String() {
				case "GET " + releasesAPI:
					resp.StatusCode = test.statusCode
					resp.Status = fmt.Sprintf("%d %s", test.statusCode, http.StatusText(test.statusCode))
				case "HEAD " + releasesWeb + "latest":
					resp.StatusCode = http.StatusFound
					resp.Header.Set("Location", releasesWeb+"tag/"+tag)
				case "HEAD " + releasesWeb + "tag/" + tag:
				case "GET " + releasesWeb + "expanded_assets/" + tag:
					contents = fmt.Sprintf(`<a href="/bol-van/zapret2/releases/download/%s/%s">%s</a>
<clipboard-copy aria-label="Copy to clipboard digest for zapret2-%s.zip" value="%s"></clipboard-copy>
<clipboard-copy aria-label="Copy to clipboard digest for %s" value="%s"></clipboard-copy>`, tag, name, name, tag, hash, name, test.digest)
				case "GET " + releasesWeb + "download/" + tag + "/" + name:
					assetDownloaded = true
					contents = string(archive)
				default:
					return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
				}
				resp.Body = io.NopCloser(strings.NewReader(contents))
				return resp, nil
			})}
			status, err := Update(context.Background())
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) || Installed() {
					t.Fatalf("expected %q and no installation, got %+v, %v", test.wantError, status, err)
				}
				if test.digest == "" && assetDownloaded {
					t.Fatal("downloaded an archive without its digest")
				}
				return
			}
			if err != nil || !status.Installed || status.Version != tag || !Installed() {
				t.Fatalf("installation via release page failed: %+v, %v", status, err)
			}
		})
	}
}

func TestOfficialReleaseDigestMismatchDoesNotReplaceExistingComponent(t *testing.T) {
	if !supported() {
		t.Skip("requires supported Linux architecture")
	}
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(BinaryPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(BinaryPath(), []byte("original binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := fixtureArchive(t, "zapret2-v9.9.9")
	mockOfficialRelease(t, archive, "sha256:"+strings.Repeat("0", 64))
	if _, err := Update(context.Background()); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("expected a digest mismatch, got %v", err)
	}
	data, err := os.ReadFile(BinaryPath())
	if err != nil || string(data) != "original binary" {
		t.Fatalf("failed update replaced existing binary: %q, %v", data, err)
	}
}

func TestRejectArchivePathTraversal(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "unsafe.tar.gz")
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	content := "should never escape"
	if err := tw.WriteHeader(&tar.Header{Name: "../escaped.txt", Typeflag: tar.TypeReg, Size: int64(len(content)), Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(temp, "extract")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := unpackArchive(path, dest); err == nil {
		t.Fatal("untrusted archive path traversal was accepted")
	}
	if _, err := os.Stat(filepath.Join(temp, "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("path traversal wrote outside extraction directory: %v", err)
	}
}
