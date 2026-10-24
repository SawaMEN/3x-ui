package domainfront

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
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
}

type DomainFrontStatus struct {
	Config       DomainFrontConfig        `json:"config"`
	Running      bool                     `json:"running"`
	Error        string                   `json:"error"`
	Certificates []DomainFrontCertificate `json:"certificates"`
}

type Manager struct {
	mu        sync.Mutex
	cachePath string
	runtime   *frontRuntime
	lastError string
}

type frontRuntime struct {
	cfg         DomainFrontConfig
	backend     Backend
	listeners   []net.Listener
	httpServer  *http.Server
	httpsServer *http.Server
	cache       *certmagic.Cache
	magic       *certmagic.Config
	issuer      *certmagic.ACMEIssuer
	manual      map[string]*tls.Certificate
	cancel      context.CancelFunc
	mu          sync.Mutex
	errors      map[string]string
}

func NewManager(cachePath string) *Manager { return &Manager{cachePath: cachePath} }

func (m *Manager) Apply(ctx context.Context, cfg DomainFrontConfig, backend Backend) error {
	if err := cfg.Validate(); err != nil {
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
	if err := os.MkdirAll(m.cachePath, 0o700); err != nil {
		return nil, err
	}
	r := &frontRuntime{cfg: cfg, backend: backend, manual: map[string]*tls.Certificate{}, errors: map[string]string{}}
	for _, d := range cfg.Domains {
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
	plain, err := lc.Listen(ctx, "tcp", net.JoinHostPort(cfg.Listen, strconv.Itoa(cfg.HTTPPort)))
	if err != nil {
		return nil, fmt.Errorf("HTTP listener: %w", err)
	}
	secure, err := lc.Listen(ctx, "tcp", net.JoinHostPort(cfg.Listen, strconv.Itoa(cfg.HTTPSPort)))
	if err != nil {
		_ = plain.Close()
		return nil, fmt.Errorf("HTTPS listener: %w", err)
	}
	r.listeners = []net.Listener{plain, secure}
	logger := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(os.Stderr), zap.WarnLevel))
	r.cache = certmagic.NewCache(certmagic.CacheOptions{Logger: logger, GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) { return r.magic, nil }})
	r.magic = certmagic.New(r.cache, certmagic.Config{Storage: &certmagic.FileStorage{Path: m.cachePath}, Logger: logger, OnEvent: r.event})
	ca := certmagic.LetsEncryptProductionCA
	if cfg.Staging {
		ca = certmagic.LetsEncryptStagingCA
	}
	r.issuer = certmagic.NewACMEIssuer(r.magic, certmagic.ACMEIssuer{CA: ca, Email: cfg.Email, Agreed: cfg.AcceptTerms, AltHTTPPort: cfg.HTTPPort, AltTLSALPNPort: cfg.HTTPSPort, Logger: logger})
	r.magic.Issuers = []certmagic.Issuer{r.issuer}
	tlsCfg := r.magic.TLSConfig()
	tlsCfg.MinVersion = tls.VersionTLS12
	tlsCfg.NextProtos = append([]string{"h2", "http/1.1"}, tlsCfg.NextProtos...)
	tlsCfg.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		d, ok := r.domain(hello.ServerName)
		if !ok {
			return nil, fmt.Errorf("unknown TLS domain")
		}
		if d.TLS == "manual" {
			return r.manual[d.Name], nil
		}
		return r.magic.GetCertificate(hello)
	}
	handler, err := r.handler()
	if err != nil {
		_ = plain.Close()
		_ = secure.Close()
		r.cache.Stop()
		return nil, err
	}
	r.httpServer = &http.Server{Handler: r.issuer.HTTPChallengeHandler(http.HandlerFunc(r.redirectHTTPS)), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
	r.httpsServer = &http.Server{Handler: handler, TLSConfig: tlsCfg, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
	r.httpsServer.Protocols = new(http.Protocols)
	r.httpsServer.Protocols.SetHTTP1(true)
	r.httpsServer.Protocols.SetHTTP2(true)

	runCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go func() { _ = network.ServeHTTP(r.httpServer, plain, "Domain HTTP server") }()
	go func() { _ = network.ServeHTTP(r.httpsServer, tls.NewListener(secure, tlsCfg), "Domain HTTPS server") }()
	var names []string
	for _, d := range cfg.Domains {
		if d.Enabled && d.TLS == "auto" {
			names = append(names, d.Name)
		}
	}
	if err := r.magic.ManageAsync(runCtx, names); err != nil {
		r.close()
		return nil, err
	}
	return r, nil
}

func (r *frontRuntime) domain(host string) (DomainFrontDomain, bool) {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	for _, d := range r.cfg.Domains {
		if d.Enabled && d.Name == host {
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
	host := d.Name
	if r.cfg.PublicPort != 443 {
		host = net.JoinHostPort(host, strconv.Itoa(r.cfg.PublicPort))
	}
	target := &url.URL{Scheme: "https", Host: host, Path: req.URL.Path, RawPath: req.URL.RawPath, RawQuery: req.URL.RawQuery}
	http.Redirect(w, req, target.String(), http.StatusPermanentRedirect)
}

func (r *frontRuntime) event(_ context.Context, event string, data map[string]any) error {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	out := DomainFrontStatus{Config: cfg, Running: m.runtime != nil, Error: m.lastError, Certificates: []DomainFrontCertificate{}}
	for _, d := range cfg.Domains {
		entry := DomainFrontCertificate{Name: d.Name, Mode: d.TLS, State: "disabled", AutoRenew: out.Running && d.Enabled && cfg.Enabled && d.TLS == "auto"}
		if !cfg.Enabled || !d.Enabled {
			out.Certificates = append(out.Certificates, entry)
			continue
		}
		entry.State = "pending"
		var raw []byte
		var err error
		if d.TLS == "manual" {
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
			raw, err = storage.Load(context.Background(), certmagic.StorageKeys.SiteCert(issuer.IssuerKey(), d.Name))
			if m.runtime != nil {
				m.runtime.mu.Lock()
				entry.Error = m.runtime.errors[d.Name]
				m.runtime.mu.Unlock()
			}
		}
		if err != nil && d.TLS == "manual" {
			entry.Error = err.Error()
		}

		if err == nil && len(raw) > 0 {
			block, _ := pem.Decode(raw)
			if block == nil {
				entry.Error = "invalid certificate PEM"
			}
			if block != nil {
				if cert, parseErr := x509.ParseCertificate(block.Bytes); parseErr == nil {
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
		if !d.Enabled {
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

func (r *frontRuntime) domainHandler(d DomainFrontDomain) (http.Handler, error) {
	subTarget := r.backend.SubscriptionTarget
	subHost := d.Name
	if d.Mode == "relay" {
		subTarget = d.Origin
		subHost = ""
	}
	subscription, err := proxy(subTarget, subHost, false)
	if err != nil {
		return nil, err
	}
	subscription.ModifyResponse = func(response *http.Response) error {
		if err := proxyTrailers(response); err != nil {
			return err
		}
		response.Header.Set("Cache-Control", "private, no-store, max-age=0")
		response.Header.Set("CDN-Cache-Control", "no-store")
		response.Header.Set("Cloudflare-CDN-Cache-Control", "no-store")
		return nil
	}
	var panel http.Handler
	if d.Panel && d.Mode != "sub_link_only" {
		target, host := r.backend.PanelTarget, r.backend.PanelHost
		if d.Mode == "relay" {
			target = d.Origin
			host = ""
		}
		panel, err = proxy(target, host, false)
		if err != nil {
			return nil, err
		}
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
		if req.URL.Path == "/" && r.backend.RedirectURL != "" && req.URL.Query().Get("panel") != "1" && (req.Method == http.MethodGet || req.Method == http.MethodHead) {
			u, err := url.Parse(r.backend.RedirectURL)
			if err == nil && !strings.EqualFold(u.Hostname(), d.Name) {
				w.Header().Set("Referrer-Policy", "no-referrer")
				http.Redirect(w, req, u.String(), http.StatusFound)
				return
			}
		}
		if panel != nil && (pathWithin(req.URL.Path, r.backend.PanelPath) || req.URL.Path == "/") {
			panel.ServeHTTP(w, req)
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
