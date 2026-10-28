package service

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/url"
	"strconv"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const telemtWebSiteMaxBytes = 256 * 1024

var telemtWebSiteHTMLDocument = regexp.MustCompile(`(?i)<html(?:\s|>)`)

type TelemtWebSiteOptions struct {
	Mode        string `json:"mode" example:"auto"`
	Origin      string `json:"origin,omitempty" example:"http://127.0.0.1:8080"`
	Theme       string `json:"theme,omitempty" example:"travel"`
	Title       string `json:"title,omitempty" example:"A travel notebook"`
	Description string `json:"description,omitempty" example:"Stories from quiet places."`
	HTML        string `json:"html,omitempty"`
	Catalog     string `json:"catalog,omitempty" example:"builtin"`
	Template    string `json:"template,omitempty" example:"notes"`
}

func telemtWebSitesEqual(a, b *TelemtWebSiteOptions) bool {
	if a == nil || a.Mode == "" || a.Mode == "auto" {
		a = nil
	}
	if b == nil || b.Mode == "" || b.Mode == "auto" {
		b = nil
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func validateTelemtWebSite(site *TelemtWebSiteOptions) error {
	if site == nil {
		return nil
	}
	if utf8.RuneCountInString(site.Title) > 80 || utf8.RuneCountInString(site.Description) > 300 || len(site.HTML) > telemtWebSiteMaxBytes {
		return errors.New("WEB site: title, description or HTML exceeds the size limit (HTML: 256 KiB)")
	}
	switch site.Mode {
	case "", "auto":
		return nil
	case "origin":
		return validateTelemtWebOrigin(site.Origin)
	case "generate":
		if site.Theme == "" || isTelemtWebTheme(site.Theme) {
			return nil
		}
		return errors.New("WEB site: unsupported theme")
	case "catalog":
		if site.Catalog == "builtin" {
			if isTelemtWebTheme(site.Template) {
				return nil
			}
		}
		if site.Catalog == "mtproxyl" {
			switch site.Template {
			case "stub", "catrunner", "mekorunner":
				return nil
			}
		}
		return errors.New("WEB site: choose a supported catalog and template")
	case "html":
		if strings.TrimSpace(site.HTML) == "" || strings.ContainsRune(site.HTML, '\x00') || !telemtWebSiteHTMLDocument.MatchString(site.HTML) {
			return errors.New("WEB site: upload a complete non-empty HTML document")
		}
		return nil
	default:
		return errors.New("WEB site: mode must be auto, origin, catalog or generate (html is retained for existing sites)")
	}
}

// An HTTP decoy is only allowed to reach a local numeric IP literal: no
// arbitrary remote URL, credentials, redirects, paths or DNS rebinding.
// Telemt http_upstream only accepts an origin with no URL path, not even
// the optional root slash accepted by browsers in operator input.
func telemtWebCanonicalOrigin(raw string) string {
	return strings.TrimSuffix(strings.TrimSpace(raw), "/")
}

func validateTelemtWebOrigin(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("WEB origin must be http://127.0.0.1:PORT (without path, query or credentials)")
	}
	ip := net.ParseIP(u.Hostname())
	port, portErr := strconv.Atoi(u.Port())
	if ip == nil || !ip.IsLoopback() || portErr != nil || port < 1 || port > 65535 {
		return errors.New("WEB origin must be a loopback IP with a valid TCP port")
	}
	return nil
}

func renderTelemtWebSite(domain string, site *TelemtWebSiteOptions) (string, error) {
	if err := validateTelemtWebSite(site); err != nil {
		return "", err
	}
	if site != nil && site.Mode == "catalog" {
		if site.Catalog == "mtproxyl" {
			data, err := telemtWebCatalogFiles.ReadFile("telemt_sites/mtproxyl/" + site.Template + ".html")
			return string(data), err
		}
		return renderTelemtWebSite(domain, &TelemtWebSiteOptions{Mode: "generate", Theme: site.Template})
	}
	if site != nil && site.Mode == "html" {
		return site.HTML, nil
	}
	if site != nil && site.Mode == "generate" {
		if theme, ok := telemtWebThemes[site.Theme]; ok {
			return renderTelemtWebTheme(domain, site, theme)
		}
	}
	page := strings.ReplaceAll(telemtWebDecoyHTML, "{{DOMAIN}}", html.EscapeString(domain))
	if site == nil || site.Mode != "generate" {
		return page, nil
	}
	title, intro, description := "Fieldnotes", "A little space for useful ideas.", "Observations on making things simpler, working with care, and finding inspiration in the everyday."
	var replacements []string
	switch site.Theme {
	case "technology":
		replacements = []string{"Design", "Software", "Leave room to breathe", "Keep software understandable", "A clear page starts with choosing what matters. A little space between ideas makes each one easier to see.", "Small, focused tools make everyday work easier. Clear interfaces and thoughtful defaults help people get things done.", "Everyday", "Digital life", "Notice the small things", "Build better habits", "A walk, an open window, a few lines in a notebook. Good ideas often begin with a moment of attention.", "A quiet workspace and fewer notifications leave more time to learn, create and focus."}
		title, intro, description = "Digital Journal", "Technology with a human perspective.", "Notes on thoughtful software, useful tools and a simpler digital life."
	case "travel":
		replacements = []string{"Design", "Places", "Leave room to breathe", "Explore at your own pace", "A clear page starts with choosing what matters. A little space between ideas makes each one easier to see.", "Take a quiet street, find a local market and leave room for an unexpected stop. A memorable journey does not need a packed schedule.", "Technology", "Packing", "Tools that last", "Travel a little lighter", "The most useful tools fit quietly into a routine. Simplicity, reliability and small improvements go a long way.", "Comfortable shoes, a notebook and the essentials are a good beginning. Bring what you use and leave space for discoveries.", "Everyday", "Walking", "Notice the small things", "Follow a new path", "A walk, an open window, a few lines in a notebook. Good ideas often begin with a moment of attention.", "A familiar town can offer a new view. Follow the river, visit a park or walk a route you have never tried before."}
		title, intro, description = "Slow Journeys", "Take the scenic route.", "A notebook of quiet places, walking routes and small discoveries along the way."
	case "nature":
		replacements = []string{"Design", "Seasons", "Leave room to breathe", "Watch the seasons change", "A clear page starts with choosing what matters. A little space between ideas makes each one easier to see.", "A tree outside the window offers a different view each month. Notice the light, the leaves and the rhythm of the year.", "Technology", "Green spaces", "Tools that last", "Find a patch of green", "The most useful tools fit quietly into a routine. Simplicity, reliability and small improvements go a long way.", "A city park, a small garden or a woodland path can make room for a slower afternoon outdoors.", "Everyday", "Observation", "Notice the small things", "Keep a nature notebook", "A walk, an open window, a few lines in a notebook. Good ideas often begin with a moment of attention.", "Write down the birds you hear, the weather and the first flowers of the season. Small observations add up over time."}
		title, intro, description = "Outside Notes", "Make time for the outdoors.", "Seasonal observations, green spaces and everyday moments in the natural world."
	}
	if len(replacements) > 0 {
		page = strings.NewReplacer(replacements...).Replace(page)
	}
	if strings.TrimSpace(site.Title) != "" {
		title = site.Title
	}
	if strings.TrimSpace(site.Description) != "" {
		description = site.Description
	}
	return strings.NewReplacer("Notes on thoughtful design, technology and everyday life.", html.EscapeString(description), "Fieldnotes is a small collection of thoughts about design, technology and daily life. Take a moment, read a note, and come back whenever you like.", html.EscapeString(description), "Fieldnotes", html.EscapeString(title), "FIELDNOTES", html.EscapeString(title), "A little space for useful ideas.", html.EscapeString(intro), "Observations on making things simpler, working with care, and finding inspiration in the everyday.", html.EscapeString(description)).Replace(page), nil
}

func prepareTelemtWebSite(domain, defaultDir, sitesDir string, site *TelemtWebSiteOptions) (string, error) {
	if err := validateTelemtWebSite(site); err != nil {
		return "", err
	}
	if site != nil && site.Mode == "origin" {
		// Telemt forwards anonymous requests to the local HTTP origin. No
		// generated website files are needed in this mode.
		return defaultDir, nil
	}
	if site == nil || site.Mode == "" || site.Mode == "auto" {
		return defaultDir, writeTelemtWebDecoy(domain, defaultDir)
	}
	page, err := renderTelemtWebSite(domain, site)
	if err != nil {
		return "", err
	}
	files := telemtWebPageFiles(page)
	if site.Mode == "html" {
		files = map[string]string{"index.html": page}
	}
	if site.Mode == "catalog" && site.Catalog == "mtproxyl" {
		license, err := telemtWebCatalogFiles.ReadFile("telemt_sites/mtproxyl/LICENSE.txt")
		if err != nil {
			return "", err
		}
		files["LICENSE.txt"] = string(license)
	}
	data, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	// Each immutable snapshot includes its resources, preserving the active site on failure.
	dir := filepath.Join(sitesDir, fmt.Sprintf("%x", sha256.Sum256(data)))
	if _, err := os.Stat(dir); err == nil {
		for name, want := range files {
			got, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil || string(got) != want {
				return "", errors.New("WEB site: existing snapshot contents do not match")
			}
		}
		return dir, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(sitesDir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(sitesDir, ".site-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dir); err != nil {
		return "", err
	}
	return dir, nil
}

//go:embed telemt_sites/mtproxyl/*.html telemt_sites/mtproxyl/LICENSE.txt
var telemtWebCatalogFiles embed.FS

var (
	telemtWebInlineStyle  = regexp.MustCompile(`(?is)<style\b[^>]*>(.*?)</style>`)
	telemtWebInlineScript = regexp.MustCompile(`(?is)<script>(.*?)</script>`)
	telemtWebRemoteFont   = regexp.MustCompile(`(?i)<link[^>]+href="https://fonts\.(?:googleapis|gstatic)\.com[^>]*>`)
)

func telemtWebPageFiles(page string) map[string]string {
	page = telemtWebRemoteFont.ReplaceAllString(page, "")
	files := map[string]string{}
	var styles, scripts []string
	for _, match := range telemtWebInlineStyle.FindAllStringSubmatch(page, -1) {
		styles = append(styles, match[1])
	}
	for _, match := range telemtWebInlineScript.FindAllStringSubmatch(page, -1) {
		scripts = append(scripts, match[1])
	}
	if len(styles) > 0 {
		files["site.css"] = strings.Join(styles, "\n")
		page = telemtWebInlineStyle.ReplaceAllString(page, "")
		page = strings.Replace(page, "</head>", `<link rel="stylesheet" href="/site.css"></head>`, 1)
	}
	if len(scripts) > 0 {
		files["site.js"] = strings.Join(scripts, ";\n")
		page = telemtWebInlineScript.ReplaceAllString(page, "")
		page = strings.Replace(page, "</body>", `<script src="/site.js"></script></body>`, 1)
	}
	for _, color := range []string{"#fd79a8", "#a855f7"} {
		attr := `style="color:` + color + `"`
		if strings.Contains(page, attr) {
			page = strings.ReplaceAll(page, attr, "")
			files["site.css"] += "\n#combo-sc{color:" + color + "}"
		}
	}
	files["index.html"] = page
	return files
}

//go:embed telemt_web_decoy.html
var telemtWebDecoyHTML string

func writeTelemtWebDecoy(domain, dir string) error {
	if !telemtWebDomainPattern.MatchString(domain) || len(domain) > 253 {
		return errors.New("invalid WEB site domain")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	index := filepath.Join(dir, "index.html")
	if info, err := os.Lstat(index); err == nil {
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("WEB site %s must be a non-empty regular file", index)
		}
		_, err = os.ReadFile(index)
		return err
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".index-*.html")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	content := strings.ReplaceAll(telemtWebDecoyHTML, "{{DOMAIN}}", html.EscapeString(domain))
	files := telemtWebPageFiles(content)
	for name, data := range files {
		if name != "index.html" {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
				_ = tmp.Close()
				return err
			}
		}
	}
	_, writeErr := tmp.WriteString(files["index.html"])
	closeErr := tmp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	// Publish a complete page without overwriting a concurrently installed site.
	if err := os.Link(tmp.Name(), index); err != nil {
		return fmt.Errorf("publish WEB site: %w", err)
	}
	return nil
}
