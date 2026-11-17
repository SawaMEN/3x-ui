package uripath

import (
	"fmt"
	"net/url"
	"strings"
)

// Normalize keeps router prefixes and session cookie paths identical.
func Normalize(value string) (string, error) {
	for _, r := range value {
		if strings.ContainsRune(`\ :*?#%;`, r) || r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("URI path contains an invalid character")
		}
	}
	parts := make([]string, 0)
	for _, part := range strings.Split(value, "/") {
		if part == "." || part == ".." {
			return "", fmt.Errorf("URI path cannot contain . or .. segments")
		}
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "/", nil
	}
	return "/" + strings.Join(parts, "/") + "/", nil
}

// CookiePath uses the browser's escaped request path for Unicode prefixes.
func CookiePath(value string) string { return (&url.URL{Path: value}).EscapedPath() }
