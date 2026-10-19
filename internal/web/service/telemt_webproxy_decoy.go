package service

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
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
	Theme       string `json:"theme,omitempty" example:"travel"`
	Title       string `json:"title,omitempty" example:"A travel notebook"`
	Description string `json:"description,omitempty" example:"Stories from quiet places."`
	HTML        string `json:"html,omitempty"`
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
	case "generate":
		switch site.Theme {
		case "", "notes", "technology", "travel", "nature":
			return nil
		}
		return errors.New("WEB site: unsupported theme")
	case "html":
		if strings.TrimSpace(site.HTML) == "" || strings.ContainsRune(site.HTML, '\x00') || !telemtWebSiteHTMLDocument.MatchString(site.HTML) {
			return errors.New("WEB site: upload a complete non-empty HTML document")
		}
		return nil
	default:
		return errors.New("WEB site: mode must be auto, generate or html")
	}
}

func renderTelemtWebSite(domain string, site *TelemtWebSiteOptions) (string, error) {
	if err := validateTelemtWebSite(site); err != nil {
		return "", err
	}
	if site != nil && site.Mode == "html" {
		return site.HTML, nil
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
	if site == nil || site.Mode == "" || site.Mode == "auto" {
		return defaultDir, writeTelemtWebDecoy(domain, defaultDir)
	}
	page, err := renderTelemtWebSite(domain, site)
	if err != nil {
		return "", err
	}
	// Content-addressed snapshots leave the active site intact until setup succeeds.
	dir := filepath.Join(sitesDir, fmt.Sprintf("%x", sha256.Sum256([]byte(page))))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	index := filepath.Join(dir, "index.html")
	if data, err := os.ReadFile(index); err == nil {
		if string(data) != page {
			return "", errors.New("WEB site: existing snapshot contents do not match")
		}
		return dir, nil
	}
	tmp, err := os.CreateTemp(dir, ".index-*.html")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.WriteString(page)
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return "", err
	}
	if err := os.Link(tmp.Name(), index); err != nil {
		return "", err
	}
	return dir, nil
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
	_, writeErr := tmp.WriteString(content)
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
