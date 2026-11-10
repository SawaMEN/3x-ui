package domainfront

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// StartDraftHTTP01Probe temporarily binds public TCP 80 to verify an
// unsaved hostname from an independent node. It never issues certificates,
// changes firewall rules, persists configuration or serves the admin panel.
// A pre-existing listener is never taken over.
func StartDraftHTTP01Probe(ctx context.Context, cfg DomainFrontConfig, name string) (string, func(), error) {
	if !cfg.Enabled || cfg.HTTPPort != 80 || net.ParseIP(cfg.Listen) == nil ||
		strings.HasPrefix(name, "*.") || cfg.DNS01Provider != "" {
		return "", nil, errors.New("draft HTTP-01 proof requires a local TCP 80 gateway")
	}
	allowed := false
	for _, d := range cfg.Domains {
		if d.Name == name && d.Enabled && d.Mode != "shared_sni" &&
			(d.TLS == "auto" || d.TLS == "certmagic") {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", nil, errors.New("hostname is not a local automatic TLS draft")
	}
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", nil, errors.New("cannot generate diagnostic proof")
	}
	token := externalHTTP01ProbePrefix + hex.EncodeToString(nonce[:])
	var listen net.ListenConfig
	listener, err := listen.Listen(ctx, "tcp", net.JoinHostPort(cfg.Listen, "80"))
	if err != nil {
		return "", nil, errors.New("TCP 80 is already occupied or unavailable for temporary proof")
	}
	path := "/.well-known/acme-challenge/" + token
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		host := strings.ToLower(strings.TrimSuffix(req.Host, "."))
		if parsed, _, err := net.SplitHostPort(req.Host); err == nil {
			host = strings.ToLower(strings.TrimSuffix(parsed, "."))
		}
		if host != name || req.URL.Path != path ||
			(req.Method != http.MethodGet && req.Method != http.MethodHead) {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set(externalHTTP01ProbeHeader, token)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if req.Method == http.MethodGet {
			_, _ = w.Write([]byte(token))
		}
	})
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 3*time.Second,
		IdleTimeout: 3*time.Second, MaxHeaderBytes: 4<<10,
	}
	var once sync.Once
	cleanup := func() {
		once.Do(func() { _ = server.Close() })
	}
	go func() { _ = server.Serve(listener) }()
	go func() {
		<-ctx.Done()
		cleanup()
	}()
	return token, cleanup, nil
}
