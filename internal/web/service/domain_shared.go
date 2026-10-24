package service

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

var domainListenerHandlers struct {
	sync.Mutex
	entries map[string]http.Handler
}

func domainListenerKey(listen string, port int, secure bool) string {
	return fmt.Sprintf("%s:%d:%t", listen, port, secure)
}

func registeredDomainHandler(listen string, port int, secure bool) http.Handler {
	domainListenerHandlers.Lock()
	defer domainListenerHandlers.Unlock()
	return domainListenerHandlers.entries[domainListenerKey(listen, port, secure)]
}

func DomainFrontHandler(fallback http.Handler, listen string, port int, secure bool) http.Handler {
	domainListenerHandlers.Lock()
	if domainListenerHandlers.entries == nil {
		domainListenerHandlers.entries = map[string]http.Handler{}
	}
	domainListenerHandlers.entries[domainListenerKey(listen, port, secure)] = fallback
	domainListenerHandlers.Unlock()
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		domainFrontState.Lock()
		manager := domainFrontState.manager
		domainFrontState.Unlock()
		if manager != nil && manager.ServeShared(w, req, listen, port, secure) {
			return
		}
		fallback.ServeHTTP(w, req)
	})
}

func DomainFrontTLS(fallback func(*tls.ClientHelloInfo) (*tls.Certificate, error), listen string, port int) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		domainFrontState.Lock()
		manager := domainFrontState.manager
		domainFrontState.Unlock()
		if manager != nil {
			cert, err := manager.SharedCertificate(hello, listen, port)
			if cert != nil || err != nil {
				return cert, err
			}
		}
		return fallback(hello)
	}
}

func domainSharesListener(cfg domainfront.DomainFrontConfig, listen string, port int, tls bool) (bool, bool) {
	if !domainfront.ListenerCovers(listen, cfg.Listen) {
		return false, false
	}
	return !tls && cfg.HTTPPort == port, tls && cfg.HTTPSPort == port
}
