package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// TLSCertificateInstall carries a certificate and its matching private key
// only inside a verified node-to-node HTTPS request. Never log or return the
// contents to the browser.
type TLSCertificateInstall struct {
	Domain         string `json:"domain"`
	CertificatePEM string `json:"certificatePem"`
	PrivateKeyPEM  string `json:"privateKeyPem"`
}

type InstalledTLSCertificate struct {
	Domain    string `json:"domain"`
	CertFile  string `json:"certFile"`
	KeyFile   string `json:"keyFile"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (r *Remote) InstallTLSCertificate(ctx context.Context, payload TLSCertificateInstall) (InstalledTLSCertificate, error) {
	var installed InstalledTLSCertificate
	if r.node == nil || r.node.Scheme != "https" || r.node.TlsVerifyMode == "skip" {
		return installed, errors.New("certificate distribution requires a verified HTTPS node connection (verify, pin or mTLS)")
	}
	env, err := r.do(ctx, http.MethodPost, "panel/api/domains/certificates/install", payload)
	if err != nil {
		return installed, err
	}
	if err := json.Unmarshal(env.Obj, &installed); err != nil {
		return InstalledTLSCertificate{}, errors.New("invalid node certificate installation response")
	}
	if installed.Domain != payload.Domain || installed.CertFile == "" || installed.KeyFile == "" {
		return InstalledTLSCertificate{}, errors.New("node returned incomplete certificate paths")
	}
	return installed, nil
}

type TLSCertificateLookup struct {
    Domain string `json:"domain"`
}

// ResolveLocalTLSCertificate asks the selected node to resolve its own files.
// Only paths for its locally validated, public TLS identity are returned.
func (r *Remote) ResolveLocalTLSCertificate(ctx context.Context, domain string) (InstalledTLSCertificate, error) {
    env, err := r.do(ctx, http.MethodPost, "panel/api/domains/certificates/resolve-local",
        TLSCertificateLookup{Domain: domain})
    if err != nil {
        return InstalledTLSCertificate{}, err
    }
    var result InstalledTLSCertificate
    if err := json.Unmarshal(env.Obj, &result); err != nil {
        return InstalledTLSCertificate{}, errors.New("invalid node TLS certificate lookup response")
    }
    if result.Domain != domain || result.CertFile == "" || result.KeyFile == "" {
        return InstalledTLSCertificate{}, errors.New("node returned an incomplete TLS certificate reference")
    }
    return result, nil
}
