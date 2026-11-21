package service

import (
	"crypto/tls"
	"net/http"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

type domainSubscriptionListener struct {
	listen  string
	port    int
	secure  bool
	handler http.Handler
}

var domainSubscription struct {
	sync.Mutex
	active *domainSubscriptionListener
}

func registeredDomainSubscriptionHandler(listen string, port int, secure bool) http.Handler {
	domainSubscription.Lock()
	defer domainSubscription.Unlock()
	if active := domainSubscription.active; active != nil && active.listen == listen && active.port == port && active.secure == secure {
		return active.handler
	}
	return nil
}

// RegisterDomainSubscriptionHandler lets the gateway use the subscription
// router directly while keeping the panel router out of public routes.
func RegisterDomainSubscriptionHandler(listen string, port int, secure bool, handler http.Handler) func() {
	active := &domainSubscriptionListener{listen: listen, port: port, secure: secure, handler: handler}
	domainSubscription.Lock()
	domainSubscription.active = active
	domainSubscription.Unlock()
	return func() {
		domainSubscription.Lock()
		defer domainSubscription.Unlock()
		if domainSubscription.active == active {
			domainSubscription.active = nil
		}
	}
}

func DomainFrontHandler(fallback http.Handler, listen string, port int, secure bool) http.Handler {
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
