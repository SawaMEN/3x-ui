package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

const coverCertificateFleetKey = "coverCertificateFleet"

var (
	coverCertificateFleetMu   sync.Mutex
	coverCertificateFleetWake = make(chan struct{}, 1)
)

type CoverCertificateTarget struct {
	NodeID        int    `json:"nodeId"`
	CoverSite     bool   `json:"coverSite,omitempty"`
	RSAOnly       bool   `json:"rsaOnly,omitempty"`
	DeliveredHash string `json:"deliveredHash,omitempty"`
	State         string `json:"state"`
	ExpiresAt     int64  `json:"expiresAt"`
	CheckedAt     int64  `json:"checkedAt"`
	Error         string `json:"error,omitempty"`
}

type CoverCertificateFleet struct {
	SourceNodeID    int                      `json:"sourceNodeId"`
	MainCertificate bool                     `json:"mainCertificate,omitempty"`
	Domain          string                   `json:"domain"`
	Targets         []CoverCertificateTarget `json:"targets"`
}

func loadCoverCertificateFleet() ([]CoverCertificateFleet, error) {
	raw, err := firewallSetting(coverCertificateFleetKey, "[]")
	if err != nil {
		return nil, err
	}
	var rows []CoverCertificateFleet
	err = json.Unmarshal([]byte(raw), &rows)
	return rows, err
}

func CoverCertificateFleets() ([]CoverCertificateFleet, error) {
	// A single settings row is read atomically. Status polling must not wait
	// for a slow node delivery; the mutex serializes writers only.
	return loadCoverCertificateFleet()
}

func verifiedCertificateNode(id int) (*runtime.Remote, error) {
	if id <= 0 || database.GetDB() == nil || runtime.GetManager() == nil {
		return nil, errors.New("node manager is unavailable")
	}
	var node model.Node
	if err := database.GetDB().First(&node, id).Error; err != nil {
		return nil, errors.New("node is unavailable")
	}
	if !node.Enable || node.Transitive || node.Scheme != "https" || node.TlsVerifyMode == "skip" {
		return nil, errors.New("select an enabled direct node with verified HTTPS")
	}
	return runtime.GetManager().RemoteFor(&node)
}

// ExportLocalCoverCertificate is deliberately restricted to an enabled owned
// cover site. It cannot export panel, imported, or distributed identities.
// Its sole caller is the authenticated master-only node endpoint.
func ExportLocalCoverCertificate(ctx context.Context, domain string) (runtime.TLSCertificateInstall, error) {
	var empty runtime.TLSCertificateInstall
	name, err := distributionDomain(domain)
	if err != nil {
		return empty, err
	}
	sites, err := LocalCoverCertificateSources()
	if err != nil {
		return empty, err
	}
	owned := false
	for _, site := range sites {
		if site.Domain == name {
			owned = true
			break
		}
	}
	if !owned {
		return empty, errors.New("source must host this enabled owned cover site")
	}
	inventory, err := ManagedSNIDomains(ctx, false)
	if err != nil {
		return empty, err
	}
	for _, cert := range inventory {
		if cert.Name != name || (cert.Source != "gateway" && cert.Source != "telemt") || cert.CertFile == "" || cert.KeyFile == "" {
			continue
		}
		chain, err := readDistributedSource(cert.CertFile)
		if err != nil {
			return empty, err
		}
		key, err := readDistributedSource(cert.KeyFile)
		if err != nil {
			return empty, err
		}
		if _, err := validateDistributedPair(name, chain, key); err != nil {
			return empty, err
		}
		return runtime.TLSCertificateInstall{Domain: name, CertificatePEM: string(chain), PrivateKeyPEM: string(key)}, nil
	}
	return empty, errors.New("source certificate is not ready; automatic retry is scheduled")
}

// SaveCoverCertificateFleet replaces this site's recipient list atomically.
// Empty recipients stop renewal delivery; installed files remain usable.
func SaveCoverCertificateFleet(ctx context.Context, source int, domain string, nodes []int) ([]CoverCertificateFleet, error) {
	name, err := distributionDomain(domain)
	if err != nil || source < 0 {
		return nil, errors.New("invalid source or hostname")
	}
	if len(nodes) > 32 {
		return nil, errors.New("select at most 32 recipient nodes")
	}
	if source > 0 && len(nodes) > 0 {
		if _, err := verifiedCertificateNode(source); err != nil {
			return nil, err
		}
	}
	seen := map[int]bool{}
	for _, id := range nodes {
		if id < 0 || id == source || seen[id] {
			return nil, errors.New("recipient nodes must be unique and different from the site node")
		}
		seen[id] = true
		if id > 0 {
			if _, err := verifiedCertificateNode(id); err != nil {
				return nil, err
			}
		}
	}
	// Only discover public site metadata. Never obtain key material in this UI request.
	report, err := CoverCertificateSources(ctx)
	if err != nil {
		return nil, err
	}
	found := false
	for _, site := range report.Sites {
		if site.NodeID == source && site.Domain == name {
			found = true
			break
		}
	}
	if !found && source == 0 && len(nodes) > 0 {
		if status, e := DomainFrontStatus(); e == nil {
			for _, d := range status.Config.Domains {
				if d.Name == name && d.Enabled && d.Mode == "cert_only" {
					found = true
				}
			}
		}
	}
	if !found && len(nodes) > 0 {
		return nil, errors.New("select an existing owned cover site on an online node")
	}
	coverCertificateFleetMu.Lock()
	defer coverCertificateFleetMu.Unlock()
	rows, err := loadCoverCertificateFleet()
	if err != nil {
		return nil, err
	}
	next := CoverCertificateFleet{SourceNodeID: source, Domain: name, Targets: []CoverCertificateTarget{}}
	for _, row := range rows {
		if row.SourceNodeID == source && row.Domain == name {
			next.MainCertificate = row.MainCertificate
		}
	}
	for _, id := range nodes {
		target := CoverCertificateTarget{NodeID: id, State: "pending"}
		for _, row := range rows {
			if row.Domain == name && row.SourceNodeID != source {
				for _, old := range row.Targets {
					if old.NodeID == id {
						return nil, errors.New("recipient already follows another source for this hostname")
					}
				}
			}
			if row.Domain == name && row.SourceNodeID == source {
				for _, old := range row.Targets {
					if old.NodeID == id {
						target = old
					}
				}
			}
		}
		next.Targets = append(next.Targets, target)
	}
	output := make([]CoverCertificateFleet, 0, len(rows)+1)
	for _, row := range rows {
		if row.Domain != name || row.SourceNodeID != source {
			output = append(output, row)
		}
	}
	if len(nodes) > 0 {
		output = append(output, next)
	}
	if len(output) > 64 {
		return nil, errors.New("at most 64 shared sites are supported")
	}
	if err := saveFirewallJSON(coverCertificateFleetKey, output); err != nil {
		return nil, err
	}
	select {
	case coverCertificateFleetWake <- struct{}{}:
	default:
	}
	return output, nil
}

func syncCoverCertificateFleet(ctx context.Context) {
	rows, err := CoverCertificateFleets()
	if err != nil {
		return
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		sourceCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		var payload runtime.TLSCertificateInstall
		if row.SourceNodeID == 0 {
			if row.MainCertificate {
				payload, _, err = localManagedCertificatePayload(sourceCtx, row.Domain, false)
			} else {
				payload, err = ExportLocalCoverCertificate(sourceCtx, row.Domain)
			}
		} else {
			var remote *runtime.Remote
			remote, err = verifiedCertificateNode(row.SourceNodeID)
			if err == nil {
				payload, err = remote.ExportCoverCertificate(sourceCtx, row.Domain)
			}
		}
		cancel()
		sourceErr := err
		expiry := int64(0)
		if sourceErr == nil {
			expiry, sourceErr = validateDistributedPair(row.Domain, []byte(payload.CertificatePEM), []byte(payload.PrivateKeyPEM))
		}
		sum := sha256.Sum256([]byte(payload.CertificatePEM + payload.PrivateKeyPEM))
		hash := hex.EncodeToString(sum[:])
		for _, target := range row.Targets {
			if ctx.Err() != nil {
				return
			}
			if sourceErr == nil && target.State == "ready" && target.DeliveredHash == hash && time.Now().Unix()-target.CheckedAt < 3600 {
				continue
			}
			// Serialize each delivery against enrollment changes; removing a target
			// cannot race a subsequent key transfer. No global lock during source IO.
			coverCertificateFleetMu.Lock()
			current, readErr := loadCoverCertificateFleet()
			if readErr != nil {
				coverCertificateFleetMu.Unlock()
				return
			}
			for i := range current {
				if current[i].SourceNodeID != row.SourceNodeID || current[i].Domain != row.Domain {
					continue
				}
				for j := range current[i].Targets {
					dst := &current[i].Targets[j]
					if dst.NodeID != target.NodeID {
						continue
					}
					opCtx, stop := context.WithTimeout(ctx, 25*time.Second)
					deliveryErr := sourceErr
					if deliveryErr == nil && dst.RSAOnly && !distributedPayloadSupportsRSA(payload) {
						deliveryErr = errors.New("an RSA certificate is required")
					}
					if deliveryErr == nil {
						if dst.NodeID == 0 {
							payload.CoverSite = dst.CoverSite
							payload.Apply = dst.DeliveredHash != hash || (dst.CoverSite && dst.State != "ready")
							_, deliveryErr = InstallAndApplyDistributedTLSCertificate(opCtx, payload)
						} else {
							var remote *runtime.Remote
							remote, deliveryErr = verifiedCertificateNode(dst.NodeID)
							if deliveryErr == nil {
								payload.CoverSite = dst.CoverSite
								payload.Apply = dst.DeliveredHash != hash || (dst.CoverSite && dst.State != "ready")
								var installed runtime.InstalledTLSCertificate
								installed, deliveryErr = remote.InstallTLSCertificate(opCtx, payload)
								if deliveryErr == nil && installed.ExpiresAt != expiry {
									deliveryErr = errors.New("recipient returned an unexpected expiration")
								}
							}
						}
					}
					stop()
					dst.CheckedAt = time.Now().Unix()
					if deliveryErr != nil {
						dst.State = "retrying"
						// Node errors can contain response bodies: never persist or expose them.
						dst.Error = "Certificate is not ready, node is unavailable, or apply failed; retrying automatically"
					} else {
						dst.State = "ready"
						dst.Error = ""
						dst.ExpiresAt = expiry
						dst.DeliveredHash = hash
					}
				}
			}
			if err := saveFirewallJSON(coverCertificateFleetKey, current); err != nil {
				logger.Error("persist cover certificate delivery state failed")
			}
			coverCertificateFleetMu.Unlock()
		}
	}
}

func runCoverCertificateFleetScheduler(ctx context.Context) {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-coverCertificateFleetWake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		syncCoverCertificateFleet(ctx)
		timer.Reset(time.Minute)
	}
}
