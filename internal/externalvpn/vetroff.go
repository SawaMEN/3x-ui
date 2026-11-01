package externalvpn

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func validateVetroff(inst Instance) error {
	s := inst.Settings
	if inst.Port < 1 || inst.Port > 65535 || (inst.Listen != "" && net.ParseIP(strings.Trim(inst.Listen, "[]")) == nil) {
		return fmt.Errorf("VetroffProxy requires an IP listen address and port 1..65535")
	}
	if !vetroffSNI(s.Hostname) || !filepath.IsAbs(s.Certificate) || !filepath.IsAbs(s.PrivateKey) {
		return fmt.Errorf("VetroffProxy requires a hostname and absolute TLS certificate/private key paths")
	}
	if len(s.Clients) > 1024 {
		return fmt.Errorf("VetroffProxy supports at most 1024 users")
	}
	seen := map[string]bool{}
	for _, c := range s.Clients {
		if c.Email == "" || seen[c.Email] || len(c.Email) > 256 || strings.ContainsAny(c.Email, ":\r\n\x00") || len(c.Password) < 24 || len(c.Password) > 1024 || strings.ContainsAny(c.Password, "\r\n\x00") {
			return fmt.Errorf("VetroffProxy requires unique usernames and passwords of 24..1024 bytes")
		}
		seen[c.Email] = true
	}
	return validateVetroffOptions(inst)
}

func vetroffFiles(inst Instance) ([]string, error) {
	folder := filepath.Join(directory(), strconv.Itoa(inst.ID))
	cert, key, err := certificate(inst, folder)
	if err != nil {
		return nil, err
	}
	v := vetroffOptions(inst)
	users := make([]map[string]any, 0, len(inst.Settings.Clients))
	for _, c := range inst.Settings.Clients {
		policy := v.UserPolicy
		for _, override := range v.UserPolicies {
			if override.Email == c.Email {
				policy = override
				break
			}
		}
		users = append(users, map[string]any{"id": FPTNUsername(c.Email), "username": FPTNUsername(c.Email), "password": c.Password, "disabled": !c.Enable || c.ExpiryTime > 0 && c.ExpiryTime <= time.Now().UnixMilli(), "max_requests": policy.MaxRequests, "max_tunnels": policy.MaxTunnels, "requests_per_second": policy.RequestsPerSecond, "burst": policy.Burst, "quota_bytes": policy.QuotaBytes})
	}
	usersPath := filepath.Join(folder, "vetroff-users.json")
	raw, err := json.Marshal(map[string]any{"v": 1, "users": users, "attempts_per_second": v.AttemptsPerSecond, "attempt_burst": v.AttemptBurst, "max_requests": v.AccessMaxRequests})
	if err != nil {
		return nil, err
	}
	if err := writePrivate(usersPath, raw); err != nil {
		return nil, err
	}
	conf := map[string]any{"v": 1, "listen": inst.Bind(), "enable_udp": v.EnableUDP, "cert": cert, "key": key, "users_file": usersPath, "users_state": filepath.Join(folder, "vetroff-usage.json"), "max_tunnels": v.MaxTunnels, "max_connections": v.MaxConnections, "protocol_policy": v.ProtocolPolicy}
	if v.HTTP3 {
		conf["h3_listen"] = v.H3Listen
		if v.H3Listen == "" {
			conf["h3_listen"] = inst.Bind()
		}
	}
	for k, value := range map[string]string{"xhttp_path": v.XHTTPPath, "xhttp_host": v.XHTTPHost, "web_root": v.WebRoot, "egress_proxy": v.EgressProxy, "egress_dns": v.EgressDNS, "bond_upstream": v.BondUpstream, "bond_ca": v.BondCA, "bond_sni": v.BondSNI, "bond_pin_sha256": v.BondPin} {
		if value != "" {
			conf[k] = value
		}
	}
	if v.BondHub {
		conf["bond_hub"] = true
		nodes := v.Nodes
		if len(nodes) == 0 {
			origin := v.PublicServer
			if origin == "" {
				origin = "https://" + net.JoinHostPort(inst.Settings.Hostname, strconv.Itoa(inst.Port))
			}
			nodes = []VetroffNode{{URL: origin, Weight: 100}}
			for _, fallback := range v.FallbackServers {
				nodes = append(nodes, VetroffNode{URL: fallback, Weight: 100})
			}
		}
		cluster := make([]map[string]any, 0, len(nodes))
		for _, n := range nodes {
			row := map[string]any{"url": n.URL, "weight": n.Weight, "disabled": n.Disabled}
			if n.SNI != "" {
				row["sni"] = n.SNI
			}
			if n.Pin != "" {
				row["pin_sha256"] = n.Pin
			}
			cluster = append(cluster, row)
		}
		clusterPath := filepath.Join(folder, "vetroff-cluster.json")
		data, err := json.Marshal(map[string]any{"v": 1, "nodes": cluster})
		if err != nil {
			return nil, err
		}
		if err := writePrivate(clusterPath, data); err != nil {
			return nil, err
		}
		conf["cluster_file"] = clusterPath
	}
	path := filepath.Join(folder, "vetroff-server.json")
	raw, err = json.Marshal(conf)
	if err != nil {
		return nil, err
	}
	if err := writePrivate(path, raw); err != nil {
		return nil, err
	}
	if !v.Padding && !v.AllowPrivate {
		return []string{"server", "--config", path}, nil
	}
	args := []string{"server"}
	for _, name := range []string{"listen", "h3_listen", "cert", "key", "users_file", "users_state", "max_tunnels", "max_connections", "protocol_policy", "xhttp_path", "xhttp_host", "web_root", "egress_proxy", "egress_dns", "bond_upstream", "bond_ca", "bond_sni", "bond_pin_sha256", "cluster_file"} {
		if value, ok := conf[name]; ok {
			flag := strings.ReplaceAll(name, "_", "-")
			if name == "bond_ca" {
				flag = "bond-ca-cert"
			}
			args = append(args, "--"+flag, fmt.Sprint(value))
		}
	}
	for name, enabled := range map[string]bool{"enable-udp": v.EnableUDP, "bond-hub": v.BondHub, "padding": v.Padding, "allow-private": v.AllowPrivate} {
		if enabled {
			args = append(args, "--"+name)
		}
	}
	return args, nil
}

func exportVetroffLink(inst Instance, email, address string, port int) (string, error) {
	if inst.Protocol != model.VetroffProxy {
		return "", fmt.Errorf("not a VetroffProxy inbound")
	}
	if err := inst.Validate(); err != nil {
		return "", err
	}
	address = strings.Trim(address, "[]")
	if address == "" || port < 1 || port > 65535 || strings.ContainsAny(address, "/?#@\r\n") {
		return "", fmt.Errorf("invalid VetroffProxy endpoint")
	}
	if net.ParseIP(address) == nil && !domainPattern.MatchString(address) {
		return "", fmt.Errorf("invalid VetroffProxy endpoint hostname")
	}
	if ip := net.ParseIP(address); ip != nil && ip.IsUnspecified() {
		return "", fmt.Errorf("unspecified endpoint")
	}
	for _, c := range inst.Settings.Clients {
		if c.Enable && c.Email == email {
			if c.ExpiryTime > 0 && c.ExpiryTime <= time.Now().UnixMilli() {
				return "", fmt.Errorf("VetroffProxy client expired")
			}
			v := vetroffOptions(inst)
			if strings.HasPrefix(v.Transport, "h3-") && v.H3Listen != "" && port == inst.Port {
				_, udpPort, err := net.SplitHostPort(v.H3Listen)
				if err != nil {
					return "", err
				}
				port, err = strconv.Atoi(udpPort)
				if err != nil {
					return "", err
				}
			}
			origin := (&url.URL{Scheme: "https", Host: net.JoinHostPort(address, strconv.Itoa(port))}).String()
			if v.PublicServer != "" {
				origin = v.PublicServer
			}
			pin := v.Pin
			if v.AutoPin && pin == "" {
				var err error
				pin, err = vetroffCertificatePin(inst.Settings.Certificate)
				if err != nil {
					return "", err
				}
			}
			servers, err := vetroffServers(origin, v.FallbackServers)
			if err != nil {
				return "", err
			}
			profile := map[string]any{"v": 1, "servers": servers, "username": FPTNUsername(c.Email), "password": c.Password, "sni": inst.Settings.Hostname, "transport": v.Transport, "tls": v.TLSBackend, "encryption": "tls13", "balance": v.Balance, "bond": v.Bond, "discover": v.Discover, "pipeline": v.Pipeline, "padding": v.Padding, "connect_timeout": v.ConnectTimeout, "attempt_timeout": v.AttemptTimeout}
			if v.ProtocolPolicy == "strict" {
				profile["protocol_policy"] = "strict"
			}
			if pin != "" {
				profile["pin_sha256"] = pin
			}
			if strings.HasPrefix(v.Transport, "xhttp-") {
				if v.XHTTPPath != "" {
					profile["xhttp_path"] = v.XHTTPPath
				}
				if v.XHTTPHost != "" {
					profile["xhttp_host"] = v.XHTTPHost
				}
			}
			raw, err := json.Marshal(profile)
			if err != nil {
				return "", err
			}
			link := "vetroff://" + base64.RawURLEncoding.EncodeToString(raw)
			if len(link) > 16<<10 {
				return "", fmt.Errorf("VetroffProxy connection profile exceeds 16 KiB")
			}
			return link, nil
		}
	}
	return "", fmt.Errorf("VetroffProxy client disabled or absent")
}

func validateVetroffPanelLimits(ib *model.Inbound) error {
	var raw struct {
		Clients []model.Client `json:"clients"`
	}
	if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil {
		return err
	}
	if ib.Total > 0 {
		return fmt.Errorf("VetroffProxy panel traffic quotas are not supported")
	}
	for _, c := range raw.Clients {
		if c.TotalGB > 0 || c.LimitIP > 0 || c.ExpiryTime < 0 || (c.SpeedLimit != nil && c.SpeedLimit.Enabled) {
			return fmt.Errorf("VetroffProxy does not support panel traffic, IP, speed or first-use duration limits")
		}
	}
	return nil
}

func vetroffCertificatePin(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read VetroffProxy certificate: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return "", fmt.Errorf("invalid VetroffProxy certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return "sha256/" + base64.StdEncoding.EncodeToString(sum[:]), nil
}
