package service

import (
	"net/http"
	"strings"
)

// A WEB decoy is an ordinary user-controlled HTTP site. 301/302 redirects,
// authorization pages, missing homepages, and HEAD-not-allowed must not be
// mistaken for a failed Telemt WEB listener.
func telemtWebHealthyDecoyStatus(status int) bool {
	if status >= 200 && status < 400 {
		return true
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed:
		return true
	default:
		return false
	}
}

// Probe the WEB mount, not an unrelated site's root or subscription route.
func telemtWebReadinessPath(basePath string) string {
	if path := strings.Trim(basePath, "/"); path != "" {
		return "/" + path + "/"
	}
	return "/"
}
