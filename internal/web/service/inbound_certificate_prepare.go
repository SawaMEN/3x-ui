package service

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"errors"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

type InboundCertificateRequest struct {
	NodeID      int    `json:"nodeId"`
	Domain      string `json:"domain"`
	CoverSite   bool   `json:"coverSite,omitempty"`
	RSAOnly     bool   `json:"rsaOnly,omitempty"`
	Email       string `json:"email,omitempty"`
	AcceptTerms *bool  `json:"acceptTerms,omitempty"`
}

type InboundCertificatePreparation struct {
	NodeID       int    `json:"nodeId"`
	SourceNodeID int    `json:"sourceNodeId,omitempty"`
	Domain       string `json:"domain"`
	State        string `json:"state"`
	CertFile     string `json:"certFile,omitempty"`
	KeyFile      string `json:"keyFile,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
}

func distributedPayloadSupportsRSA(payload runtime.TLSCertificateInstall) bool {
	pair, err := tls.X509KeyPair([]byte(payload.CertificatePEM), []byte(payload.PrivateKeyPEM))
	if err != nil {
		return false
	}
	_, ok := pair.PrivateKey.(*rsa.PrivateKey)
	return ok
}

// This status lookup is read-only, including when the SNI is still being typed.
// Only paths belonging to the selected node can appear in its response.
func InboundCertificateStatus(ctx context.Context, req InboundCertificateRequest) (InboundCertificatePreparation, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	name, err := distributionDomain(req.Domain)
	result := InboundCertificatePreparation{NodeID: req.NodeID, Domain: name, State: "not_requested"}
	if err != nil || req.NodeID < 0 {
		return result, errors.New("select a valid node and DNS hostname")
	}
	if req.NodeID > 0 {
		if _, err := verifiedCertificateNode(req.NodeID); err != nil {
			return result, err
		}
	}
	rows, err := CoverCertificateFleets()
	if err != nil {
		return result, err
	}
	enrolled := false
	// An unacknowledged runtime apply must not become 'valid' merely because
	// the file pair is already present on disk. The worker will retry applying.
	for _, row := range rows {
		if row.Domain != name {
			continue
		}
		for _, dst := range row.Targets {
			if dst.NodeID != req.NodeID {
				continue
			}
			enrolled = true
			if dst.State != "ready" {
				result.State = dst.State
				return result, nil
			}
		}
	}
	if cert, err := ResolveTLSCertificate(ctx, req.NodeID, name, req.RSAOnly); err == nil {
		result.State, result.CertFile, result.KeyFile, result.ExpiresAt = "valid", cert.CertFile, cert.KeyFile, cert.ExpiresAt
		return result, nil
	}
	if enrolled {
		result.State = "retrying"
		return result, nil
	}
	if _, _, err := localManagedCertificatePayload(ctx, name, req.RSAOnly); err == nil {
		result.State = "available"
	}
	if result.State == "not_requested" {
		if source, ok := readyCoverCertificateSource(ctx, name, req.NodeID, req.RSAOnly); ok {
			result.State = "available"
			result.SourceNodeID = source
		}
	}

	if result.State == "not_requested" && !req.RSAOnly {
		current, err := DomainFrontStatus()
		if err != nil {
			return result, err
		}
		if current.Running {
			for _, d := range current.Config.Domains {
				if d.Name == name && d.Enabled && d.Mode != "shared_sni" && (d.TLS == "auto" || d.TLS == "certmagic") {
					if req.NodeID == 0 {
						result.State = "pending"
					} else {
						result.State = "available"
					}
				}
			}
		}
	}
	return result, nil
}

// Only a certificate belonging to an enabled owned website is offered as a
// source. A panel certificate with the same hostname is not that site's TLS.
func readyCoverCertificateSource(ctx context.Context, name string, target int, rsaOnly bool) (int, bool) {
	report, err := CoverCertificateSources(ctx)
	if err != nil {
		return 0, false
	}
	for _, site := range report.Sites {
		if site.Domain != name || site.NodeID == target || site.NodeID <= 0 {
			continue
		}
		source, err := verifiedCertificateNode(site.NodeID)
		if err != nil {
			continue
		}
		certs, err := source.ListCertificateSummaries(ctx)
		if err != nil {
			continue
		}
		for _, cert := range certs {
			if cert.Name != name || (!cert.CoverSite && cert.Manager != "certmagic" && site.SiteCatalog != "telemt") || cert.ExpiresAt <= time.Now().Unix() || (cert.State != "valid" && cert.State != "renewal_due") {
				continue
			}
			if _, err := source.ResolveLocalTLSCertificate(ctx, name, rsaOnly); err == nil {
				return site.NodeID, true
			}
		}
	}
	return 0, false
}

// Enroll additively: preparing Finland must not remove USA or other recipients.
// The same persistent worker delivers both DNS-01 and owned-site certificates.
func enrollMainCertificateTarget(domain string, nodeID int, rsaOnly bool) error {
	return enrollCertificateTarget(domain, 0, nodeID, rsaOnly, false)
}

func enrollCertificateTarget(domain string, sourceID, nodeID int, rsaOnly, coverSite bool) error {
	if nodeID == sourceID {
		return nil
	}
	coverCertificateFleetMu.Lock()
	defer coverCertificateFleetMu.Unlock()
	rows, err := loadCoverCertificateFleet()
	if err != nil {
		return err
	}
	index := -1
	for i, row := range rows {
		if row.Domain != domain {
			continue
		}
		if row.SourceNodeID == sourceID {
			index = i
			continue
		}
		for _, dst := range row.Targets {
			if dst.NodeID == nodeID {
				return errors.New("this node already receives this certificate from another source")
			}
		}
	}
	if index < 0 {
		if len(rows) >= 64 {
			return errors.New("at most 64 shared domains are supported")
		}
		rows = append(rows, CoverCertificateFleet{SourceNodeID: sourceID, Domain: domain, MainCertificate: sourceID == 0, Targets: []CoverCertificateTarget{}})
		index = len(rows) - 1
	}
	rows[index].MainCertificate = sourceID == 0
	found := false
	for i := range rows[index].Targets {
		dst := &rows[index].Targets[i]
		if dst.NodeID == nodeID {
			dst.RSAOnly = dst.RSAOnly || rsaOnly
			dst.CoverSite = dst.CoverSite || coverSite
			dst.State = "pending"
			found = true
			break
		}
	}
	if !found {
		if len(rows[index].Targets) >= 32 {
			return errors.New("at most 32 recipient nodes per domain")
		}
		rows[index].Targets = append(rows[index].Targets, CoverCertificateTarget{NodeID: nodeID, RSAOnly: rsaOnly, CoverSite: coverSite, State: "pending"})
	}
	if err := saveFirewallJSON(coverCertificateFleetKey, rows); err != nil {
		return err
	}
	select {
	case coverCertificateFleetWake <- struct{}{}:
	default:
	}
	return nil
}

func (s *ManagerService) PrepareInboundCertificate(ctx context.Context, owner int, req InboundCertificateRequest) (InboundCertificatePreparation, error) {
	var empty InboundCertificatePreparation
	if owner < 1 {
		return empty, errors.New("administrator session required")
	}
	name, err := distributionDomain(req.Domain)
	if err != nil || req.NodeID < 0 {
		return empty, errors.New("select a valid node and DNS hostname")
	}
	req.Domain = name
	if req.NodeID > 0 {
		if _, err := verifiedCertificateNode(req.NodeID); err != nil {
			return empty, err
		}
	}
	status, err := InboundCertificateStatus(ctx, req)
	if err != nil {
		return empty, err
	}
	if (status.State == "valid" && !req.CoverSite) || status.State == "pending" || status.State == "retrying" {
		return status, nil
	}
	if req.CoverSite {
		var sites []ManagedCoverSiteSummary
		if req.NodeID == 0 {
			sites, err = LocalManagedCoverSites()
		} else {
			sites, err = ManagedCoverSites(ctx)
		}
		found := false
		for _, site := range sites {
			if site.NodeID == req.NodeID && site.Domain == name {
				found = true
			}
		}
		if err != nil || !found {
			return empty, errors.New("select an existing enabled cover site on this node")
		}
	}
	if status.State == "valid" && req.CoverSite {
		if req.NodeID == 0 {
			return status, nil
		}
		// A valid site needs no new issuance; a missing site that has only an
		// installed copy must still enroll and apply it to the HTTPS frontend.
		status.State = "not_requested"
		if _, _, e := localManagedCertificatePayload(ctx, name, req.RSAOnly); e == nil {
			status.State = "available"
		} else if source, ok := readyCoverCertificateSource(ctx, name, req.NodeID, req.RSAOnly); ok {
			status.State = "available"
			status.SourceNodeID = source
		}
	}
	if status.State != "available" {
		// CertMagic currently issues ECDSA identities. Do not silently give FPTN
		// an incompatible certificate or start an order that cannot be consumed.
		if req.RSAOnly {
			return empty, errors.New("FPTN requires an existing RSA certificate; automatic ECDSA issuance is not compatible")
		}
		current, err := DomainFrontStatus()
		if err != nil {
			return empty, err
		}
		if strings.TrimSpace(req.Email) == "" {
			req.Email = current.Config.Email
		}
		accepted := current.Config.AcceptTerms
		if req.AcceptTerms != nil {
			accepted = *req.AcceptTerms
		}
		if _, err := s.ProvisionLocalCertificate(ctx, owner, ManagerLocalCertificateInput{Domain: name, Email: req.Email, AcceptTerms: accepted}); err != nil {
			return empty, err
		}
	}
	if err := enrollCertificateTarget(name, status.SourceNodeID, req.NodeID, req.RSAOnly, req.CoverSite); err != nil {
		return empty, err
	}
	return InboundCertificateStatus(ctx, req)
}
