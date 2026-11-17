package service

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
)

func prepareVetroff(ib *model.Inbound, previous string) error {
	var settings externalvpn.Settings
	if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
		return err
	}
	if settings.Vetroff == nil {
		v := externalvpn.DefaultVetroffSettings()
		v.AutoTLS = settings.Certificate == "" && settings.PrivateKey == ""
		settings.Vetroff = &v
	}
	if err := resolveVetroffTLS(ib, &settings); err != nil {
		return err
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	// Keep the full panel client records; runtime credentials are a smaller view.
	var original map[string]json.RawMessage
	if err := json.Unmarshal([]byte(ib.Settings), &original); err != nil {
		return err
	}
	if original == nil {
		original = map[string]json.RawMessage{}
	}
	var prepared map[string]json.RawMessage
	if err := json.Unmarshal(raw, &prepared); err != nil {
		return err
	}
	for _, key := range []string{"hostname", "certificate", "privateKey", "vetroff"} {
		original[key] = prepared[key]
	}
	raw, err = json.Marshal(original)
	if err != nil {
		return err
	}
	ib.Settings = string(raw)
	return externalvpn.PrepareAdditional(ib, previous)
}

func resolveVetroffTLS(ib *model.Inbound, settings *externalvpn.Settings) error {
	if !settings.Vetroff.AutoTLS {
		if settings.Hostname == "" {
			settings.Hostname = certificateHost(settings.Certificate, "")
		}
		return nil
	}
	svc := &SettingService{}
	all, err := svc.GetAllSetting()
	if err != nil {
		return err
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(settings.Hostname)), ".")
	if host == "" {
		candidate := strings.Trim(ib.ShareAddr, "[]")
		if net.ParseIP(candidate) == nil && !strings.ContainsAny(candidate, "/: ") {
			host = candidate
		}
	}
	type tlsCandidate struct {
		domainfront.CertificateFiles
		hostname string
	}
	candidates := []tlsCandidate{
		{domainfront.CertificateFiles{CertFile: all.WebCertFile, KeyFile: all.WebKeyFile}, all.WebDomain},
		{domainfront.CertificateFiles{CertFile: all.SubCertFile, KeyFile: all.SubKeyFile}, all.SubDomain},
	}
	if cfg, err := svc.GetDomainFrontConfig(); err == nil {
		files, _, err := svc.domainCertificateFiles(cfg)
		if err == nil {
			for _, domain := range cfg.Domains {
				if domain.Enabled {
					if f, ok := files[domain.Name]; ok {
						candidates = append(candidates, tlsCandidate{f, domain.Name})
					}
				}
			}
		}
	}
	for _, candidate := range candidates {
		if candidate.CertFile == "" || candidate.KeyFile == "" {
			continue
		}
		pair, err := tls.LoadX509KeyPair(candidate.CertFile, candidate.KeyFile)
		if err != nil || len(pair.Certificate) == 0 {
			continue
		}
		cert, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil || time.Now().Before(cert.NotBefore) || !time.Now().Before(cert.NotAfter) {
			continue
		}
		name := host
		if name == "" {
			name = candidate.hostname
		}
		if name == "" {
			for _, dns := range cert.DNSNames {
				if !strings.Contains(dns, "*") {
					name = dns
					break
				}
			}
		}
		if name == "" || cert.VerifyHostname(name) != nil {
			continue
		}
		settings.Hostname, settings.Certificate, settings.PrivateKey = name, candidate.CertFile, candidate.KeyFile
		return nil
	}
	return fmt.Errorf("VetroffProxy: no valid matching panel/domain TLS certificate; configure HTTPS or select custom certificate paths")
}
