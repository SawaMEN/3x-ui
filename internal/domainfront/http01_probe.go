package domainfront

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
)

// A proof is deliberately ephemeral: it never becomes an ACME token and
// cannot expose the panel. Only a running, explicitly configured hostname
// may receive a challenge; wildcard names require DNS-01 instead.
const externalHTTP01ProbePrefix = "3xui-reach-"
const externalHTTP01ProbeHeader = "X-3xui-Reachability"

type http01Proof struct {
	name    string
	expires time.Time
}

func (m *Manager) StartHTTP01Probe(name string) (string, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.runtime
	if r == nil || r.httpServer == nil || r.cfg.HTTPPort != 80 {
		return "", nil, errors.New("HTTP-01 gateway must already be listening on public TCP 80")
	}
	allowed := false
	for _, d := range r.cfg.Domains {
		if d.Enabled && d.Name == name && d.Mode != "shared_sni" && !strings.HasPrefix(d.Name, "*.") {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", nil, errors.New("HTTP-01 host must be an active local gateway domain")
	}
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", nil, errors.New("could not generate temporary HTTP-01 proof")
	}
	token := externalHTTP01ProbePrefix + hex.EncodeToString(random[:])
	r.mu.Lock()
	if r.proofs == nil {
		r.proofs = make(map[string]http01Proof)
	}
	for key, proof := range r.proofs {
		if time.Now().After(proof.expires) {
			delete(r.proofs, key)
		}
	}
	r.proofs[token] = http01Proof{name: name, expires: time.Now().Add(20 * time.Second)}
	r.mu.Unlock()
	return token, func() {
		r.mu.Lock()
		delete(r.proofs, token)
		r.mu.Unlock()
	}, nil
}

// serveHTTP01Proof returns true when the path belongs to this diagnostic.
// Unknown tokens are always handled as 404, never redirected or served as
// another site's file. The public token grants no other access.
func (r *frontRuntime) serveHTTP01Proof(w http.ResponseWriter, req *http.Request) bool {
	const prefix = "/.well-known/acme-challenge/" + externalHTTP01ProbePrefix
	if !strings.HasPrefix(req.URL.Path, prefix) {
		return false
	}
	token := strings.TrimPrefix(req.URL.Path, "/.well-known/acme-challenge/")
	r.mu.Lock()
	proof, ok := r.proofs[token]
	r.mu.Unlock()
	d, allowed := r.domain(req.Host)
	if !ok || !allowed || d.Name != proof.name || !time.Now().Before(proof.expires) ||
		(req.Method != http.MethodGet && req.Method != http.MethodHead) {
		http.NotFound(w, req)
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set(externalHTTP01ProbeHeader, token)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(token))
	return true
}
