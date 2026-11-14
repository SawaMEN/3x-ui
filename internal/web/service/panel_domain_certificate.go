package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

// ResolvePanelDomainCertificate only reuses a saved, local direct domain.
// It reads the pair without persisting settings, issuing, or restarting.
func ResolvePanelDomainCertificate(ctx context.Context, domain string) (DistributedTLSCertificate, error) {
	if err := ctx.Err(); err != nil {
		return DistributedTLSCertificate{}, err
	}
	status, err := DomainFrontStatus()
	if err != nil {
		return DistributedTLSCertificate{}, err
	}
	return panelDomainCertificate(status, domain, time.Now())
}

func panelDomainCertificate(status domainfront.DomainFrontStatus, domain string, now time.Time) (DistributedTLSCertificate, error) {
	empty := DistributedTLSCertificate{}
	name, err := distributionDomain(domain)
	if err != nil || !status.Config.Enabled || !status.Running {
		return empty, errors.New("select an enabled direct domain on the running local gateway")
	}
	var selected *domainfront.DomainFrontDomain
	for i := range status.Config.Domains {
		d := &status.Config.Domains[i]
		if d.Name == name && d.Enabled && d.Mode == "direct" {
			selected = d
			break
		}
	}
	if selected == nil {
		return empty, errors.New("panel certificate source must be a saved local direct domain")
	}
	if status.Config.Staging && (selected.TLS == "auto" || selected.TLS == "certmagic") {
		return empty, errors.New("a staging certificate cannot be selected for the panel")
	}
	for _, cert := range status.Certificates {
		if cert.Name != name || !filepath.IsAbs(cert.CertFile) || !filepath.IsAbs(cert.KeyFile) {
			continue
		}
		pair, err := tls.LoadX509KeyPair(cert.CertFile, cert.KeyFile)
		if err != nil || len(pair.Certificate) == 0 {
			continue
		}
		leaf, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil || leaf.VerifyHostname(name) != nil || now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
			continue
		}
		return DistributedTLSCertificate{Domain: name, CertFile: cert.CertFile, KeyFile: cert.KeyFile, ExpiresAt: leaf.NotAfter.Unix()}, nil
	}
	return empty, errors.New("the direct domain does not have a valid matching certificate and private key yet")
}

// Sharing the gateway's managed pair with the panel must not make that same
// pair an external input to its issuer, which would turn off ACME renewal.
func isDomainManagedCertificatePair(certFile, keyFile string) bool {
	root, err := filepath.Abs(filepath.Join(config.GetDBFolderPath(), "domain-tls"))
	if err != nil {
		return false
	}
	for _, path := range []string{certFile, keyFile} {
		if !filepath.IsAbs(path) {
			return false
		}
		rel, err := filepath.Rel(root, filepath.Clean(path))
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return false
		}
	}
	return true
}

func validatePanelCertificateSource(old, next domainfront.DomainFrontConfig, domain, certFile, keyFile string) error {
	if !isDomainManagedCertificatePair(certFile, keyFile) {
		return nil
	}
	owned := false
	for _, d := range old.Domains {
		if d.Enabled && d.Name == domain {
			owned = true
		}
	}
	if !owned {
		return nil
	}
	if next.Staging != old.Staging {
		return errors.New("pages.settings.domains.panelDomain.sourceRequired")
	}
	for _, d := range next.Domains {
		if next.Enabled && d.Enabled && d.Name == domain && d.Mode != "shared_sni" && (d.TLS == "auto" || d.TLS == "certmagic") {
			return nil
		}
	}
	return errors.New("pages.settings.domains.panelDomain.sourceRequired")
}
