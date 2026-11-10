package domainfront

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/certmagic"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/SawaMEN/3x-ui/v3/internal/web/network"
)

type DomainFrontCertificate struct {
	Name      string `json:"name"`
	Mode      string `json:"mode"`
	State     string `json:"state"`
	AutoRenew bool   `json:"autoRenew"`
	Issuer    string `json:"issuer"`
	ExpiresAt int64  `json:"expiresAt"`
	DaysLeft  int    `json:"daysLeft"`
	Error     string `json:"error"`
	Source    string `json:"source"`
	Renewal   string `json:"renewal"`
	CertFile  string `json:"certFile"`
	KeyFile   string `json:"keyFile"`
}

type DomainFrontStatus struct {
	Config               DomainFrontConfig        `json:"config"`
	Running              bool                     `json:"running"`
	Error                string                   `json:"error"`
	Certificates         []DomainFrontCertificate `json:"certificates"`
	ExistingCertificates []DomainFrontCertificate `json:"existingCertificates,omitempty"`
}

type Manager struct {
	mu        sync.Mutex
	cachePath string
	runtime   *frontRuntime
	lastError string
}

type frontRuntime struct {
	cfg            DomainFrontConfig
	backend        Backend
	challengeRoots map[string]string
	listeners      []net.Listener
	httpServer     *http.Server
	httpsServer    *http.Server
	cache          *certmagic.Cache
	magic          *certmagic.Config
	issuer         *certmagic.ACMEIssuer
	manual         map[string]*tls.Certificate
	external       map[string]*network.FileCertificate
	cancel         context.CancelFunc
	mu             sync.Mutex
	errors         map[string]string
	proofs         map[string]http01Proof
	handlerHTTPS   http.Handler
	routing        atomic.Pointer[http.Handler]
	handlerHTTP    http.Handler
	tlsConfig      *tls.Config
}

func NewManager(cachePath string) *Manager { return &Manager{cachePath: cachePath} }

func (m *Manager) Apply(ctx context.Context, cfg DomainFrontConfig, backend Backend) error {
	if err := cfg.ValidateWithCertificates(backend.Certificates); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.runtime
	m.runtime = nil
	if old != nil {
		old.close()
	}
	next, err := m.start(ctx, cfg, backend)
	if err != nil {
		m.lastError = err.Error()
		if old != nil {
			restored, restoreErr := m.start(context.Background(), old.cfg, old.backend)
			m.runtime = restored
			if restoreErr != nil {
				m.lastError += "; rollback: " + restoreErr.Error()
			}
		}
		return errors.New(m.lastError)
	}
	m.runtime = next
	m.lastError = ""
	return nil
}

// CheckManagedRenewal checks one local CertMagic certificate against the
// issuer's renewal policy. It never forces a renewal and never issues for a
// foreign, disabled or externally managed name. Caller bounds the context.
func (m *Manager) CheckManagedRenewal(ctx context.Context, name string) error {
	m.mu.Lock()
	r := m.runtime
	if r == nil || r.magic == nil {
		m.mu.Unlock()
		return errors.New("domain gateway is not running")
	}
	managed := false
	for _, domain := range r.cfg.Domains {
		if !domain.Enabled || domain.Name != name || domain.Mode == "shared_sni" {
			continue
		}
		if (domain.TLS == "auto" || domain.TLS == "certmagic") && r.external[name] == nil {
			managed = true
		}
		break
	}
	magic := r.magic
	m.mu.Unlock()
	if !managed {
		return errors.New("certificate is not managed locally by CertMagic")
	}
	// false means never force ACME issuance for a healthy certificate.
	return magic.RenewCertSync(ctx, name, false)
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runtime != nil {
		m.runtime.close()
		m.runtime = nil
	}
}

func (r *frontRuntime) close() {
	r.cancel()
	for _, listener := range r.listeners {
		_ = listener.Close()
	}
	r.httpServer.SetKeepAlivesEnabled(false)
	r.httpsServer.SetKeepAlivesEnabled(false)
	r.cache.Stop()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := r.httpServer.Shutdown(ctx); err != nil {
			_ = r.httpServer.Close()
		}
		if err := r.httpsServer.Shutdown(ctx); err != nil {
			_ = r.httpsServer.Close()
		}
	}()
}

func (m *Manager) start(ctx context.Context, cfg DomainFrontConfig, backend Backend) (*frontRuntime, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	// A collection of foreign SNI references does not need a local listener,
	// ACME account or open public ports on the controlling panel.
	hasLocalDomain := false
	for _, domain := range cfg.Domains {
		if domain.Enabled && domain.Mode != "shared_sni" {
			hasLocalDomain = true
			break
		}
	}
	if !hasLocalDomain {
		return nil, nil
	}
	if err := os.MkdirAll(m.cachePath, 0o700); err != nil {
		return nil, err
	}
	r := &frontRuntime{cfg: cfg, backend: backend, challengeRoots: backend.ChallengeRoots, manual: map[string]*tls.Certificate{}, external: map[string]*network.FileCertificate{}, errors: map[string]string{}}
	if r.challengeRoots == nil {
		r.challengeRoots = map[string]string{}
	}
	for _, d := range cfg.Domains {
		if d.Mode == "shared_sni" {
			continue
		}
		if files, ok := backend.Certificates[d.Name]; ok && d.Enabled && (d.TLS == "auto" || d.TLS == "existing") {
			cert, err := network.NewFileCertificate(files.CertFile, files.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", d.Name, err)
			}
			pair, _ := cert.GetCertificate(nil)
			leaf, err := x509.ParseCertificate(pair.Certificate[0])
			if err != nil {
				return nil, err
			}
			if err := leaf.VerifyHostname(d.Name); err != nil {
				return nil, err
			}
			r.external[d.Name] = cert
		}
		if !d.Enabled || d.TLS != "manual" {
			continue
		}
		pair, err := tls.LoadX509KeyPair(d.CertFile, d.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", d.Name, err)
		}
		pair.Leaf, err = x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return nil, err
		}
		if err := pair.Leaf.VerifyHostname(d.Name); err != nil {
			return nil, fmt.Errorf("%s: %w", d.Name, err)
		}
		r.manual[d.Name] = &pair
	}
	var lc net.ListenConfig
	var plain, secure net.Listener
	var err error
	if !backend.SharedHTTP {
		plain, err = lc.Listen(ctx, "tcp", net.JoinHostPort(cfg.Listen, strconv.Itoa(cfg.HTTPPort)))
		if err != nil {
			return nil, fmt.Errorf("HTTP listener: %w", err)
		}
		r.listeners = append(r.listeners, plain)
	}
	if !backend.SharedHTTPS {
		secure, err = lc.Listen(ctx, "tcp", net.JoinHostPort(cfg.Listen, strconv.Itoa(cfg.HTTPSPort)))
		if err != nil {
			for _, listener := range r.listeners {
				_ = listener.Close()
			}
			return nil, fmt.Errorf("HTTPS listener: %w", err)
		}
		r.listeners = append(r.listeners, secure)
	}
	logger := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(os.Stderr), zap.WarnLevel))
	r.cache = certmagic.NewCache(certmagic.CacheOptions{Logger: logger, GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) { return r.magic, nil }})
	r.magic = certmagic.New(r.cache, certmagic.Config{Storage: &certmagic.FileStorage{Path: m.cachePath}, Logger: logger, OnEvent: r.event})
	ca := certmagic.LetsEncryptProductionCA
	if cfg.Staging {
		ca = certmagic.LetsEncryptStagingCA
	}
	issuerOptions := certmagic.ACMEIssuer{CA: ca, Email: cfg.Email, Agreed: cfg.AcceptTerms, AltHTTPPort: cfg.HTTPPort, AltTLSALPNPort: cfg.HTTPSPort, Logger: logger}
	if cfg.DNS01Provider == "cloudflare" {
		provider, err := NewCloudflareTXTProvider(cfg.DNS01Zone, cfg.DNS01ZoneID, backend.DNS01Token)
		if err != nil {
			r.cache.Stop()
			for _, listener := range r.listeners {
				_ = listener.Close()
			}
			return nil, fmt.Errorf("Cloudflare DNS-01 is not configured: %w", err)
		}
		// All automatic certificates use DNS-01 when explicitly configured.
		// HTTP/TLS-ALPN challenges are disabled, so ACME never needs to open
		// temporary firewall ports for these certificates.
		issuerOptions.DisableHTTPChallenge = true
		issuerOptions.DisableTLSALPNChallenge = true
		issuerOptions.DNS01Solver = &certmagic.DNS01Solver{DNSManager: certmagic.DNSManager{
			DNSProvider: provider,
			PropagationDelay: time.Duration(cfg.DNS01Propagation) * time.Second,
			PropagationTimeout: 4 * time.Minute,
		}}
	}
	r.issuer = certmagic.NewACMEIssuer(r.magic, issuerOptions)
	r.magic.Issuers = []certmagic.Issuer{r.issuer}
	// ManageAsync also loads saved certificates asynchronously. Warm the cache
	// before serving HTTPS so the first handshake after a restart does not race
	// that loader. New certificates are still obtained in the background.
	var names []string
	for _, d := range cfg.Domains {
		if !d.Enabled || d.Mode == "shared_sni" || (d.TLS != "auto" && d.TLS != "certmagic") || r.external[d.Name] != nil {
			continue
		}
		names = append(names, d.Name)
		if _, err := r.magic.CacheManagedCertificate(ctx, d.Name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			for _, listener := range r.listeners {
				_ = listener.Close()
			}
			r.cache.Stop()
			return nil, fmt.Errorf("%s: cached certificate: %w", d.Name, err)
		}
	}
	tlsCfg := r.magic.TLSConfig()
	tlsCfg.MinVersion = tls.VersionTLS12
	tlsCfg.NextProtos = append([]string{"h2", "http/1.1"}, tlsCfg.NextProtos...)
	tlsCfg.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		d, ok := r.domain(hello.ServerName)
		if !ok {
			return nil, fmt.Errorf("unknown TLS domain")
		}
		if cert := r.external[d.Name]; cert != nil {
			return cert.GetCertificate(hello)
		}
		if d.TLS == "manual" {
			return r.manual[d.Name], nil
		}
		return r.magic.GetCertificate(hello)
	}
	snapshot := &frontRuntime{cfg: cfg, backend: backend}
	handler, err := snapshot.handler()
	if err != nil {
		for _, listener := range r.listeners {
			_ = listener.Close()
		}
		r.cache.Stop()
		return nil, err
	}
	r.routing.Store(&handler)
	r.handlerHTTPS = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { (*r.routing.Load()).ServeHTTP(w, req) })
	r.tlsConfig = tlsCfg
	r.handlerHTTP = r.issuer.HTTPChallengeHandler(http.HandlerFunc(r.externalChallenge))
	r.httpServer = &http.Server{Handler: r.handlerHTTP, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
	r.httpsServer = &http.Server{Handler: r.handlerHTTPS, TLSConfig: tlsCfg, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
	r.httpsServer.Protocols = new(http.Protocols)
	r.httpsServer.Protocols.SetHTTP1(true)
	r.httpsServer.Protocols.SetHTTP2(true)

	runCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	if plain != nil {
		go func() { _ = network.ServeHTTP(r.httpServer, plain, "Domain HTTP server") }()
	}
	if secure != nil {
		go func() { _ = network.ServeHTTP(r.httpsServer, tls.NewListener(secure, tlsCfg), "Domain HTTPS server") }()
	}
	if err := r.magic.ManageAsync(runCtx, names); err != nil {
		r.close()
		return nil, err
	}
	// Preloaded certificates are already managed, so ManageAsync skips them.
	// Check renewal now as well as on CertMagic's normal maintenance interval.
	go func() {
		if err := r.cache.RenewManagedCertificates(runCtx); err != nil && runCtx.Err() == nil {
			logger.Warn("checking cached certificate renewal", zap.Error(err))
		}
	}()
	return r, nil
}

func (r *frontRuntime) domain(host string) (DomainFrontDomain, bool) {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	// Exact hosts take priority over a wildcard, even if the wildcard was
	// listed first. Wildcards match exactly one DNS label, never the apex.
	for _, d := range r.cfg.Domains {
		if d.Enabled && d.Mode != "shared_sni" && d.Name == host {
			return d, true
		}
	}
	for _, d := range r.cfg.Domains {
		if !d.Enabled || d.Mode == "shared_sni" || !strings.HasPrefix(d.Name, "*.") {
			continue
		}
		suffix := strings.TrimPrefix(d.Name, "*.")
		if !strings.HasSuffix(host, "."+suffix) {
			continue
		}
		label := strings.TrimSuffix(host, "."+suffix)
		if label != "" && !strings.Contains(label, ".") {
			return d, true
		}
	}
	return DomainFrontDomain{}, false
}

func (r *frontRuntime) redirectHTTPS(w http.ResponseWriter, req *http.Request) {
	d, ok := r.domain(req.Host)
	if !ok {
		http.NotFound(w, req)
		return
	}
	redirectURL := r.backend.RedirectURL
	if r.backend.GetRedirect != nil {
		redirectURL, _ = r.backend.GetRedirect()
	}
	if d.Mode != "sni_only" && !r.protectedRedirectPath(d, req.URL.Path) {
		if target := RootRedirectTarget(req, redirectURL); target != "" {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			http.Redirect(w, req, target, http.StatusFound)
			return
		}
	}
	host := d.Name
	if strings.HasPrefix(d.Name, "*.") {
		host = req.Host
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		host = strings.TrimSuffix(strings.ToLower(host), ".")
	}
	if r.cfg.PublicPort != 443 {
		host = net.JoinHostPort(host, strconv.Itoa(r.cfg.PublicPort))
	}
	target := &url.URL{Scheme: "https", Host: host, Path: req.URL.Path, RawPath: req.URL.RawPath, RawQuery: req.URL.RawQuery}
	http.Redirect(w, req, target.String(), http.StatusPermanentRedirect)
}

func (r *frontRuntime) event(ctx context.Context, event string, data map[string]any) error {
	if r.backend.CertificateEvent != nil {
		if err := r.backend.CertificateEvent(ctx, event, data); err != nil {
			return err
		}
	}
	name, _ := data["identifier"].(string)
	if name == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if event == "cert_failed" {
		r.errors[name] = fmt.Sprint(data["error"])
	}
	if event == "cert_obtained" || event == "cert_renewed" {
		delete(r.errors, name)
	}
	return nil
}

func (m *Manager) Status(cfg DomainFrontConfig) DomainFrontStatus {
	return m.StatusWithCertificates(cfg, nil)
}

func (m *Manager) StatusWithCertificates(cfg DomainFrontConfig, files map[string]CertificateFiles) DomainFrontStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := DomainFrontStatus{Config: cfg, Running: m.runtime != nil, Error: m.lastError, Certificates: []DomainFrontCertificate{}}
	for _, d := range cfg.Domains {
		entry := DomainFrontCertificate{Name: d.Name, Mode: d.TLS, State: "disabled", AutoRenew: out.Running && d.Enabled && cfg.Enabled && (d.TLS == "auto" || d.TLS == "certmagic")}
		if !d.Enabled {
			out.Certificates = append(out.Certificates, entry)
			continue
		}
		if d.Mode == "shared_sni" {
			entry.State, entry.Source, entry.Renewal, entry.AutoRenew = "external", "remote", "external", false
			out.Certificates = append(out.Certificates, entry)
			continue
		}
		entry.State = "pending"
		entry.Source, entry.Renewal = "gateway", "certmagic"
		if d.TLS == "manual" {
			entry.Source, entry.Renewal = "manual", "manual"
			entry.CertFile, entry.KeyFile = d.CertFile, d.KeyFile
		}
		external, inherited := files[d.Name]
		if m.runtime != nil {
			if active, ok := m.runtime.backend.Certificates[d.Name]; ok {
				external, inherited = active, true
			}
		}
		if inherited && (d.TLS == "auto" || d.TLS == "existing") {
			entry.Source, entry.Renewal, entry.CertFile, entry.KeyFile = external.Source, external.Renewal, external.CertFile, external.KeyFile
			entry.AutoRenew, entry.Error = external.AutoRenew, external.Warning
		}
		var raw []byte
		var err error
		if inherited && (d.TLS == "auto" || d.TLS == "existing") {
			if m.runtime != nil && m.runtime.external[d.Name] != nil {
				pair, _ := m.runtime.external[d.Name].GetCertificate(nil)
				raw = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]})
			} else {
				raw, err = os.ReadFile(external.CertFile)
			}
		} else if d.TLS == "manual" {
			if m.runtime != nil && m.runtime.manual[d.Name] != nil {
				raw = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: m.runtime.manual[d.Name].Certificate[0]})
			} else {
				raw, err = os.ReadFile(d.CertFile)
			}
		} else {
			ca := certmagic.LetsEncryptProductionCA
			if cfg.Staging {
				ca = certmagic.LetsEncryptStagingCA
			}
			issuer := &certmagic.ACMEIssuer{CA: ca}
			storage := &certmagic.FileStorage{Path: m.cachePath}
			certStorageKey := certmagic.StorageKeys.SiteCert(issuer.IssuerKey(), d.Name)
			keyStorageKey := certmagic.StorageKeys.SitePrivateKey(issuer.IssuerKey(), d.Name)
			raw, err = storage.Load(context.Background(), certStorageKey)
			if err == nil {
				if _, keyErr := storage.Load(context.Background(), keyStorageKey); keyErr == nil {
					entry.CertFile = filepath.Join(m.cachePath, filepath.FromSlash(certStorageKey))
					entry.KeyFile = filepath.Join(m.cachePath, filepath.FromSlash(keyStorageKey))
				}
			}
			if m.runtime != nil {
				m.runtime.mu.Lock()
				entry.Error = m.runtime.errors[d.Name]
				m.runtime.mu.Unlock()
			}
		}
		if err != nil && (d.TLS == "manual" || inherited) {
			entry.Error = err.Error()
		}

		if err == nil && len(raw) > 0 {
			block, _ := pem.Decode(raw)
			if block == nil {
				entry.Error = "invalid certificate PEM"
			}
			if block != nil {
				if cert, parseErr := x509.ParseCertificate(block.Bytes); parseErr == nil {
					if strings.Contains(d.Name, ".") {
						if verifyErr := cert.VerifyHostname(d.Name); verifyErr != nil {
							entry.Error = "certificate does not match configured domain"
							entry.State = "error"
							out.Certificates = append(out.Certificates, entry)
							continue
						}
					}
					entry.ExpiresAt = cert.NotAfter.Unix()
					entry.DaysLeft = int(math.Ceil(time.Until(cert.NotAfter).Hours() / 24))
					entry.Issuer = cert.Issuer.CommonName
					entry.State = "valid"
					if entry.DaysLeft <= 30 {
						entry.State = "renewal_due"
					}
					if !time.Now().Before(cert.NotAfter) {
						entry.State = "expired"
					}
				}
			}
		}
		if entry.Error != "" && entry.State == "pending" {
			entry.State = "error"
		}
		if !out.Running && entry.State == "pending" {
			entry.State = "stopped"
		}
		out.Certificates = append(out.Certificates, entry)
	}
	return out
}

func proxy(target, host string, h2c bool) (*httputil.ReverseProxy, error) {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid domain upstream")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ForceAttemptHTTP2 = true
	if u.Scheme == "https" && host != "" {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: strings.TrimSuffix(host, ".")}
	}
	if h2c {
		transport.Protocols = new(http.Protocols)
		transport.Protocols.SetUnencryptedHTTP2(true)
	}

	return &httputil.ReverseProxy{Transport: transport, FlushInterval: -1, ModifyResponse: proxyTrailers, Rewrite: func(p *httputil.ProxyRequest) {
		p.SetURL(u)
		p.SetXForwarded()
		if host != "" {
			p.Out.Host = host
		}
	}, ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}}, nil
}

func (r *frontRuntime) handler() (http.Handler, error) {
	handlers := map[string]http.Handler{}
	for _, d := range r.cfg.Domains {
		if !d.Enabled || d.Mode == "shared_sni" {
			continue
		}
		h, err := r.domainHandler(d)
		if err != nil {
			return nil, err
		}
		handlers[d.Name] = h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		d, ok := r.domain(req.Host)
		if !ok {
			http.NotFound(w, req)
			return
		}
		handlers[d.Name].ServeHTTP(w, req)
	}), nil
}

func pathWithin(path, prefix string) bool {
	prefix = strings.TrimRight(prefix, "/")
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// panelRequestPath protects panel URLs from publication by the public gateway.
// Even legacy domains with Panel=true must never forward these endpoints.
func (r *frontRuntime) panelRequestPath(path string) bool {
	// Reserve panel URLs on every domain, including subscription-only hosts.
	if base := r.backend.PanelPath; base != "" && base != "/" {
		return pathWithin(path, base)
	}
	for _, prefix := range []string{
		"/panel", "/login", "/logout", "/csrf-token", "/getTwoFactorEnable",
		"/ws", "/bus", "/api", "/assets", "/manifest.webmanifest",
		"/pwa-register.js", "/service-worker.js", "/icons",
	} {
		if pathWithin(path, prefix) {
			return true
		}
	}
	return false
}

// Protected paths never reach an external decoy redirect, on either HTTP
// (where they must upgrade to HTTPS) or HTTPS (where they are dispatched).
func (r *frontRuntime) protectedRedirectPath(d DomainFrontDomain, path string) bool {
	if strings.HasPrefix(path, "/.well-known/acme-challenge/") {
		return true
	}
	for _, route := range r.backend.Routes[d.Name] {
		if pathWithin(path, route.Path) {
			return true
		}
	}
	for _, prefix := range r.backend.SubscriptionPaths {
		if prefix != "" && pathWithin(path, prefix) {
			return true
		}
	}
	for _, prefix := range r.backend.ProtectedPaths {
		if prefix != "" && pathWithin(path, prefix) {
			return true
		}
	}
	return r.panelRequestPath(path)
}

func (r *frontRuntime) domainHandler(d DomainFrontDomain) (http.Handler, error) {
	// SNI-only hosts serve Telemt's generated static website while denying
	// every panel, subscription and inbound route, even if its path exists.
	if d.Mode == "sni_only" {
		dir := r.backend.DecoyDirectories[d.Name]
		if dir == "" {
			return nil, fmt.Errorf("SNI site %s has no prepared website", d.Name)
		}
		static := http.FileServer(http.Dir(dir))
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Robots-Tag", "noindex, nofollow")
			w.Header().Set("Cache-Control", "public, max-age=300")
			if req.Method != http.MethodGet && req.Method != http.MethodHead {
				http.NotFound(w, req)
				return
			}
			switch req.URL.Path {
			case "/", "/index.html", "/site.css", "/site.js":
				static.ServeHTTP(w, req)
			default:
				http.NotFound(w, req)
			}
		}), nil
	}
	subTarget := r.backend.SubscriptionTarget
	subHost := d.Name
	if r.backend.SubscriptionHost != "" {
		subHost = r.backend.SubscriptionHost
	}
	if d.Mode == "relay" {
		subTarget = d.Origin
		subHost = ""
	}
	subscriptionProxy, err := proxy(subTarget, subHost, false)
	if err != nil {
		return nil, err
	}
	subscriptionProxy.ModifyResponse = func(response *http.Response) error {
		if err := proxyTrailers(response); err != nil {
			return err
		}
		response.Header.Set("Cache-Control", "private, no-store, max-age=0")
		response.Header.Set("CDN-Cache-Control", "no-store")
		response.Header.Set("Cloudflare-CDN-Cache-Control", "no-store")
		return nil
	}
	var subscription http.Handler = subscriptionProxy
	if r.backend.SubscriptionHandler != nil && d.Mode != "relay" {
		subscription = localHandler(r.backend.SubscriptionHandler, d.Name, true)
	}
	type routeHandler struct {
		path    string
		handler http.Handler
	}
	var routes []routeHandler
	for _, route := range r.backend.Routes[d.Name] {
		if d.Mode == "sub_link_only" {
			break
		}
		target, host := route.Target, route.Host
		if d.Mode == "relay" {
			target = d.Origin
			host = ""
		}
		handler, err := proxy(target, host, route.H2C && d.Mode != "relay")
		if err != nil {
			return nil, err
		}
		routes = append(routes, routeHandler{route.Path, handler})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		for _, route := range routes {
			if pathWithin(req.URL.Path, route.path) {
				route.handler.ServeHTTP(w, req)
				return
			}
		}
		for _, prefix := range r.backend.SubscriptionPaths {
			if pathWithin(req.URL.Path, prefix) {
				w.Header().Set("Cache-Control", "private, no-store, max-age=0")
				w.Header().Set("CDN-Cache-Control", "no-store")
				subscription.ServeHTTP(w, req)
				return
			}
		}
		if r.protectedRedirectPath(d, req.URL.Path) {
			// An unmatched legacy/application endpoint must never fall
			// through to the public decoy. Its former behavior was 404.
			http.NotFound(w, req)
			return
		}
		redirectURL := r.backend.RedirectURL
		if r.backend.GetRedirect != nil {
			redirectURL, _ = r.backend.GetRedirect()
		}
		if target := RootRedirectTarget(req, redirectURL); target != "" {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			http.Redirect(w, req, target, http.StatusFound)
			return
		}
		http.NotFound(w, req)
	}), nil
}

// HTTP/1.1 trailers require chunked framing even when the HTTP/2 upstream knows its length.
func proxyTrailers(response *http.Response) error {
	if len(response.Trailer) > 0 {
		response.Header.Del("Content-Length")
		response.ContentLength = -1
	}
	return nil
}
