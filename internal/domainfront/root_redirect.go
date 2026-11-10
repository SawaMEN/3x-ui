package domainfront

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// RootRedirectTarget builds the public decoy URL for a browser request.
// The caller must route panel, subscription and VPN transport paths before
// calling this function. Only GET/HEAD requests may be redirected.
func RootRedirectTarget(req *http.Request, raw string) string {
	if req == nil || req.URL == nil ||
		(req.Method != http.MethodGet && req.Method != http.MethodHead) ||
		req.URL.Query().Get("panel") == "1" ||
		strings.HasPrefix(req.URL.Path, "/.well-known/acme-challenge/") {
		return ""
	}

	target, err := ValidateRootRedirectURL(raw)
	if err != nil {
		return ""
	}
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	origin, err := url.Parse(scheme + "://" + req.Host)
	if err != nil || origin.Hostname() == "" {
		return ""
	}

	// Explicit destination ports take priority. On an alternate HTTP(S)
	// listener, keep the incoming port so /path at :2345 maps to :2345/path.
	defaultPort := "80"
	if scheme == "https" {
		defaultPort = "443"
	}
	if incomingPort := origin.Port(); target.Port() == "" && incomingPort != "" && incomingPort != defaultPort {
		target.Host = net.JoinHostPort(target.Hostname(), incomingPort)
	}

	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	sameOrigin := strings.EqualFold(strings.TrimSuffix(origin.Hostname(), "."), strings.TrimSuffix(target.Hostname(), ".")) &&
		origin.Scheme == target.Scheme && port(origin) == port(target)
	basePath := target.Path

	// Same-origin destinations with a path are allowed, but don't
	// repeatedly prepend /site to /site/anything.
	if sameOrigin && basePath != "" && basePath != "/" && pathWithin(req.URL.Path, basePath) {
		return ""
	}

	if req.URL.Path != "/" {
		// Join escaped paths rather than cleaning them: changing %2F or
		// trailing slashes would alter the URL the user requested.
		escaped := strings.TrimSuffix(target.EscapedPath(), "/") + req.URL.EscapedPath()
		decoded, err := url.PathUnescape(escaped)
		if err != nil {
			return ""
		}
		target.Path, target.RawPath = decoded, escaped
		// Root requests retain the previous privacy behavior: their query
		// string is not forwarded. Non-root decoy URLs retain theirs.
		if req.URL.RawQuery != "" {
			if target.RawQuery != "" {
				target.RawQuery += "&"
			}
			target.RawQuery += req.URL.RawQuery
		}
	}

	if sameOrigin {
		if target.Path == req.URL.Path && target.RawQuery == req.URL.RawQuery {
			return ""
		}
		// A same-origin root destination would otherwise keep redirecting
		// non-root URLs, including by accumulating query parameters.
		if req.URL.Path != "/" && (basePath == "" || basePath == "/") && target.Query().Get("panel") != "1" {
			return ""
		}
	}
	return target.String()
}
