package service

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

// CertificateInventoryItem describes a certificate owned by the local panel,
// gateway, or a remote node. Private keys and ACME account secrets are never read.
type CertificateInventoryItem struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Names    []string `json:"names"`
	UsedBy   []string `json:"usedBy"`
	Manager  string   `json:"manager"`
	Challenge string   `json:"challenge"`
	Ports    []int    `json:"ports"`
	Source   string   `json:"source"`
	State    string   `json:"state"`
	Issuer   string   `json:"issuer"`
	ExpiresAt int64    `json:"expiresAt"`
	DaysLeft int      `json:"daysLeft"`
	AutoRenew bool     `json:"autoRenew"`
	CertFile string   `json:"certFile,omitempty"`
	Warning  string   `json:"warning,omitempty"`
	NodeID   int      `json:"nodeId,omitempty"`
	NodeName string   `json:"nodeName,omitempty"`
}

type CertificateInventory struct {
	Items           []CertificateInventoryItem `json:"items"`
	FirewallEnabled bool                       `json:"firewallEnabled"`
	FirewallBackend string                     `json:"firewallBackend"`
	FirewallWarning string                     `json:"firewallWarning,omitempty"`
}

// acme.sh and Certbot keep renewal metadata in different formats. A missing
// or unrecognized authenticator is NOT assumed to be HTTP-01.
func acmeSHChallenge(webroot string) string {
	switch {
	case strings.HasPrefix(webroot, "dns_"):
		return "dns-01"
	case webroot == "alpn":
		return "tls-alpn-01"
	case webroot == "no", filepath.IsAbs(webroot):
		return "http-01"
	default:
		return "unknown"
	}
}

func certbotAuthenticator(contents []byte) string {
	section := ""
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[] ")
			continue
		}
		if section != "renewalparams" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == "authenticator" {
			return strings.Trim(strings.TrimSpace(value), "\"' ")
		}
	}
	return ""
}

func certbotChallenge(certFile string) string {
	clean := filepath.Clean(certFile)
	// Only inspect the well-known Certbot renewal store, never arbitrary
	// user-supplied configuration paths.
	const prefix = "/etc/letsencrypt/live/"
	if !strings.HasPrefix(clean, prefix) || filepath.Base(clean) != "fullchain.pem" {
		return ""
	}
	name := filepath.Base(filepath.Dir(clean))
	if name == "." || name == ".." || name == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join("/etc/letsencrypt/renewal", name+".conf"))
	if err != nil {
		return ""
	}
	switch auth := strings.ToLower(certbotAuthenticator(raw)); {
	case strings.HasPrefix(auth, "dns-"):
		return "dns-01"
	case auth == "standalone", auth == "webroot", auth == "nginx", auth == "apache":
		return "http-01"
	default:
		return "unknown"
	}
}

func certificateManager(cert domainfront.DomainFrontCertificate, acme []domainACME) (string, string) {
	if cert.Source == "remote" || cert.State == "external" {
		return "remote", "none"
	}
	for _, state := range acme {
		if cert.CertFile != "" && cert.CertFile == state.values["Le_RealFullChainPath"] {
			return "acme.sh", acmeSHChallenge(state.values["Le_Webroot"])
		}
	}
	if challenge := certbotChallenge(cert.CertFile); challenge != "" {
		return "certbot", challenge
	}
	switch cert.Renewal {
	case "certmagic":
		return "certmagic", "http-01+tls-alpn-01"
	case "manual":
		return "manual", "none"
	default:
		return "external", "unknown"
	}
}

func challengeFirewallPorts(challenge string, httpPort, httpsPort int) []int {
	switch challenge {
	case "http-01":
		return []int{httpPort}
	case "tls-alpn-01":
		return []int{httpsPort}
	case "http-01+tls-alpn-01":
		return []int{httpPort, httpsPort}
	default:
		return []int{}
	}
}

func certificateNames(certFile string, name string) []string {
	names := []string{}
	raw, err := os.ReadFile(certFile)
	if err == nil {
		block, _ := pem.Decode(raw)
		if block != nil {
			if leaf, parseErr := x509.ParseCertificate(block.Bytes); parseErr == nil {
				names = append(names, leaf.DNSNames...)
			}
		}
	}
	if len(names) == 0 && name != "" {
		names = append(names, name)
	}
	return names
}

func makeCertificateInventory(status domainfront.DomainFrontStatus, states []domainACME, telemt *TelemtWebProxyState) []CertificateInventoryItem {
	certs := append([]domainfront.DomainFrontCertificate{}, status.Certificates...)
	certs = append(certs, status.ExistingCertificates...)
	if telemt != nil && telemt.Enabled && telemt.CertFile != "" {
		// A separate Telemt WEB frontend may own the public HTTPS listener.
		certs = append(certs, domainfront.DomainFrontCertificate{
			Name: telemt.Domain, Source: "telemt", Renewal: "external",
			Mode: "existing", State: "valid", CertFile: telemt.CertFile,
			KeyFile: telemt.KeyFile,
		})
	}
	byKey := map[string]*CertificateInventoryItem{}
	for _, cert := range certs {
		manager, challenge := certificateManager(cert, states)
		if cert.Source == "telemt" && manager == "remote" {
			manager, challenge = "external", "unknown"
		}
		// A remote SNI placeholder never identifies a certificate stored here.
		key := cert.CertFile + "|" + cert.KeyFile
		if cert.CertFile == "" || cert.KeyFile == "" {
			key = cert.Source + "|" + cert.Name + "|" + cert.Mode
		}
		item, ok := byKey[key]
		if !ok {
			hash := sha256.Sum256([]byte(key))
			item = &CertificateInventoryItem{
				ID: hex.EncodeToString(hash[:12]), Name: cert.Name,
				Names:  certificateNames(cert.CertFile, cert.Name),
				UsedBy: []string{}, Manager: manager, Challenge: challenge,
				Ports: []int{}, Source: cert.Source, State: cert.State,
				Issuer: cert.Issuer, ExpiresAt: cert.ExpiresAt, DaysLeft: cert.DaysLeft,
				AutoRenew: cert.AutoRenew, CertFile: cert.CertFile, Warning: cert.Error,
			}
			byKey[key] = item
		}
		if cert.Source != "" && !containsCertificateLabel(item.UsedBy, cert.Source) {
			item.UsedBy = append(item.UsedBy, cert.Source)
		}
		if item.ExpiresAt == 0 && cert.ExpiresAt > 0 {
			item.ExpiresAt, item.DaysLeft, item.Issuer, item.State = cert.ExpiresAt, cert.DaysLeft, cert.Issuer, cert.State
		}
		if cert.AutoRenew {
			item.AutoRenew = true
		}
		if item.Warning == "" {
			item.Warning = cert.Error
		}
		// Public challenges apply to the host which owns the certificate;
		// remote SNI files must never be opened or renewed locally.
		if manager != "remote" {
			httpPort, httpsPort := 80, 443
			if manager == "certmagic" {
				httpPort, httpsPort = status.Config.HTTPPort, status.Config.HTTPSPort
			}
			item.Ports = challengeFirewallPorts(challenge, httpPort, httpsPort)
		}
	}
	out := make([]CertificateInventoryItem, 0, len(byKey))
	for _, item := range byKey {
		sort.Strings(item.UsedBy)
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func containsCertificateLabel(labels []string, target string) bool {
	for _, label := range labels {
		if label == target {
			return true
		}
	}
	return false
}

// appendRemoteCertificateReferences adds node references without touching
// their certificate files or disclosing node-local private-key paths.
func appendRemoteCertificateReferences(items []CertificateInventoryItem, sites []ManagedSNIDomain) []CertificateInventoryItem {
	for _, site := range sites {
		if site.Source != "remote" || site.NodeID <= 0 {
			continue
		}
		key := "remote|" + strconv.Itoa(site.NodeID) + "|" + site.Name
		hash := sha256.Sum256([]byte(key))
		items = append(items, CertificateInventoryItem{
			ID: hex.EncodeToString(hash[:12]), Name: site.Name,
			Names: []string{site.Name}, UsedBy: []string{"node"},
			Manager: "remote", Challenge: "unknown",
			Ports: []int{}, Source: "remote", State: "external",
			NodeID: site.NodeID, NodeName: site.NodeName,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].ID < items[j].ID
	})
	return items
}

// DomainCertificateInventory only inspects certificates of THIS host.
// No remote node's private-key path is ever used as a local filesystem path.
func DomainCertificateInventory(ctx context.Context) (CertificateInventory, error) {
	status, err := DomainFrontStatus()
	if err != nil {
		return CertificateInventory{}, err
	}
	var state *TelemtWebProxyState
	if telemt, readErr := readTelemtWebState(); readErr == nil && telemt.Enabled {
		state = &telemt
	}
	inventory := CertificateInventory{Items: makeCertificateInventory(status, domainACMEStates(domainACMEHomes()), state)}
	if sites, sitesErr := ManagedSNIDomains(ctx, true); sitesErr == nil {
		inventory.Items = appendRemoteCertificateReferences(inventory.Items, sites)
	}
	firewall, fwErr := (&FirewallService{}).GetManagedStatusSafe(ctx, rememberedFirewallSafetyPort())
	if fwErr == nil {
		inventory.FirewallEnabled = firewall.Enabled && firewall.Supported
		inventory.FirewallBackend = firewall.Backend
		if !inventory.FirewallEnabled {
			inventory.FirewallWarning = "3x-ui firewall management is disabled or not supported; verify host and provider firewall rules."
		}
		if firewall.InspectionError != "" {
			inventory.FirewallWarning = firewall.InspectionError
		}
	} else {
		inventory.FirewallWarning = fmt.Sprintf("Firewall status unavailable: %v", fwErr)
	}
	return inventory, nil
}
