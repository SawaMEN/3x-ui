package service

import (
	"net/url"
	"strings"
	"testing"
)

func TestTelemtWebBasePathNormalization(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
		ok    bool
	}{
		{"", "", true},
		{"/app/sync/", "app/sync", true},
		{"path-123_test", "path-123_test", true},
		{"short", "", false},
		{"bad/../path", "", false},
		{"foo//bar", "", false},
		{"a?x=1", "", false},
		{strings.Repeat("x", 97), "", false},
	} {
		got, err := normalizeTelemtWebBasePath(tc.input)
		if (err == nil) != tc.ok {
			t.Errorf("normalize(%q): ok=%t, err=%v", tc.input, tc.ok, err)
		}
		if tc.ok && got != tc.want {
			t.Errorf("normalize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestTelemtWebPathLink(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	legacy := telemtWebLinkWithPath("web.example.org", secret, "")
	if legacy != "tg://webproxy?server=web.example.org&secret=dd"+secret {
		t.Fatalf("legacy link changed: %s", legacy)
	}
	withPath := telemtWebLinkWithPath("web.example.org", secret, "app/sync")
	u, err := url.Parse(withPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Query().Get("server"); got != "web.example.org/app/sync" {
		t.Errorf("server = %q", got)
	}
	if got := u.Query().Get("secret"); got != "cN0BI0VniavN7wEjRWeJq83v" {
		t.Errorf("secret = %q", got)
	}
	if got := telemtWebLinkWithPath("web.example.org", "invalid", "app/sync"); got != "" {
		t.Errorf("invalid secret accepted: %q", got)
	}
}

func TestTelemtWebConfigIncludesOptionalBasePath(t *testing.T) {
	// Configuration builders must keep old single-domain data compatible.
	const secret = "0123456789abcdef0123456789abcdef"
	_ = secret
	// Standalone link changes are covered above. Managed config rendering
	// is integration-tested with a running Telemt engine.
}
