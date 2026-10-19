package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTelemtWebDecoyPreservesSite(t *testing.T) {
	dir := t.TempDir()
	if err := writeTelemtWebDecoy("example.com", dir); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(dir, "index.html")
	page, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "example.com") || strings.Contains(string(page), "{{DOMAIN}}") {
		t.Fatal("domain was not rendered")
	}
	custom := []byte("<!doctype html><title>My site</title><p>Keep this content</p>")
	if err := os.WriteFile(index, custom, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeTelemtWebDecoy("other.example.com", dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(index)
	if err != nil || string(got) != string(custom) {
		t.Fatalf("reconfiguration replaced the site: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary page leaked: %v, %v", entries, err)
	}
}

func TestTelemtWebDecoyRejectsInvalidIndex(t *testing.T) {
	for _, kind := range []string{"empty", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			index := filepath.Join(dir, "index.html")
			var err error
			switch kind {
			case "empty":
				err = os.WriteFile(index, nil, 0o644)
			case "directory":
				err = os.Mkdir(index, 0o755)
			case "symlink":
				err = os.Symlink(filepath.Join(dir, "missing.html"), index)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := writeTelemtWebDecoy("example.com", dir); err == nil {
				t.Fatal("accepted an invalid site index")
			}
		})
	}
}

func TestTelemtWebSiteSnapshotsPreserveActiveSite(t *testing.T) {
	root := t.TempDir()
	defaultDir, sitesDir := filepath.Join(root, "default"), filepath.Join(root, "sites")
	if _, err := prepareTelemtWebSite("example.com", defaultDir, sitesDir, nil); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(defaultDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	travel := &TelemtWebSiteOptions{Mode: "generate", Theme: "travel", Title: "Travel notebook"}
	first, err := prepareTelemtWebSite("example.com", defaultDir, sitesDir, travel)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := os.ReadFile(filepath.Join(first, "index.html"))
	if !strings.Contains(string(page), "Travel notebook") || !strings.Contains(string(page), "Travel a little lighter") {
		t.Fatal("theme content was not generated")
	}
	second, err := prepareTelemtWebSite("example.com", defaultDir, sitesDir, &TelemtWebSiteOptions{Mode: "generate", Theme: "nature"})
	if err != nil || first == second {
		t.Fatalf("new site reused the active directory: %s, %v", second, err)
	}
	unchanged, _ := os.ReadFile(filepath.Join(first, "index.html"))
	if string(unchanged) != string(page) {
		t.Fatal("preparing a new site overwrote the previous snapshot")
	}
	reused, err := prepareTelemtWebSite("example.com", defaultDir, sitesDir, travel)
	if err != nil || reused != first {
		t.Fatalf("identical site did not reuse its snapshot: %s, %v", reused, err)
	}
	restored, err := prepareTelemtWebSite("example.com", defaultDir, sitesDir, &TelemtWebSiteOptions{Mode: "auto"})
	if err != nil || restored != defaultDir {
		t.Fatalf("automatic mode did not restore the default site: %s, %v", restored, err)
	}
	after, _ := os.ReadFile(filepath.Join(defaultDir, "index.html"))
	if string(after) != string(original) {
		t.Fatal("optional sites replaced the automatic site")
	}
}

func TestTelemtWebSiteValidationAndEscaping(t *testing.T) {
	for _, invalid := range []*TelemtWebSiteOptions{
		{Mode: "download"}, {Mode: "generate", Theme: "../../etc"}, {Mode: "html"},
		{Mode: "html", HTML: "<html>" + strings.Repeat("a", telemtWebSiteMaxBytes)},
		{Mode: "html", HTML: "<html>\x00</html>"},
	} {
		if _, err := normalizeTelemtWebOptions(TelemtWebProxyOptions{Site: invalid}); err == nil {
			t.Fatalf("accepted invalid site: mode=%s theme=%s", invalid.Mode, invalid.Theme)
		}
	}
	site := &TelemtWebSiteOptions{Mode: "generate", Theme: "technology", Title: `<script>alert(1)</script>`, Description: `A & B "C"`}
	page, err := renderTelemtWebSite("example.com", site)
	if err != nil || strings.Contains(page, "<script>") || !strings.Contains(page, "&lt;script&gt;") || !strings.Contains(page, "A &amp; B &#34;C&#34;") {
		t.Fatalf("generated content was not escaped: %v", err)
	}
	imported := "<!doctype html><html><head><title>Imported</title></head><body>Ready</body></html>"
	page, err = renderTelemtWebSite("example.com", &TelemtWebSiteOptions{Mode: "html", HTML: imported})
	if err != nil || page != imported {
		t.Fatalf("import changed the supplied HTML: %v", err)
	}
	if !telemtWebSitesEqual(nil, &TelemtWebSiteOptions{Mode: "auto"}) || telemtWebSitesEqual(nil, site) {
		t.Fatal("site reconciliation does not preserve the default or detect changes")
	}
}
