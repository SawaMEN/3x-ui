package domainfront

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var challengeToken = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (r *frontRuntime) externalChallenge(w http.ResponseWriter, req *http.Request) {
	host := req.Host
	if d, ok := r.domain(host); ok {
		host = d.Name
	} else {
		host = strings.ToLower(strings.Split(host, ":")[0])
	}
	roots := r.challengeRoots
	if roots == nil {
		roots = r.backend.ChallengeRoots
	}
	root := roots[host]
	token := strings.TrimPrefix(req.URL.Path, "/.well-known/acme-challenge/")
	if root != "" && token != req.URL.Path {
		if (req.Method != http.MethodGet && req.Method != http.MethodHead) || !challengeToken.MatchString(token) {
			http.NotFound(w, req)
			return
		}
		file, err := os.Open(filepath.Join(root, ".well-known", "acme-challenge", token))
		if err != nil {
			http.NotFound(w, req)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeContent(w, req, token, info.ModTime(), file)
		return
	}
	r.redirectHTTPS(w, req)
}
