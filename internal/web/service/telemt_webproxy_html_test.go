package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTelemtImportedHTMLUsesCSPAssetsWithoutChangingGatewayHTML(t *testing.T) {
	page := `<!doctype html><HTML><HEAD><STYLE media="screen">h1{color:red}</STYLE></HEAD><BODY><h1>Cover</h1><SCRIPT>window.steps=['first']</SCRIPT><div id="middle"></div><SCRIPT TYPE="module">window.steps.push('second')</SCRIPT><script type="application/ld+json">{"name":"Cover"}</script></BODY></HTML>`
	root := t.TempDir()
	options := &TelemtWebSiteOptions{Mode: "html", HTML: page}
	original, err := prepareTelemtWebSite("example.com", filepath.Join(root, "default"), filepath.Join(root, "sites"), options)
	if err != nil {
		t.Fatal(err)
	}
	safe, err := prepareTelemtWebProxySite("example.com", filepath.Join(root, "default"), filepath.Join(root, "sites"), options)
	if err != nil {
		t.Fatal(err)
	}
	if original == safe {
		t.Fatal("CSP conversion reused the raw active snapshot")
	}
	unchanged, err := os.ReadFile(filepath.Join(original, "index.html"))
	if err != nil || string(unchanged) != page {
		t.Fatal("ordinary gateway owner HTML was changed")
	}
	index, err := os.ReadFile(filepath.Join(safe, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(index)
	for _, want := range []string{`href="/site-inline-1.css"`, `media="screen"`, `src="/site-inline-2.js"`, `type="module"`, `src="/site-inline-3.js"`, `<script type="application/ld+json">`} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %s in %s", want, content)
		}
	}
	if strings.Index(content, "site-inline-2.js") > strings.Index(content, `id="middle"`) || strings.Index(content, "site-inline-3.js") < strings.Index(content, `id="middle"`) {
		t.Fatal("script positions changed")
	}
	if safe != telemtWebImportedSnapshotPath(filepath.Join(root, "sites"), page) {
		t.Fatal("reconcile cannot recognise the converted snapshot")
	}
	again, err := prepareTelemtWebProxySite("example.com", filepath.Join(root, "default"), filepath.Join(root, "sites"), options)
	if err != nil || again != safe {
		t.Fatal("converted snapshot is not idempotent")
	}
}
