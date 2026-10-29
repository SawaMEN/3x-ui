package domainfront

import (
	"net/http"
	"net/url"
	"strings"
)

// RootRedirectTarget allows same-domain paths while preventing a redirect loop
// back to the very same origin/root. Ports are compared as part of the origin.
func RootRedirectTarget(req *http.Request, raw string) string {
	if req.URL.Path != "/" || (req.Method != http.MethodGet && req.Method != http.MethodHead) || req.URL.Query().Get("panel") == "1" {
		return ""
	}
	target, err := ValidateRootRedirectURL(raw)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" || target.User != nil {
		return ""
	}
	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	origin, err := url.Parse(scheme + "://" + req.Host)
	if err != nil {
		return ""
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
	if strings.EqualFold(strings.TrimSuffix(origin.Hostname(), "."), strings.TrimSuffix(target.Hostname(), ".")) && origin.Scheme == target.Scheme && port(origin) == port(target) && (target.Path == "" || target.Path == "/") && target.RawQuery == req.URL.RawQuery {
		return ""
	}
	return target.String()
}
