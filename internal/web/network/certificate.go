package network

import (
	"crypto/tls"
	"sync"
)

// FileCertificate keeps the last usable pair while an external renewer replaces files.
type FileCertificate struct {
	mu                sync.Mutex
	certFile, keyFile string
	pair              *tls.Certificate
}

func NewFileCertificate(certFile, keyFile string) (*FileCertificate, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &FileCertificate{certFile: certFile, keyFile: keyFile, pair: &pair}, nil
}

func (c *FileCertificate) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pair, err := tls.LoadX509KeyPair(c.certFile, c.keyFile); err == nil {
		c.pair = &pair
	}
	return c.pair, nil
}
