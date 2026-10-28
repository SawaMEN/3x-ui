package coreinstall

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func archiveFixture(t *testing.T, name, binary string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if strings.HasSuffix(name, ".zip") {
		w := zip.NewWriter(&buf)
		f, e := w.Create(binary)
		if e != nil {
			t.Fatal(e)
		}
		f.Write([]byte("new executable"))
		for _, name := range []string{"geoip.dat", "geosite.dat"} {
			file, _ := w.Create(name)
			_, _ = file.Write([]byte("geodata"))
		}
		w.Close()
	} else {
		g := gzip.NewWriter(&buf)
		w := tar.NewWriter(g)
		e := w.WriteHeader(&tar.Header{Name: strings.TrimSuffix(name, ".tar.gz") + "/" + binary, Mode: 0o755, Size: 14})
		if e != nil {
			t.Fatal(e)
		}
		w.Write([]byte("new executable"))
		w.Close()
		g.Close()
	}
	return buf.Bytes()
}

func TestInstallChecksAndAtomicallyPublishes(t *testing.T) {
	for _, engine := range []string{"xray", "sing-box-upstream"} {
		for _, failure := range []string{"", "checksum", "validation", "http"} {
			t.Run(engine+"/"+failure, func(t *testing.T) {
				dir := t.TempDir()
				target := filepath.Join(dir, "core")
				os.WriteFile(target, []byte("old executable"), 0o755)
				name, err := AssetName(engine, "v1.2.3", "linux", "amd64", "")
				if err != nil {
					t.Fatal(err)
				}
				binary := "sing-box"
				if engine == "xray" {
					binary = "xray"
				}
				data := archiveFixture(t, name, binary)
				sum := sha256.Sum256(data)
				digest := hex.EncodeToString(sum[:])
				if failure == "checksum" {
					digest = strings.Repeat("0", 64)
				}
				repo, _ := repository(engine)
				url := "https://github.com/" + repo + "/releases/download/v1.2.3/" + name
				release, _ := json.Marshal(Release{Tag: "v1.2.3", Assets: []Asset{{Name: name, URL: url, Digest: "sha256:" + digest}}})
				client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
					b := release
					status := 200
					if r.URL.String() == url {
						b = data
						if failure == "http" {
							status = 503
						}
					}
					return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header)}, nil
				})}
				checked := false
				tag, err := (Installer{Client: client}).Install(context.Background(), engine, "latest", "linux", "amd64", "", target, func(staged string) error {
					checked = true
					if staged == target {
						t.Fatal("validating live target")
					}
					b, _ := os.ReadFile(target)
					if string(b) != "old executable" {
						t.Fatal("target replaced before validation")
					}
					b, _ = os.ReadFile(staged)
					if string(b) != "new executable" {
						t.Fatalf("bad binary: %q", b)
					}
					if failure == "validation" {
						return errors.New("wrong engine")
					}
					return nil
				})
				got, _ := os.ReadFile(target)
				if failure == "" {
					if err != nil || tag != "v1.2.3" || !checked || string(got) != "new executable" {
						t.Fatalf("install=%s checked=%v target=%s err=%v", tag, checked, got, err)
					}
				} else {
					if err == nil || string(got) != "old executable" {
						t.Fatalf("failed install modified live binary: %s %v", got, err)
					}
				}
				matches, _ := filepath.Glob(filepath.Join(dir, ".core-install-*"))
				if len(matches) > 0 {
					t.Fatal("staged binaries leaked")
				}
			})
		}
	}
}
func TestAssetNames(t *testing.T) {
	for _, tc := range []struct{ core, arch, arm, want string }{
		{"xray", "amd64", "", "Xray-linux-64.zip"}, {"xray", "arm64", "", "Xray-linux-arm64-v8a.zip"}, {"xray", "arm", "6", "Xray-linux-arm32-v6.zip"},
		{"sing-box-upstream", "arm", "7", "sing-box-1.2.3-linux-armv7.tar.gz"}, {"sing-box-upstream", "mips", "", "sing-box-1.2.3-linux-mips-softfloat.tar.gz"},
	} {
		got, err := AssetName(tc.core, "v1.2.3", "linux", tc.arch, tc.arm)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: %q %v", tc, got, err)
		}
	}
	if _, err := AssetName("unknown", "v1", "linux", "amd64", ""); err == nil {
		t.Fatal("unknown core accepted")
	}
	if _, err := AssetName("xray", "v1", "linux", "arm", "4"); err == nil {
		t.Fatal("unknown ARM accepted")
	}
}
func TestDigestAndArchiveRejection(t *testing.T) {
	want := strings.Repeat("a", 64)
	got, err := xrayDigest([]byte("SHA2-256 (Xray.zip) = " + want + "\nSHA2-512 (Xray.zip) = " + strings.Repeat("b", 128)))
	if err != nil || got != want {
		t.Fatalf("digest=%q err=%v", got, err)
	}
	if _, err := xrayDigest([]byte("MD5 = " + want)); err == nil {
		t.Fatal("wrong digest accepted")
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, _ := z.Create("../xray")
	f.Write([]byte("evil"))
	z.Close()
	if _, err := extract(buf.Bytes(), "release.zip", "xray"); err == nil {
		t.Fatal("unexpected archive member accepted")
	}
}
