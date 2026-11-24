package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

// A prepared static site's root must be present. An HTTP response from the
// wrong virtual host or a freshly bound frontend is not enough.
func telemtWebSiteReady(state TelemtWebProxyState, status int) bool {
	if state.Site == nil || state.Site.Mode != "origin" {
		return status == http.StatusOK
	}
	return telemtWebHealthyDecoyStatus(status)
}

// A successful status alone may be an unrelated default virtual host or an
// empty response. Compare the actual static document without logging its
// contents. Origin sites retain their existing status-based policy.
func telemtWebVerifySiteResponse(state TelemtWebProxyState, response *http.Response) error {
	if !telemtWebSiteReady(state, response.StatusCode) {
		return fmt.Errorf("returned HTTP %d", response.StatusCode)
	}
	if state.Site != nil && state.Site.Mode == "origin" {
		return nil
	}
	dir := state.DecoyDir
	if dir == "" {
		dir = telemtWebDecoyDir
	}
	expected, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		return fmt.Errorf("read prepared WEB index: %w", err)
	}
	if len(expected) == 0 {
		return fmt.Errorf("prepared WEB index is empty")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(len(expected))+1))
	if err != nil {
		return fmt.Errorf("read WEB index response: %w", err)
	}
	if !bytes.Equal(body, expected) {
		return fmt.Errorf("response does not match the prepared WEB index")
	}
	return nil
}
