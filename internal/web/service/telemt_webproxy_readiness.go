package service

import "net/http"

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

// A prepared static site's root must be present. An HTTP response from the
// wrong virtual host or a freshly bound frontend is not enough.
func telemtWebSiteReady(state TelemtWebProxyState, status int) bool {
	if state.Site == nil || state.Site.Mode != "origin" {
		return status >= 200 && status < 300
	}
	return telemtWebHealthyDecoyStatus(status)
}
