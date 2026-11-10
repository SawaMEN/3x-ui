package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

// nodeCertificateInventoryItem accepts metadata only. A node's paths and
// arbitrary warnings never leave that node or become local filesystem inputs.
func nodeCertificateInventoryItem(nodeID int, nodeName string, cert runtime.RemoteCertificateSummary) (CertificateInventoryItem, bool) {
	name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cert.Name), "."))
	dnsName := strings.TrimPrefix(name, "*.")
	if nodeID <= 0 || len(name) < 4 || len(name) > 253 ||
		!strings.Contains(dnsName, ".") || strings.Contains(dnsName, "*") {
		return CertificateInventoryItem{}, false
	}
	for _, label := range strings.Split(dnsName, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return CertificateInventoryItem{}, false
		}
		for _, ch := range label {
			if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
				continue
			}
			return CertificateInventoryItem{}, false
		}
	}
	manager := cert.Manager
	switch manager {
	case "certmagic", "acme.sh", "certbot", "manual", "external":
	default:
		manager = "external"
	}
	challenge := cert.Challenge
	switch challenge {
	case "dns-01", "http-01", "tls-alpn-01", "http-01+tls-alpn-01", "none":
	default:
		challenge = "unknown"
	}
	state := cert.State
	switch state {
	case "valid", "renewal_due", "expired", "error", "pending", "disabled", "external":
	default:
		state = "external"
	}
	id := sha256.Sum256([]byte("node|" + strconv.Itoa(nodeID) + "|" + cert.ID + "|" + name))
	names := make([]string, 0, len(cert.Names))
	for _, n := range cert.Names {
		n = strings.TrimSpace(n)
		if len(n) > 0 && len(n) <= 253 {
			names = append(names, n)
		}
		if len(names) == 32 {
			break
		}
	}
	if len(names) == 0 {
		names = []string{name}
	}
	issuer := strings.TrimSpace(cert.Issuer)
	if len(issuer) > 128 {
		issuer = issuer[:128]
	}
	return CertificateInventoryItem{
		ID: hex.EncodeToString(id[:12]), Name: name, Names: names,
		UsedBy: []string{"node"}, Manager: manager, Challenge: challenge,
		Ports: []int{}, Source: "remote", State: state,
		Issuer: issuer, ExpiresAt: cert.ExpiresAt, DaysLeft: cert.DaysLeft,
		AutoRenew: cert.AutoRenew,
		NodeID: nodeID, NodeName: nodeName,
	}, true
}

// readRemoteCertificateInventories fetches node-owned certificate state through
// the existing pinned, authenticated node channel. Failure on one node cannot
// block the local registry or trigger certificate issuance on another machine.
func readRemoteCertificateInventories(ctx context.Context) []CertificateInventoryItem {
	manager := runtime.GetManager()
	db := database.GetDB()
	if manager == nil || db == nil {
		return nil
	}
	var nodes []model.Node
	if err := db.Where("enable = ?", true).Limit(64).Find(&nodes).Error; err != nil {
		return nil
	}
	if len(nodes) == 0 {
		return nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	limit := make(chan struct{}, 6)
	results := make(chan []CertificateInventoryItem, len(nodes))
	var wg sync.WaitGroup
	for _, node := range nodes {
		if node.Id <= 0 {
			continue
		}
		wg.Add(1)
		go func(node model.Node) {
			defer wg.Done()
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-requestCtx.Done():
				return
			}
			remote, err := manager.RemoteFor(&node)
			if err != nil {
				return
			}
			certs, err := remote.ListCertificateSummaries(requestCtx)
			if err != nil {
				return
			}
			items := make([]CertificateInventoryItem, 0, len(certs))
			for _, cert := range certs {
				if item, ok := nodeCertificateInventoryItem(node.Id, node.Name, cert); ok {
					items = append(items, item)
				}
			}
			results <- items
		}(node)
	}
	wg.Wait()
	close(results)
	var out []CertificateInventoryItem
	for group := range results {
		out = append(out, group...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].NodeID != out[j].NodeID {
			return out[i].NodeID < out[j].NodeID
		}
		return out[i].ID < out[j].ID
	})
	return out
}
