package network

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
	"time"
)

// FileCertificate keeps the last usable pair while an external renewer replaces files.
type FileCertificate struct {
	mu                sync.Mutex
	certFile, keyFile string
	hostname          string
	pair              *tls.Certificate
}

func NewFileCertificate(certFile, keyFile string) (*FileCertificate, error) {
	return NewNamedFileCertificate(certFile, keyFile, "")
}

// NewNamedFileCertificate checks that initial and renewed pairs cover hostname.
func NewNamedFileCertificate(certFile, keyFile, hostname string) (*FileCertificate, error) {
	c := &FileCertificate{certFile: certFile, keyFile: keyFile, hostname: hostname}
	pair, err := c.load()
	if err != nil {
		return nil, err
	}
	c.pair = pair
	return c, nil
}

func (c *FileCertificate) load() (*tls.Certificate, error) {
	pair, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
	if err != nil {
		return nil, err
	}
	if c.hostname != "" {
		if pair.Leaf == nil {
			pair.Leaf, err = x509.ParseCertificate(pair.Certificate[0])
			if err != nil {
				return nil, err
			}
		}
		if err := pair.Leaf.VerifyHostname(c.hostname); err != nil {
			return nil, err
		}
		now := time.Now()
		if now.Before(pair.Leaf.NotBefore) || !now.Before(pair.Leaf.NotAfter) {
			return nil, fmt.Errorf("TLS certificate for %q is outside its validity period", c.hostname)
		}
	}
	return &pair, nil
}

func (c *FileCertificate) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pair, err := c.load(); err == nil {
		c.pair = pair
	}
	return c.pair, nil
}
