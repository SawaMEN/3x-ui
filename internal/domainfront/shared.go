package domainfront

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"time"
)

func ListenerCovers(listener, desired string) bool {
	if listener == "" || listener == "::" {
		return true
	}
	if desired == "" || desired == "::" {
		return false
	}
	a, b := net.ParseIP(listener), net.ParseIP(desired)
	if a == nil || b == nil {
		return false
	}
	if listener == "0.0.0.0" {
		return b.To4() != nil
	}
	return a.Equal(b)
}

func (m *Manager) ServeShared(w http.ResponseWriter, req *http.Request, listen string, port int, secure bool) bool {
	m.mu.Lock()
	r := m.runtime
	var handler http.Handler
	if r != nil && ListenerCovers(listen, r.cfg.Listen) {
		if secure && r.backend.SharedHTTPS && r.cfg.HTTPSPort == port {
			handler = r.handlerHTTPS
		}
		if !secure && r.backend.SharedHTTP && r.cfg.HTTPPort == port {
			handler = r.handlerHTTP
		}
		_, known := r.domain(req.Host)
		host := strings.ToLower(strings.Split(req.Host, ":")[0])
		challenge := strings.HasPrefix(req.URL.Path, "/.well-known/acme-challenge/") && r.backend.ChallengeRoots[host] != ""
		if !known && !challenge {
			handler = nil
		}
	}
	m.mu.Unlock()
	if handler == nil {
		return false
	}
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Time{})
	_ = controller.SetWriteDeadline(time.Time{})
	handler.ServeHTTP(w, req)
	return true
}

func (m *Manager) SharedCertificate(hello *tls.ClientHelloInfo, listen string, port int) (*tls.Certificate, error) {
	m.mu.Lock()
	r := m.runtime
	if r == nil || !r.backend.SharedHTTPS || r.cfg.HTTPSPort != port || !ListenerCovers(listen, r.cfg.Listen) {
		m.mu.Unlock()
		return nil, nil
	}
	if _, known := r.domain(hello.ServerName); !known {
		m.mu.Unlock()
		return nil, nil
	}
	get := r.tlsConfig.GetCertificate
	m.mu.Unlock()
	return get(hello)
}

func (m *Manager) RefreshRoutes(backend Backend) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runtime == nil {
		return nil
	}
	backend.Certificates = m.runtime.backend.Certificates
	backend.ChallengeRoots = m.runtime.backend.ChallengeRoots
	backend.SharedHTTP, backend.SharedHTTPS = m.runtime.backend.SharedHTTP, m.runtime.backend.SharedHTTPS
	snapshot := &frontRuntime{cfg: m.runtime.cfg, backend: backend}
	handler, err := snapshot.handler()
	if err != nil {
		return err
	}
	m.runtime.routing.Store(&handler)
	m.runtime.backend = backend
	return nil
}
