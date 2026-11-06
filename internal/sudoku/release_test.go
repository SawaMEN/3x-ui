package sudoku

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func jsonReleaseResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

// CI also runs this package with the panel's development build ldflags.
// The official stable release must be used in both panel channels.
func TestFetchOfficialRelease(t *testing.T) {
	client := &http.Client{Transport: releaseTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://api.github.com/repos/SUDOKU-ASCII/sudoku/releases/latest" {
			t.Fatalf("unexpected release URL: %s", req.URL)
		}
		return jsonReleaseResponse(`{"tag_name":"v0.5.2.0"}`), nil
	})}
	got, err := fetchRelease(context.Background(), client)
	if err != nil || got.TagName != "v0.5.2.0" {
		t.Fatalf("release = %+v, error = %v", got, err)
	}
}

func TestReleaseVersion(t *testing.T) {
	for _, tc := range []struct{ tag, want string }{
		{" v0.5.2.0 ", "v0.5.2.0"}, {"", ""}, {"  ", ""},
	} {
		got, err := releaseVersion(release{TagName: tc.tag})
		if got != tc.want || (err != nil) != (tc.want == "") {
			t.Fatalf("version = %q, error = %v; want %q", got, err, tc.want)
		}
	}
}

func TestInstallAndStatusOfficialRelease(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("Linux component")
	}
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "official CLI", false: "invalid executable"}[valid], func(t *testing.T) {
			binDir := t.TempDir()
			t.Setenv("XUI_BIN_FOLDER", binDir)
			// A previous panel build must be offered an update and replaced without
			// changing the existing keys/configuration directory.
			const previous = "previous executable"
			if err := os.WriteFile(GetBinaryPath(binDir), []byte(previous), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := writeInstalledVersion(binDir, "dev+0123456789abcdef0123456789abcdef01234567"); err != nil {
				t.Fatal(err)
			}
			payload := "#!/bin/sh\necho 'usage: -c string -test -keygen'\n"
			if !valid {
				payload = "#!/bin/sh\nexit 1\n"
			}
			var archive bytes.Buffer
			gz := gzip.NewWriter(&archive)
			tw := tar.NewWriter(gz)
			if err := tw.WriteHeader(&tar.Header{Name: "sudoku", Mode: 0o755, Size: int64(len(payload))}); err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(tw, payload); err != nil {
				t.Fatal(err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			const assetURL = "https://github.com/SUDOKU-ASCII/sudoku/releases/download/v0.5.2.0/sudoku-linux-test.tar.gz"
			tag := "v0.5.2.0"
			oldClient := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = oldClient })
			http.DefaultClient = &http.Client{Transport: releaseTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() == assetURL {
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(archive.Bytes()))}, nil
				}
				if req.URL.String() != "https://api.github.com/repos/SUDOKU-ASCII/sudoku/releases/latest" {
					t.Fatalf("unexpected request: %s", req.URL)
				}
				data, err := json.Marshal(map[string]any{"tag_name": tag, "assets": []map[string]string{{"name": "sudoku-linux-" + runtime.GOARCH + ".tar.gz", "browser_download_url": assetURL}}})
				if err != nil {
					t.Fatal(err)
				}
				return jsonReleaseResponse(string(data)), nil
			})}
			status, err := GetStatus(context.Background())
			if err != nil || !status.UpdateAvailable {
				t.Fatalf("legacy build update not detected: %+v %v", status, err)
			}
			path, err := InstallLatest(context.Background(), binDir)
			if !valid {
				if err == nil {
					t.Fatal("invalid executable accepted")
				}
				data, _ := os.ReadFile(GetBinaryPath(binDir))
				if string(data) != previous {
					t.Fatal("failed installation replaced working executable")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != payload {
				t.Fatalf("installed payload = %q, error = %v", data, err)
			}
			status, err = GetStatus(context.Background())
			if err != nil || !status.Installed || !status.Supported || status.UpdateAvailable || status.Version != tag || status.LatestVersion != tag {
				t.Fatalf("status after install: %+v %v", status, err)
			}
			tag = "v0.5.3"
			status, err = GetStatus(context.Background())
			if err != nil || !status.UpdateAvailable || status.LatestVersion != tag {
				t.Fatalf("new upstream release not detected: %+v %v", status, err)
			}
		})
	}
}

func TestStatusShowsReleaseFailureAndHidesRemovedVersion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	if err := writeInstalledVersion(dir, "v0.5.2.0"); err != nil {
		t.Fatal(err)
	}
	oldClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldClient })
	requests := 0
	http.DefaultClient = &http.Client{Transport: releaseTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusForbidden, Status: "403 Forbidden", Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
	})}
	local, err := GetStatus(context.Background(), true)
	if err != nil || local.Version != "" || local.Installed || requests != 0 {
		t.Fatalf("removed local status: %+v %v requests=%d", local, err, requests)
	}
	remote, err := GetStatus(context.Background())
	if err != nil || remote.Error == "" || remote.Version != "" || !remote.Supported {
		t.Fatalf("release failure hidden: %+v %v", remote, err)
	}
}
