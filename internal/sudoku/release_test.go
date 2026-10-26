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

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

const testReleaseCommit = "0123456789abcdef0123456789abcdef01234567"

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func jsonReleaseResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func TestFetchReleaseChannel(t *testing.T) {
	for _, dev := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable", true: "dev"}[dev], func(t *testing.T) {
			wantURL := releasesAPI + "/latest"
			rel := release{TagName: "v3.9.8"}
			if dev {
				wantURL = releasesAPI + "/tags/dev-latest"
				rel = release{TagName: "dev-latest", Body: "Rolling development build from main. commit=" + testReleaseCommit}
			}
			client := &http.Client{Transport: releaseTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != wantURL {
					t.Fatalf("release URL = %s, want %s", req.URL, wantURL)
				}
				data, err := json.Marshal(rel)
				if err != nil {
					t.Fatal(err)
				}
				return jsonReleaseResponse(string(data)), nil
			})}
			got, err := fetchRelease(context.Background(), client, dev)
			if err != nil || got.TagName != rel.TagName {
				t.Fatalf("release = %+v, error = %v", got, err)
			}
		})
	}
}

func TestRollingReleaseVersion(t *testing.T) {
	for _, tc := range []struct{ name, tag, body, want string }{
		{"stable", " v3.9.8 ", "", "v3.9.8"},
		{"rolling", "dev-latest", "Rolling build\ncommit=" + testReleaseCommit, "dev+" + testReleaseCommit},
		{"missing tag", "", "", ""},
		{"missing commit", "dev-latest", "Rolling build", ""},
		{"short commit", "dev-latest", "commit=01234567", ""},
		{"invalid commit", "dev-latest", "commit=" + strings.Repeat("z", 40), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := releaseVersion(release{TagName: tc.tag, Body: tc.body})
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("version = %q, error = %v; want %q", got, err, tc.want)
			}
		})
	}
}

// Exercise the same public install/status functions in stable and dev builds.
// The dev CI invocation injects config.buildCommit using the release ldflags.
func TestInstallAndStatusRelease(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("panel sidecar is published for Linux amd64/arm64")
	}
	for _, compatible := range []bool{true, false} {
		t.Run(map[bool]string{true: "panel egress", false: "reject upstream"}[compatible], func(t *testing.T) {
			binDir := t.TempDir()
			t.Setenv("XUI_BIN_FOLDER", binDir)
			payload := "#!/bin/sh\necho 'usage: -panel-egress address'\n"
			if !compatible {
				payload = "#!/bin/sh\necho 'usage: stock upstream'\n"
			}
			var archive bytes.Buffer
			gz := gzip.NewWriter(&archive)
			tw := tar.NewWriter(gz)
			if err := tw.WriteHeader(&tar.Header{Name: "sudoku-tunnel", Mode: 0755, Size: int64(len(payload))}); err != nil {
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

			revision := testReleaseCommit
			oldClient := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = oldClient })
			http.DefaultClient = &http.Client{Transport: releaseTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() == "https://example.com/panel-tunnel.tar.gz" {
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(archive.Bytes()))}, nil
				}
				wantURL := releasesAPI + "/latest"
				tag := "v3.9.8"
				if config.IsDevBuild() {
					wantURL = releasesAPI + "/tags/dev-latest"
					tag = "dev-latest"
				}
				if req.URL.String() != wantURL {
					t.Fatalf("unexpected request: %s", req.URL)
				}
				data, err := json.Marshal(map[string]any{
					"tag_name": tag, "body": "Rolling build commit=" + revision,
					"assets": []map[string]string{{"name": "panel-tunnel-linux-" + runtime.GOARCH + ".tar.gz", "browser_download_url": "https://example.com/panel-tunnel.tar.gz"}},
				})
				if err != nil {
					t.Fatal(err)
				}
				return jsonReleaseResponse(string(data)), nil
			})}

			path, err := InstallLatest(context.Background(), binDir)
			if !compatible {
				if err == nil || !strings.Contains(err.Error(), "lacks panel-egress") {
					t.Fatalf("incompatible install error = %v", err)
				}
				if IsInstalled(binDir) {
					t.Fatal("installed incompatible binary")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != payload {
				t.Fatalf("installed payload = %q, error = %v", data, err)
			}
			status, err := GetStatus(context.Background())
			if err != nil || !status.Installed || !status.Supported || status.UpdateAvailable {
				t.Fatalf("status after install: %+v, error = %v", status, err)
			}
			wantVersion := "v3.9.8"
			if config.IsDevBuild() {
				wantVersion = "dev+" + revision
			}
			if status.Version != wantVersion || status.LatestVersion != wantVersion {
				t.Fatalf("wrong version: %+v", status)
			}
			if config.IsDevBuild() {
				revision = strings.Repeat("a", 40)
				status, err = GetStatus(context.Background())
				if err != nil || !status.UpdateAvailable || status.LatestVersion != "dev+"+revision {
					t.Fatalf("new rolling build not detected: %+v, error = %v", status, err)
				}
			}
		})
	}
}
