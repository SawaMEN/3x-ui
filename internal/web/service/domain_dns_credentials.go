package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

const (
	cloudflareDNSCredentialEnv  = "X_UI_CLOUDFLARE_DNS_TOKEN"
	cloudflareDNSCredentialFile = "domain-cloudflare-dns-token"
)

var domainDNSCredentialMu sync.Mutex

type DomainDNSCredentialStatus struct {
	Configured bool   `json:"configured"`
	Source     string `json:"source,omitempty"`
}

func validDomainDNSToken(value string) bool {
	if len(value) < 20 || len(value) > 512 || strings.TrimSpace(value) != value {
		return false
	}
	for _, ch := range value {
		if ch < '!' || ch > '~' || ch == '"' || ch == '\\' {
			return false
		}
	}
	return true
}

func domainDNSCredentialTarget(provider string) (string, string, error) {
	switch provider {
	case "cloudflare":
		return cloudflareDNSCredentialFile, cloudflareDNSCredentialEnv, nil
	case "digitalocean":
		return "domain-digitalocean-dns-token", "X_UI_DIGITALOCEAN_DNS_TOKEN", nil
	default:
		return "", "", errors.New("unsupported DNS-01 provider")
	}
}

func readDomainDNSCredentialFor(provider string) (string, string, error) {
	filename, environment, err := domainDNSCredentialTarget(provider)
	if err != nil {
		return "", "", err
	}
	domainDNSCredentialMu.Lock()
	defer domainDNSCredentialMu.Unlock()
	if token := strings.TrimSpace(os.Getenv(environment)); token != "" {
		if !validDomainDNSToken(token) {
			return "", "", errors.New("invalid Cloudflare DNS-01 environment credential")
		}
		return token, "environment", nil
	}
	path := filepath.Join(config.GetDBFolderPath(), filename)
	stat, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", "", nil
	}
	if err != nil {
		return "", "", errors.New("cannot inspect DNS credential file")
	}
	// Never follow symlinks or read credentials accessible to other users.
	if !stat.Mode().IsRegular() || stat.Mode().Perm()&0o077 != 0 || stat.Size() > 520 {
		return "", "", errors.New("DNS credential file must be a private regular file (0600)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", errors.New("cannot read DNS credential")
	}
	token := strings.TrimSpace(string(raw))
	if !validDomainDNSToken(token) {
		return "", "", errors.New("invalid stored DNS credential")
	}
	return token, "file", nil
}

func DomainDNSCredentialState() (DomainDNSCredentialStatus, error) {
	return DomainDNSCredentialStateFor("cloudflare")
}

func DomainDNSCredentialStateFor(provider string) (DomainDNSCredentialStatus, error) {
	token, source, err := readDomainDNSCredentialFor(provider)
	return DomainDNSCredentialStatus{Configured: token != "", Source: source}, err
}

// SaveDomainDNSCredential stores the token outside the database and API
// responses. Its owner-only file survives restarts. A systemd environment
// credential takes precedence over the saved file when configured.
func SaveDomainDNSCredential(token string, clear bool) error {
	return SaveDomainDNSCredentialFor("cloudflare", token, clear)
}

func SaveDomainDNSCredentialFor(provider, token string, clear bool) error {
	filename, _, err := domainDNSCredentialTarget(provider)
	if err != nil {
		return err
	}
	domainDNSCredentialMu.Lock()
	defer domainDNSCredentialMu.Unlock()
	path := filepath.Join(config.GetDBFolderPath(), filename)
	if clear {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("cannot remove DNS credential")
		}
		return nil
	}
	if !validDomainDNSToken(token) {
		return errors.New("Cloudflare DNS token must be 20–512 printable characters without whitespace")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("cannot prepare DNS credential directory")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".domain-dns-token-*")
	if err != nil {
		return errors.New("cannot create private DNS credential file")
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return errors.New("cannot restrict DNS credential permissions")
	}
	if _, err := f.WriteString(token + "\n"); err != nil {
		_ = f.Close()
		return errors.New("cannot save DNS credential")
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return errors.New("cannot sync DNS credential")
	}
	if err := f.Close(); err != nil {
		return errors.New("cannot close DNS credential file")
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return errors.New("cannot activate DNS credential")
	}
	return nil
}

func configureDomainDNS01(cfg domainfront.DomainFrontConfig, backend *domainfront.Backend) error {
	if cfg.DNS01Provider == "" || !cfg.UsesManagedCertificates(backend.Certificates) {
		return nil
	}
	if _, _, err := domainDNSCredentialTarget(cfg.DNS01Provider); err != nil {
		return err
	}
	token, _, err := readDomainDNSCredentialFor(cfg.DNS01Provider)
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("%s DNS-01 requires a scoped token; configure it in Domains and TLS settings", cfg.DNS01Provider)
	}
	backend.DNS01Token = token
	return nil
}
