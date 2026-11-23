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

// ServePanelCover applies only on the panel's original listener. Gateway
// listeners on other ports keep their independent public routing.
func (m *Manager) ServePanelCover(w http.ResponseWriter, req *http.Request, panel http.Handler) bool {
	m.mu.Lock()
	r := m.runtime
	if r == nil || r.backend.PanelPath == "" || r.backend.PanelPath == "/" || r.backend.PanelHost == "" ||
		strings.HasPrefix(req.URL.Path, "/.well-known/acme-challenge/") {
		m.mu.Unlock()
		return false
	}
	domain, ok := r.domain(req.Host)
	dir := r.backend.DecoyDirectories[domain.Name]
	if !ok || domain.Mode != "sni_only" || dir == "" || !strings.EqualFold(domain.Name, r.backend.PanelHost) {
		m.mu.Unlock()
		return false
	}
	panelPath := r.panelRequestPath(req.URL.Path)
	m.mu.Unlock()

	if panelPath {
		panel.ServeHTTP(w, req)
		return true
	}
	if !isCoverSitePath(req.URL.Path) {
		return false
	}
	// Custom HTML must run in an opaque origin when served alongside the
	// administration UI. It cannot read panel cookies, storage or responses.
	w.Header().Set("Content-Security-Policy", "sandbox allow-scripts; connect-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'")
	staticCoverSite(dir).ServeHTTP(w, req)
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
	backend.CertificateEvent = m.runtime.backend.CertificateEvent
	backend.DNS01Token = m.runtime.backend.DNS01Token
	backend.SharedHTTP, backend.SharedHTTPS = m.runtime.backend.SharedHTTP, m.runtime.backend.SharedHTTPS
	snapshot := &frontRuntime{cfg: m.runtime.cfg, backend: backend}
	handler, err := snapshot.handler()
	if err != nil {
		snapshot.closeProxyPools()
		return err
	}
	oldPools := m.runtime.proxyPools
	m.runtime.routing.Store(&handler)
	m.runtime.proxyPools = snapshot.proxyPools
	m.runtime.backend = backend
	for _, pool := range oldPools {
		pool.CloseIdleConnections()
	}
	return nil
}

// SubscriptionCertificate reuses the running gateway identity for its
// canonical subscription domain on the separate subscription listener.
func (m *Manager) SubscriptionCertificate(hello *tls.ClientHelloInfo, domain string) (*tls.Certificate, error) {
	m.mu.Lock()
	r := m.runtime
	if r == nil || r.cfg.SubscriptionDomain != domain || !strings.EqualFold(strings.TrimSuffix(hello.ServerName, "."), domain) {
		m.mu.Unlock()
		return nil, nil
	}
	get := r.tlsConfig.GetCertificate
	m.mu.Unlock()
	return get(hello)
}
