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

func TestTelemtWebAliases(t *testing.T) {
	got, err := normalizeTelemtWebAliases("web.example.org", []string{"B.example.org", "a.example.org"})
	if err != nil || strings.Join(got, ",") != "a.example.org,b.example.org" {
		t.Fatalf("aliases = %v, err = %v", got, err)
	}
	for _, tc := range [][]string{
		{"web.example.org"},
		{"a.example.org", "a.example.org"},
		{"invalid localhost"},
		{"a.example.org", "b.example.org", "c.example.org", "d.example.org", "e.example.org", "f.example.org", "g.example.org", "h.example.org"},
	} {
		if _, err := normalizeTelemtWebAliases("web.example.org", tc); err == nil {
			t.Errorf("invalid aliases accepted: %v", tc)
		}
	}
}

func TestTelemtWebOriginSecurity(t *testing.T) {
	if err := validateTelemtWebSite(&TelemtWebSiteOptions{Mode: "origin", Origin: "http://127.0.0.1:8080"}); err != nil {
		t.Fatal(err)
	}
	if err := validateTelemtWebSite(&TelemtWebSiteOptions{Mode: "origin", Origin: "http://127.0.0.1:8080/"}); err != nil {
		t.Fatalf("trailing slash on local origin should be supported: %v", err)
	}
	if got := telemtWebCanonicalOrigin("http://127.0.0.1:8080/"); got != "http://127.0.0.1:8080" {
		t.Errorf("origin was not normalized: %q", got)
	}
	if err := validateTelemtWebSite(&TelemtWebSiteOptions{Mode: "origin", Origin: "http://[::1]:18081"}); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{
		"http://localhost:8080",
		"http://example.com:8080",
		"http://127.0.0.1:8080/private",
		"http://127.0.0.1:8080?secret=a",
		"http://user:pass@127.0.0.1:8080",
		"https://127.0.0.1:8080",
		"http://127.0.0.1:0",
	} {
		if err := validateTelemtWebSite(&TelemtWebSiteOptions{Mode: "origin", Origin: origin}); err == nil {
			t.Errorf("unsafe local HTTP origin accepted: %s", origin)
		}
	}
}

func TestTelemtWebFrontendAliases(t *testing.T) {
	for _, frontend := range []string{"caddy", "haproxy"} {
		state := TelemtWebProxyState{
			Domain: "web.example.org", Aliases: []string{"alt.example.org"},
			Frontend: frontend, ListenPort: 15080, PublicAddr: "203.0.113.9:443",
		}
		data, err := telemtWebFrontendConfig(state, telemtWebSubscriptionTarget{})
		if err != nil {
			t.Fatalf("%s config: %v", frontend, err)
		}
		if !strings.Contains(string(data), "web.example.org") || !strings.Contains(string(data), "alt.example.org") {
			t.Fatalf("%s did not include both WEB hostnames", frontend)
		}
	}
}

func TestTelemtWebProfileCapacityGrowth(t *testing.T) {
	for _, tc := range []struct {
		users, hosts, want int
	}{
		{0, 1, 64},
		{48, 1, 64},
		{49, 1, 128},
		{50, 1, 128},
		{112, 1, 128},
		{113, 1, 256},
		{1, 8, 256},
		{100, 8, 1024},
	} {
		got := telemtWebProfileCapacity(tc.users, tc.hosts)
		if got != tc.want {
			t.Errorf("users=%d hosts=%d: capacity %d, want %d", tc.users, tc.hosts, got, tc.want)
		}
		if got < tc.users*tc.hosts {
			t.Errorf("users=%d hosts=%d: capacity too low", tc.users, tc.hosts)
		}
	}
}
