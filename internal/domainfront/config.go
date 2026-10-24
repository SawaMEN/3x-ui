package domainfront

import (
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
	Name       string `json:"name" example:"sub.example.com"`
	Mode       string `json:"mode" example:"sub_link_only"`
	Enabled    bool   `json:"enabled"`
	Panel      bool   `json:"panel"`
	InboundIDs []int  `json:"inboundIds"`
	Origin     string `json:"origin"`
	TLS        string `json:"tls" example:"auto"`
	CertFile   string `json:"certFile"`
	KeyFile    string `json:"keyFile"`
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
	SubscriptionDomain string              `json:"subscriptionDomain"`
	Domains            []DomainFrontDomain `json:"domains"`
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
	GetRedirect         func() (string, error)
	SubscriptionHost    string

	SubscriptionTarget string
	PanelTarget        string
	PanelHost          string
	PanelPath          string
	SubscriptionPaths  []string
	Routes             map[string][]Route
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
		name, err := netsafe.NormalizeHost(d.Name)
		if err != nil || net.ParseIP(name) != nil || !strings.Contains(name, ".") {
			return fmt.Errorf("a public DNS hostname is required: %s", d.Name)
		}
		if names[d.Name] {
			return fmt.Errorf("duplicate domain: %s", d.Name)
		}
		names[d.Name] = true
		switch d.Mode {
		case "direct", "cdn", "relay", "sub_link_only":
		default:
			return fmt.Errorf("unknown domain mode: %s", d.Mode)
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
		if d.Mode == "sub_link_only" && (d.Panel || len(d.InboundIDs) > 0) {
			return fmt.Errorf("subscription-only domains cannot expose the panel or VPN inbounds")
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
			if d.Enabled && d.Name == c.SubscriptionDomain {
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
