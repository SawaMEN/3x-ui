package service

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"errors"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

type InboundCertificateRequest struct {
	NodeID      int    `json:"nodeId"`
	Domain      string `json:"domain"`
	RSAOnly     bool   `json:"rsaOnly,omitempty"`
	Email       string `json:"email,omitempty"`
	AcceptTerms *bool  `json:"acceptTerms,omitempty"`
}

type InboundCertificatePreparation struct {
	NodeID    int    `json:"nodeId"`
	Domain    string `json:"domain"`
	State     string `json:"state"`
	CertFile  string `json:"certFile,omitempty"`
	KeyFile   string `json:"keyFile,omitempty"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
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

// Enroll additively: preparing Finland must not remove USA or other recipients.
// The same persistent worker delivers both DNS-01 and owned-site certificates.
func enrollMainCertificateTarget(domain string, nodeID int, rsaOnly bool) error {
	if nodeID <= 0 {
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
		if row.SourceNodeID == 0 {
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
		rows = append(rows, CoverCertificateFleet{SourceNodeID: 0, Domain: domain, MainCertificate: true, Targets: []CoverCertificateTarget{}})
		index = len(rows) - 1
	}
	rows[index].MainCertificate = true
	found := false
	for i := range rows[index].Targets {
		dst := &rows[index].Targets[i]
		if dst.NodeID == nodeID {
			dst.RSAOnly = dst.RSAOnly || rsaOnly
			dst.State = "pending"
			found = true
			break
		}
	}
	if !found {
		if len(rows[index].Targets) >= 32 {
			return errors.New("at most 32 recipient nodes per domain")
		}
		rows[index].Targets = append(rows[index].Targets, CoverCertificateTarget{NodeID: nodeID, RSAOnly: rsaOnly, State: "pending"})
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
	if status.State == "valid" || status.State == "pending" || status.State == "retrying" {
		return status, nil
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
	if err := enrollMainCertificateTarget(name, req.NodeID, req.RSAOnly); err != nil {
		return empty, err
	}
	return InboundCertificateStatus(ctx, req)
}
