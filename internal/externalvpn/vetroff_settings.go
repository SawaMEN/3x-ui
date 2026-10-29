package externalvpn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type VetroffNode struct {
	URL      string `json:"url"`
	Weight   int    `json:"weight"`
	Disabled bool   `json:"disabled"`
	SNI      string `json:"sni"`
	Pin      string `json:"pin"`
}

type VetroffUserPolicy struct {
	Email             string  `json:"email"`
	MaxRequests       int     `json:"maxRequests"`
	MaxTunnels        int     `json:"maxTunnels"`
	RequestsPerSecond float64 `json:"requestsPerSecond"`
	Burst             int     `json:"burst"`
	QuotaBytes        int64   `json:"quotaBytes"`
}

type VetroffSettings struct {
	AutoTLS           bool                `json:"autoTLS"`
	AutoPin           bool                `json:"autoPin"`
	Transport         string              `json:"transport"`
	TLSBackend        string              `json:"tlsBackend"`
	HTTP3             bool                `json:"http3"`
	EnableUDP         bool                `json:"enableUDP"`
	H3Listen          string              `json:"h3Listen"`
	XHTTPPath         string              `json:"xhttpPath"`
	XHTTPHost         string              `json:"xhttpHost"`
	ProtocolPolicy    string              `json:"protocolPolicy"`
	Padding           bool                `json:"padding"`
	MaxTunnels        int                 `json:"maxTunnels"`
	MaxConnections    int                 `json:"maxConnections"`
	WebRoot           string              `json:"webRoot"`
	AllowPrivate      bool                `json:"allowPrivate"`
	EgressProxy       string              `json:"egressProxy"`
	EgressDNS         string              `json:"egressDNS"`
	PublicServer      string              `json:"publicServer"`
	FallbackServers   []string            `json:"fallbackServers"`
	Pin               string              `json:"pin"`
	Balance           string              `json:"balance"`
	Bond              bool                `json:"bond"`
	Discover          bool                `json:"discover"`
	Pipeline          string              `json:"pipeline"`
	ConnectTimeout    string              `json:"connectTimeout"`
	AttemptTimeout    string              `json:"attemptTimeout"`
	BondHub           bool                `json:"bondHub"`
	BondUpstream      string              `json:"bondUpstream"`
	BondCA            string              `json:"bondCA"`
	BondSNI           string              `json:"bondSNI"`
	BondPin           string              `json:"bondPin"`
	Nodes             []VetroffNode       `json:"nodes"`
	AttemptsPerSecond float64             `json:"attemptsPerSecond"`
	AttemptBurst      int                 `json:"attemptBurst"`
	AccessMaxRequests int                 `json:"accessMaxRequests"`
	UserPolicy        VetroffUserPolicy   `json:"userPolicy"`
	UserPolicies      []VetroffUserPolicy `json:"userPolicies"`
}

func DefaultVetroffSettings() VetroffSettings {
	return VetroffSettings{AutoTLS: true, AutoPin: true, Transport: "h2-connect", TLSBackend: "auto", HTTP3: true, EnableUDP: true, ProtocolPolicy: "legacy", MaxTunnels: 256, MaxConnections: 512, Balance: "failover", Pipeline: "auto", ConnectTimeout: "15s", AttemptTimeout: "5s", FallbackServers: []string{}, Nodes: []VetroffNode{}, AttemptsPerSecond: 200, AttemptBurst: 400, AccessMaxRequests: 1024, UserPolicy: VetroffUserPolicy{MaxRequests: 128, MaxTunnels: 32, RequestsPerSecond: 100, Burst: 200}, UserPolicies: []VetroffUserPolicy{}}
}

func (v *VetroffSettings) UnmarshalJSON(data []byte) error {
	type plain VetroffSettings
	value := plain(DefaultVetroffSettings())
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*v = VetroffSettings(value)
	return nil
}

func vetroffOptions(inst Instance) VetroffSettings {
	if inst.Settings.Vetroff != nil {
		return *inst.Settings.Vetroff
	}
	value := DefaultVetroffSettings()
	value.AutoTLS, value.AutoPin = false, false
	return value
}

func vetroffOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.HasSuffix(u.Host, ":") || strings.ContainsAny(u.Host, " \x00\r\n\t") {
		return "", fmt.Errorf("VetroffProxy requires root HTTPS server URLs")
	}
	if u.Port() != "" {
		if ap, err := netip.ParseAddrPort(net.JoinHostPort("127.0.0.1", u.Port())); err != nil || ap.Port() == 0 {
			return "", fmt.Errorf("invalid VetroffProxy server port")
		}
	}
	host := strings.ToLower(u.Hostname())
	if ip, err := netip.ParseAddr(host); err == nil {
		host = ip.String()
	}
	port := 443
	if u.Port() != "" {
		port, _ = strconv.Atoi(u.Port())
	}
	u.Host = net.JoinHostPort(host, strconv.Itoa(port))
	u.Path = ""
	return u.String(), nil
}

func vetroffSNI(name string) bool {
	if name == "" || len(name) > 253 || net.ParseIP(name) != nil {
		return false
	}
	for _, label := range strings.Split(strings.ToLower(name), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}

func vetroffServers(primary string, fallback []string) ([]string, error) {
	servers := make([]string, 0, len(fallback)+1)
	seen := map[string]bool{}
	for _, raw := range append([]string{primary}, fallback...) {
		normalized, err := vetroffOrigin(raw)
		if err != nil {
			return nil, err
		}
		if seen[normalized] {
			return nil, fmt.Errorf("duplicate VetroffProxy server")
		}
		seen[normalized] = true
		servers = append(servers, raw)
	}
	return servers, nil
}

func vetroffPin(pin string) bool {
	if pin == "" {
		return true
	}
	if !strings.HasPrefix(pin, "sha256/") {
		return false
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(pin, "sha256/"))
	return err == nil && len(raw) == 32
}

func validateVetroffOptions(inst Instance) error {
	v := vetroffOptions(inst)
	if v.ProtocolPolicy != "legacy" && v.ProtocolPolicy != "strict" {
		return fmt.Errorf("invalid VetroffProxy protocol policy")
	}
	switch v.Transport {
	case "h2-connect", "h2-post", "wss", "h3-post", "h3-connect", "xhttp-stream-one", "xhttp-stream-up", "xhttp-packet-up":
	default:
		return fmt.Errorf("invalid VetroffProxy transport")
	}
	if v.TLSBackend != "auto" && v.TLSBackend != "go" && v.TLSBackend != "nss" {
		return fmt.Errorf("invalid VetroffProxy TLS backend")
	}
	if strings.HasPrefix(v.Transport, "h3-") && (!v.HTTP3 || v.TLSBackend == "nss") {
		return fmt.Errorf("HTTP/3 requires its listener and auto/Go TLS")
	}
	if v.H3Listen != "" {
		host, port, err := net.SplitHostPort(v.H3Listen)
		if err != nil || net.ParseIP(host) == nil {
			return fmt.Errorf("invalid HTTP/3 listen IP:port")
		}
		if ap, err := netip.ParseAddrPort(net.JoinHostPort(host, port)); err != nil || ap.Port() == 0 {
			return fmt.Errorf("invalid HTTP/3 port")
		}
	}
	if v.MaxTunnels < 1 || v.MaxTunnels > 65536 || v.MaxConnections < 1 || v.MaxConnections > 65536 {
		return fmt.Errorf("VetroffProxy connection limits must be 1..65536")
	}
	for _, path := range []string{v.WebRoot, v.BondCA} {
		if path != "" && !filepath.IsAbs(path) {
			return fmt.Errorf("VetroffProxy file paths must be absolute")
		}
	}
	if v.XHTTPPath != "" {
		path := strings.TrimSuffix(v.XHTTPPath, "/")
		if len(v.XHTTPPath) > 256 || !strings.HasPrefix(path, "/") || path == "" {
			return fmt.Errorf("invalid XHTTP path")
		}
		for _, part := range strings.Split(path[1:], "/") {
			if part == "" || part == "." || part == ".." {
				return fmt.Errorf("invalid XHTTP path")
			}
			for _, c := range part {
				if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune("-_.~", c) {
					return fmt.Errorf("invalid XHTTP path")
				}
			}
		}
	}
	if v.XHTTPHost != "" {
		if len(v.XHTTPHost) > 259 || strings.ContainsAny(v.XHTTPHost, " \r\n\t\\%") {
			return fmt.Errorf("invalid XHTTP Host")
		}
		if _, err := vetroffOrigin("https://" + v.XHTTPHost); err != nil {
			return err
		}
	}
	if !vetroffPin(v.Pin) || !vetroffPin(v.BondPin) {
		return fmt.Errorf("invalid VetroffProxy SPKI pin")
	}
	if v.BondSNI != "" && !vetroffSNI(v.BondSNI) {
		return fmt.Errorf("invalid Bond SNI")
	}
	for _, row := range []struct {
		value string
		max   time.Duration
	}{{v.ConnectTimeout, 2 * time.Minute}, {v.AttemptTimeout, 30 * time.Second}} {
		d, err := time.ParseDuration(row.value)
		if err != nil || d <= 0 || d > row.max {
			return fmt.Errorf("invalid VetroffProxy timeout")
		}
		if v.Bond && row.value == v.AttemptTimeout && d < 100*time.Millisecond {
			return fmt.Errorf("Bond attempt timeout must be at least 100ms")
		}
	}
	if v.Balance != "failover" && v.Balance != "round-robin" && v.Balance != "least-active" && v.Balance != "adaptive" {
		return fmt.Errorf("invalid VetroffProxy balance mode")
	}
	if v.Pipeline != "auto" && v.Pipeline != "4" && v.Pipeline != "8" {
		return fmt.Errorf("invalid VetroffProxy pipeline")
	}
	if len(v.FallbackServers) > 7 || len(v.Nodes) > 8 {
		return fmt.Errorf("VetroffProxy supports up to eight servers")
	}
	if v.Discover && (!v.Bond || !v.BondHub || len(v.FallbackServers) > 0) || v.Bond && (v.Padding || v.Balance == "adaptive" || !v.Discover && len(v.FallbackServers) == 0) {
		return fmt.Errorf("invalid VetroffProxy Bond/discovery combination")
	}
	if v.BondHub && v.BondUpstream != "" || v.ProtocolPolicy == "strict" && (v.BondHub || v.BondUpstream != "" || v.Bond || v.Discover) {
		return fmt.Errorf("strict policy and Bond/discovery cannot be combined")
	}
	primary := v.PublicServer
	if primary == "" {
		primary = "https://" + net.JoinHostPort(inst.Settings.Hostname, strconv.Itoa(inst.Port))
	}
	if _, err := vetroffServers(primary, v.FallbackServers); err != nil {
		return err
	}
	if v.BondUpstream != "" {
		if _, err := vetroffOrigin(v.BondUpstream); err != nil {
			return err
		}
	}
	active := 0
	seen := map[string]bool{}
	for _, n := range v.Nodes {
		origin, err := vetroffOrigin(n.URL)
		if err != nil {
			return err
		}
		if seen[strings.ToLower(origin)] || n.Weight < 1 || n.Weight > 100 || !vetroffPin(n.Pin) || n.SNI != "" && !vetroffSNI(n.SNI) {
			return fmt.Errorf("invalid VetroffProxy cluster node")
		}
		seen[strings.ToLower(origin)] = true
		if !n.Disabled {
			active++
		}
	}
	if len(v.Nodes) > 0 && (!v.BondHub || active == 0) {
		return fmt.Errorf("cluster nodes require a hub and at least one active node")
	}
	if v.EgressProxy == "" {
		if v.EgressDNS != "" {
			return fmt.Errorf("egress DNS requires a SOCKS5 proxy")
		}
	} else {
		u, err := url.Parse(v.EgressProxy)
		if err != nil || u.Scheme != "socks5" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("invalid SOCKS5 egress")
		}
		if ap, err := netip.ParseAddrPort(u.Host); err != nil || ap.Port() == 0 || ap.Addr().IsUnspecified() || ap.Addr().IsMulticast() || ap.Addr().Zone() != "" {
			return fmt.Errorf("SOCKS5 egress requires a numeric IP:port")
		}
		if u.User != nil {
			password, ok := u.User.Password()
			if !ok || u.User.Username() == "" || len(u.User.Username()) > 255 || password == "" || len(password) > 255 {
				return fmt.Errorf("invalid SOCKS5 credentials")
			}
		}
		if v.EgressDNS != "" {
			if ap, err := netip.ParseAddrPort(v.EgressDNS); err != nil || ap.Port() == 0 || ap.Addr().IsUnspecified() || ap.Addr().IsMulticast() || ap.Addr().Zone() != "" {
				return fmt.Errorf("egress DNS requires a numeric IP:port")
			}
		}
	}
	if v.AttemptsPerSecond < 0.1 || v.AttemptsPerSecond > 10000 || v.AttemptBurst < 1 || v.AttemptBurst > 20000 || v.AccessMaxRequests < 1 || v.AccessMaxRequests > 65536 {
		return fmt.Errorf("invalid VetroffProxy access limits")
	}
	clients := map[string]bool{}
	for _, c := range inst.Settings.Clients {
		clients[c.Email] = true
	}
	overrides := map[string]bool{}
	for i, p := range append([]VetroffUserPolicy{v.UserPolicy}, v.UserPolicies...) {
		if p.MaxRequests < 1 || p.MaxRequests > 4096 || p.MaxTunnels < 1 || p.MaxTunnels > 4096 || p.RequestsPerSecond < 0.1 || p.RequestsPerSecond > 10000 || p.Burst < 1 || p.Burst > 20000 || p.QuotaBytes < 0 || p.QuotaBytes > 1<<60 || v.BondUpstream != "" && p.QuotaBytes > 0 {
			return fmt.Errorf("invalid VetroffProxy user access policy")
		}
		if i > 0 {
			if !clients[p.Email] || overrides[p.Email] {
				return fmt.Errorf("user policy must name one unique inbound client")
			}
			overrides[p.Email] = true
		}
	}
	return nil
}
