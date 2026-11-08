package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNativeTelemtWebCarrierConfig(t *testing.T) {
	binary := os.Getenv("TELEMT_TEST_BINARY")
	if binary == "" {
		t.Skip("set TELEMT_TEST_BINARY for native WEB transport validation")
	}
	for _, tc := range []struct {
		name   string
		site   *TelemtWebSiteOptions
		marker string
	}{
		{"automatic", nil, "example.com — Fieldnotes"},
		{"catalog-notes", &TelemtWebSiteOptions{Mode: "catalog", Catalog: "builtin", Template: "notes"}, "Fieldnotes"},
		{"catalog-stub", &TelemtWebSiteOptions{Mode: "catalog", Catalog: "mtproxyl", Template: "stub"}, "Сайт временно недоступен"},
		{"catalog-catrunner", &TelemtWebSiteOptions{Mode: "catalog", Catalog: "mtproxyl", Template: "catrunner"}, "Cat Runner"},
		{"catalog-mekorunner", &TelemtWebSiteOptions{Mode: "catalog", Catalog: "mtproxyl", Template: "mekorunner"}, "Meko Runner"},
		{"generated", &TelemtWebSiteOptions{Mode: "generate", Theme: "travel", Title: "Native Travel Check"}, "Native Travel Check"},
		{"imported", &TelemtWebSiteOptions{Mode: "html", HTML: "<!doctype html><html><head><title>Native Imported Page</title></head><body>Ready</body></html>"}, "Native Imported Page"},
	} {
		t.Run(tc.name, func(t *testing.T) { testNativeTelemtWebSite(t, binary, tc.site, tc.marker) })
	}
}

func testNativeTelemtWebSite(t *testing.T, binary string, site *TelemtWebSiteOptions, marker string) {
	dir := t.TempDir()
	freePort := func() int {
		t.Helper()
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
		return port
	}
	rawPort, webPort := freePort(), freePort()
	siteDir, err := prepareTelemtWebSite("example.com", filepath.Join(dir, "default"), filepath.Join(dir, "sites"), site)
	if err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("[general]\ndata_path = %q\nuse_middle_proxy = false\n\n[general.modes]\nclassic = true\nsecure = true\ntls = false\n\n[network]\nipv4 = true\nipv6 = false\n\n[server]\nport = %d\n\n[[server.listeners]]\nip = \"127.0.0.1\"\n\n[access.users]\nxui = \"0123456789abcdef0123456789abcdef\"\n\n[[upstreams]]\ntype = \"direct\"\n", dir, rawPort)
	config, err := appendTelemtWebProxyConfig(base, TelemtWebProxyState{Enabled: true, Domain: "example.com", Secret: "abcdef0123456789abcdef0123456789", PublicAddr: "203.0.113.1:443", ListenPort: webPort, DecoyDir: siteDir, Carrier: "https", Carriers: []string{"websocket-lanes", "websocket", "https-lanes", "https"}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "telemt.toml")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "run", path)
	cmd.Dir = dir
	log, err := os.Create(filepath.Join(dir, "telemt.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	defer func() { _ = cmd.Process.Kill(); <-exited }()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(25 * time.Second)
	for {
		select {
		case <-exited:
			logs, _ := os.ReadFile(log.Name())
			t.Fatalf("Telemt exited before WEB startup: %s", logs)
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(webPort))+"/", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "example.com"
		resp, err := client.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if readErr != nil || !strings.Contains(string(body), marker) {
					t.Fatalf("Telemt did not serve the generated cover site: %s, %v", body, readErr)
				}
				entries, err := os.ReadDir(siteDir)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if entry.Name() == "index.html" {
						continue
					}
					assetReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(webPort))+"/"+entry.Name(), nil)
					if err != nil {
						t.Fatal(err)
					}
					assetReq.Host = "example.com"
					asset, err := client.Do(assetReq)
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(io.LimitReader(asset.Body, 256<<10))
					_ = asset.Body.Close()
					expected, readErr := os.ReadFile(filepath.Join(siteDir, entry.Name()))
					if err != nil || readErr != nil || asset.StatusCode != http.StatusOK || string(data) != string(expected) {
						t.Fatalf("asset %s not served correctly: HTTP %d, %v, %v", entry.Name(), asset.StatusCode, err, readErr)
					}
					if strings.HasSuffix(entry.Name(), ".css") && !strings.Contains(asset.Header.Get("Content-Type"), "text/css") {
						t.Fatal("invalid CSS content type")
					}
					if strings.HasSuffix(entry.Name(), ".js") && !strings.Contains(asset.Header.Get("Content-Type"), "javascript") {
						t.Fatal("invalid JavaScript content type")
					}
				}
				return
			}
		}
		if time.Now().After(deadline) {
			logs, _ := os.ReadFile(log.Name())
			t.Fatalf("native Telemt rejected WEB carriers or did not start: %v\n%s", err, logs)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
