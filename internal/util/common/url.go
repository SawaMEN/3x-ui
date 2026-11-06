package common

import (
	"errors"
	"net/url"
	"strings"
)

// EnsureURLScheme prepends https:// to a URL that carries no scheme, so
// subscription apps and browsers don't resolve it relative to the panel's own
// domain (e.g. "t.me/support" turning into "https://panel.example/t.me/support").
// Values with an explicit scheme (https://, tg://, mailto:, tel:) and empty
// strings pass through untouched.
func EnsureURLScheme(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	// Links are included in subscription metadata and consumed by clients.
	// Reject control characters (including CR/LF and embedded tabs) and URL
	// schemes capable of script execution or local file access.
	if strings.IndexFunc(trimmed, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return ""
	}
	if colon := strings.IndexByte(trimmed, ':'); colon > 0 {
		switch strings.ToLower(trimmed[:colon]) {
		case "javascript", "vbscript", "data", "file", "blob":
			return ""
		}
	}
	if strings.HasPrefix(trimmed, "//") {
		return "https:" + trimmed
	}
	scheme, _, explicit := strings.Cut(trimmed, "://")
	lower := strings.ToLower(trimmed)
	if (explicit && !strings.ContainsAny(scheme, "/?#")) ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "tel:") {
		return trimmed
	}
	return "https://" + trimmed
}

// ParseRemoteRoutingURL classifies a routing settings value: one single-line
// absolute HTTPS URL is a remote source (canonicalized); anything else is inline.
func ParseRemoteRoutingURL(raw string) (string, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.ContainsAny(trimmed, "\r\n") {
		return "", false, nil
	}
	if !strings.HasPrefix(strings.ToLower(trimmed), "https://") {
		return "", false, nil
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return "", true, errors.New("must be an absolute HTTPS URL")
	}
	if u.User != nil {
		return "", true, errors.New("must not contain URL credentials")
	}
	u.Scheme = "https"
	u.Fragment = ""
	return u.String(), true, nil
}
