package entity

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/util/uripath"
)

func TestPathHasForbiddenChar(t *testing.T) {
	valid := []string{
		"",
		"/",
		"/sub/",
		"/json/",
		"/a/b/c/",
		"/My-Path_123/",
	}
	for _, p := range valid {
		if _, err := uripath.Normalize(p); err != nil {
			t.Errorf("pathHasForbiddenChar(%q) = true, want false", p)
		}
	}

	invalid := []string{
		"/:param/", "/catch*all/", "/secret?query/", "/secret#fragment/", "/encoded%2F/",
		"/sub path/",
		"/back\\slash/",
		"/tab\there/",
		"/new\nline/",
		"/\x7f/",
	}
	for _, p := range invalid {
		if _, err := uripath.Normalize(p); err == nil {
			t.Errorf("pathHasForbiddenChar(%q) = false, want true", p)
		}
	}
}

func TestPanelPathsAndDomainValidation(t *testing.T) {
	s := &AllSetting{WebPort: 2053, SubPort: 2096, WebBasePath: "//secret///child", SubPath: "//sub//", WebDomain: "Panel.Example.COM."}
	if err := s.CheckValid(); err != nil {
		t.Fatal(err)
	}
	if s.WebBasePath != "/secret/child/" || s.SubPath != "/sub/" || s.WebDomain != "panel.example.com" {
		t.Fatalf("not normalized: %q %q %q", s.WebBasePath, s.SubPath, s.WebDomain)
	}
	for _, value := range []string{"/a/../", "/./"} {
		s.WebBasePath = value
		if err := s.CheckValid(); err == nil {
			t.Fatalf("accepted path %q", value)
		}
	}
	s.WebBasePath = "/safe/"
	for _, value := range []string{"https://panel.example.com", "panel.example.com:443", "panel.example.com/path"} {
		s.WebDomain = value
		if err := s.CheckValid(); err == nil {
			t.Fatalf("accepted domain %q", value)
		}
	}
}
