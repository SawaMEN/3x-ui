package domainfront

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

type DomainFrontDomain struct {
	Name         string `json:"name" example:"sub.example.com"`
	Mode         string `json:"mode" example:"sub_link_only"`
	Enabled      bool   `json:"enabled"`
	Panel        bool   `json:"panel"`
	InboundIDs   []int  `json:"inboundIds"`
	Origin       string `json:"origin"`
	TLS          string `json:"tls" example:"auto"`
	CertFile     string `json:"certFile"`
	KeyFile      string `json:"keyFile"`
	SiteCatalog  string `json:"siteCatalog,omitempty"`
	SiteTemplate string `json:"siteTemplate,omitempty"`
}

type DomainFrontConfig struct {
	Enabled            bool                `json:"enabled"`
	Listen             string              `json:"listen"`
	HTTPPort           int                 `json:"httpPort" example:"80"`
	HTTPSPort          int                 `json:"httpsPort" example:"443"`
	PublicPort         int                 `json:"publicPort" example:"443"`
	Email              string              `json:"email"`
	AcceptTerms        bool                `json:"acceptTerms"`
	Staging            bool                `json:"staging"`
	DNS01Provider      string              `json:"dns01Provider,omitempty"`
	DNS01Zone          string              `json:"dns01Zone,omitempty"`
	DNS01ZoneID        string              `json:"dns01ZoneId,omitempty"`
	DNS01Propagation   int                 `json:"dns01Propagation,omitempty"`
	SubscriptionDomain string              `json:"subscriptionDomain"`
	Domains            []DomainFrontDomain `json:"domains"`
}

// HasPublicSites is false for DNS-01 certificate-only and foreign SNI records.
// Such records must not bind public HTTP/HTTPS listeners.
func (c DomainFrontConfig) HasPublicSites() bool {
	if !c.Enabled {
		return false
	}
	for _, d := range c.Domains {
		if d.Enabled && d.Mode != "shared_sni" && d.Mode != "cert_only" {
			return true
		}
	}
	return false
}

type Route struct {
	Path   string
	Target string
	Host   string
	H2C    bool
}

type CertificateFiles struct {
	CertFile  string
	KeyFile   string
	Source    string
	Renewal   string
	AutoRenew bool
	Warning   string
}

type Backend struct {
	PanelHandler        http.Handler
	SubscriptionHandler http.Handler
	SharedHTTP          bool
	SharedHTTPS         bool
	Certificates        map[string]CertificateFiles
	ChallengeRoots      map[string]string
	CertificateEvent    func(context.Context, string, map[string]any) error
	DNS01Token          string // private; never serialized into DomainFrontConfig or API
	GetRedirect         func() (string, error)
	SubscriptionHost    string

	SubscriptionTarget string
	PanelTarget        string
	PanelHost          string
	PanelPath          string
	SubscriptionPaths  []string
	ProtectedPaths     []string // Legacy application paths served by NoRoute.
	Routes             map[string][]Route
	DecoyDirectories   map[string]string
	RedirectURL        string
}

func DefaultConfig() DomainFrontConfig {
	return DomainFrontConfig{Listen: "0.0.0.0", HTTPPort: 80, HTTPSPort: 443, PublicPort: 443, Domains: []DomainFrontDomain{}}
}

func (c *DomainFrontConfig) Validate() error { return c.ValidateWithCertificates(nil) }

func (c *DomainFrontConfig) ValidateWithCertificates(files map[string]CertificateFiles) error {
	if c.Domains == nil {
		c.Domains = []DomainFrontDomain{}
	}
	c.SubscriptionDomain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(c.SubscriptionDomain)), ".")
	if c.Listen == "" {
		c.Listen = "0.0.0.0"
	}
	if net.ParseIP(c.Listen) == nil {
		return fmt.Errorf("listen address must be an IP address")
	}
	for _, port := range []int{c.HTTPPort, c.HTTPSPort, c.PublicPort} {
		if port < 1 || port > 65535 {
			return fmt.Errorf("domain gateway ports must be between 1 and 65535")
		}
	}
	if c.HTTPPort == c.HTTPSPort {
		return fmt.Errorf("HTTP and HTTPS ports must be different")
	}
	if len(c.Domains) > 100 {
		return fmt.Errorf("at most 100 domains are supported")
	}
	c.DNS01Provider = strings.ToLower(strings.TrimSpace(c.DNS01Provider))
	c.DNS01Zone = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(c.DNS01Zone), "."))
	c.DNS01ZoneID = strings.TrimSpace(c.DNS01ZoneID)
	if c.DNS01Provider != "" {
		if c.DNS01Provider != "cloudflare" && c.DNS01Provider != "digitalocean" {
			return fmt.Errorf("unsupported DNS-01 provider")
		}
		if !validPublicDNSName(c.DNS01Zone) {
			return fmt.Errorf("DNS-01 requires a valid public DNS zone")
		}
		if c.DNS01Provider == "cloudflare" && !cloudflareID.MatchString(c.DNS01ZoneID) {
			return fmt.Errorf("DNS-01 requires a valid Cloudflare zone ID")
		}
		if c.DNS01Provider == "digitalocean" && c.DNS01ZoneID != "" {
			return fmt.Errorf("DigitalOcean DNS-01 does not use a zone ID")
		}
		if c.DNS01Propagation < 0 || c.DNS01Propagation > 180 {
			return fmt.Errorf("DNS-01 propagation delay must be between 0 and 180 seconds")
		}
	} else if c.DNS01Zone != "" || c.DNS01ZoneID != "" || c.DNS01Propagation != 0 {
		return fmt.Errorf("DNS-01 provider must be selected before configuring a zone")
	}
	c.Email = strings.TrimSpace(c.Email)
	if c.Email != "" {
		address, err := mail.ParseAddress(c.Email)
		if err != nil {
			return fmt.Errorf("invalid ACME email: %w", err)
		}
		c.Email = address.Address
	}
	names := map[string]bool{}
	auto := false
	for i := range c.Domains {
		d := &c.Domains[i]
		if d.InboundIDs == nil {
			d.InboundIDs = []int{}
		}
		d.Name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d.Name)), ".")
		// A wildcard covers precisely one subdomain label. Its issuance
		// requires DNS-01; the normal HTTP challenge cannot validate it.
		wildcard := strings.HasPrefix(d.Name, "*.")
		base := d.Name
		if wildcard {
			base = strings.TrimPrefix(base, "*.")
			if c.DNS01Provider == "" || (d.TLS != "auto" && d.TLS != "certmagic") {
				return fmt.Errorf("%s: wildcard certificates require automatic DNS-01 validation", d.Name)
			}
		}
		name, err := netsafe.NormalizeHost(base)
		if err != nil || net.ParseIP(name) != nil || !strings.Contains(name, ".") {
			return fmt.Errorf("a public DNS hostname is required: %s", d.Name)
		}
		if c.DNS01Provider != "" && d.Enabled && d.Mode != "shared_sni" && (d.TLS == "auto" || d.TLS == "certmagic") &&
			d.Name != c.DNS01Zone && !strings.HasSuffix(d.Name, "."+c.DNS01Zone) {
			return fmt.Errorf("%s: DNS-01 zone does not own this domain", d.Name)
		}
		if names[d.Name] {
			return fmt.Errorf("duplicate domain: %s", d.Name)
		}
		names[d.Name] = true
		if d.Mode == "shared_sni" {
			// A foreign cover site is registered on the controlling panel only.
			// It must never issue a certificate or serve traffic from here.
			if d.Panel || len(d.InboundIDs) != 0 || strings.TrimSpace(d.Origin) != "" {
				return fmt.Errorf("shared SNI domains cannot expose panel, inbounds or relay origin")
			}
			d.TLS, d.CertFile, d.KeyFile, d.Origin = "auto", "", "", ""
			continue
		}
		switch d.Mode {
		case "direct", "cdn", "relay", "sub_link_only", "sni_only", "cert_only":
		default:
			return fmt.Errorf("unknown domain mode: %s", d.Mode)
		}
		if d.Mode == "sni_only" {
			if (d.SiteCatalog == "") != (d.SiteTemplate == "") {
				return fmt.Errorf("%s: cover site catalog and template must be selected together", d.Name)
			}
		} else if d.SiteCatalog != "" || d.SiteTemplate != "" {
			return fmt.Errorf("%s: only SNI-only domains can select a cover site template", d.Name)
		}
		switch d.TLS {
		case "auto", "certmagic", "existing":
			_, inherited := files[d.Name]
			if d.TLS == "existing" && c.Enabled && d.Enabled && !inherited {
				return fmt.Errorf("%s: no matching existing panel, subscription or installer certificate", d.Name)
			}
			auto = auto || (d.Enabled && (d.TLS == "certmagic" || d.TLS == "auto" && !inherited))
		case "manual":
			if d.CertFile == "" || d.KeyFile == "" {
				return fmt.Errorf("certificate and private key files are required for %s", d.Name)
			}
		default:
			return fmt.Errorf("TLS must be auto, certmagic, existing or manual")
		}
		if d.Mode == "cert_only" {
			if d.TLS != "certmagic" || (c.Enabled && d.Enabled && c.DNS01Provider == "") {
				return fmt.Errorf("%s: certificate-only domains require CertMagic and DNS-01", d.Name)
			}
			if d.Panel || len(d.InboundIDs) != 0 || strings.TrimSpace(d.Origin) != "" {
				return fmt.Errorf("%s: certificate-only domains cannot expose sites, panel, VPN inbounds or origins", d.Name)
			}
		}
		if d.Mode == "sub_link_only" && (d.Panel || len(d.InboundIDs) > 0) {
			return fmt.Errorf("subscription-only domains cannot expose the panel or VPN inbounds")
		}
		if d.Mode == "sni_only" && (d.Panel || len(d.InboundIDs) > 0) {
			return fmt.Errorf("SNI-only domains cannot expose the panel or VPN inbounds")
		}
		d.Origin = strings.TrimSpace(d.Origin)
		if d.Mode == "relay" {
			u, err := url.Parse(d.Origin)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
				return fmt.Errorf("Relay requires an HTTPS origin without path, credentials, query or fragment")
			}
			if port := u.Port(); port != "" {
				n, err := strconv.Atoi(port)
				if err != nil || n < 1 || n > 65535 {
					return fmt.Errorf("invalid Relay origin port")
				}
			}
			if _, err := netsafe.NormalizeHost(u.Hostname()); err != nil {
				return fmt.Errorf("invalid Relay origin hostname: %w", err)
			}
			if strings.EqualFold(u.Hostname(), d.Name) {
				return fmt.Errorf("Relay origin cannot point to itself")
			}
		} else if d.Origin != "" {
			return fmt.Errorf("origin is only used by Relay domains")
		}
		ids := map[int]bool{}
		for _, id := range d.InboundIDs {
			if id <= 0 || ids[id] {
				return fmt.Errorf("inbound IDs must be positive and unique")
			}
			ids[id] = true
		}
	}
	for _, start := range c.Domains {
		visited := map[string]bool{}
		current := start
		for current.Mode == "relay" {
			if visited[current.Name] {
				return fmt.Errorf("Relay origin cycle involving %s", current.Name)
			}
			visited[current.Name] = true
			u, _ := url.Parse(current.Origin)
			found := false
			for _, candidate := range c.Domains {
				if candidate.Name == strings.ToLower(u.Hostname()) {
					current = candidate
					found = true
					break
				}
			}
			if !found {
				break
			}
		}
	}
	if c.Enabled && auto && !c.AcceptTerms {
		return fmt.Errorf("accept the Let's Encrypt terms to enable automatic TLS")
	}
	if c.SubscriptionDomain != "" {
		found := false
		for _, d := range c.Domains {
			if d.Enabled && d.Mode != "sni_only" && d.Mode != "shared_sni" && d.Mode != "cert_only" && d.Name == c.SubscriptionDomain {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("canonical subscription domain must be an enabled domain")
		}
	}
	return nil
}

type DomainFrontCheck struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	CheckedAt int64    `json:"checkedAt"`
	ExpiresAt int64    `json:"expiresAt"`
	DaysLeft  int      `json:"daysLeft"`
	Issuer    string   `json:"issuer"`
	Error     string   `json:"error"`
}
