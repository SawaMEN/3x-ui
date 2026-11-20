package externalvpn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// PingtunnelClientConfig is a native Pingtunnel 3.x config, not an Xray or
// sing-box outbound. The custom pingtunnel:// envelope is used only by the panel.
type PingtunnelClientConfig struct {
	Type       string `json:"type"`
	Listen     string `json:"listen"`
	Server     string `json:"server"`
	Socks5     bool   `json:"socks5"`
	Password   string `json:"password"`
	Encrypt    string `json:"encrypt"`
	Congestion string `json:"congestion"`
}

// ExportPingtunnelLink publishes the shared-secret client configuration.
// Pingtunnel cannot enforce per-subscriber quotas or disable a single user;
// callers must gate publishing on an explicit client-to-inbound attachment.
func ExportPingtunnelLink(inst Instance, server, remark string) (string, error) {
	if inst.Protocol != model.Pingtunnel {
		return "", fmt.Errorf("not a Pingtunnel inbound")
	}
	if err := inst.Validate(); err != nil {
		return "", err
	}
	server = strings.TrimSpace(strings.Trim(server, "[]"))
	if !validPingtunnelServer(server) {
		return "", fmt.Errorf("Pingtunnel public server must be a valid IPv4 address or hostname without a port")
	}
	encrypt := inst.Settings.Encrypt
	if encrypt == "" {
		encrypt = "chacha20"
	}
	congestion := inst.Settings.Congestion
	if congestion == "" {
		congestion = "bb"
	}
	// Upstream enables BB only when congestion == "bb". The nonempty
	// "none" sentinel survives the JSON loader, which ignores empty values.
	// It therefore disables BB for native JSON clients.
	cfg := PingtunnelClientConfig{
		Type: "client", Listen: "127.0.0.1:1080", Server: server,
		Socks5: true, Password: inst.PingtunnelPassword(),
		Encrypt: encrypt, Congestion: congestion,
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return "pingtunnel://" + base64.RawURLEncoding.EncodeToString(raw) + "#" + url.QueryEscape(remark), nil
}

// DecodePingtunnelLink validates the panel's export envelope without claiming
// that an Xray/sing-box parser can load the embedded native config.
func DecodePingtunnelLink(link string) (PingtunnelClientConfig, error) {
	var cfg PingtunnelClientConfig
	if !strings.HasPrefix(link, "pingtunnel://") {
		return cfg, fmt.Errorf("not a Pingtunnel link")
	}
	payload, _, _ := strings.Cut(strings.TrimPrefix(link, "pingtunnel://"), "#")
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return cfg, fmt.Errorf("Pingtunnel link base64: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("Pingtunnel link JSON: %w", err)
	}
	if cfg.Type != "client" || !cfg.Socks5 || cfg.Listen == "" || len(cfg.Password) < 8 ||
		!validPingtunnelServer(cfg.Server) {
		return cfg, fmt.Errorf("Pingtunnel link missing or invalid native client credentials")
	}
	_, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return cfg, fmt.Errorf("Pingtunnel link has invalid local SOCKS5 listen address: %w", err)
	}
	listenPort, err := strconv.Atoi(port)
	if err != nil || listenPort < 1 || listenPort > 65535 {
		return cfg, fmt.Errorf("Pingtunnel link has invalid local SOCKS5 listen port")
	}
	if cfg.Encrypt != "chacha20" && cfg.Encrypt != "aes128" && cfg.Encrypt != "aes256" {
		return cfg, fmt.Errorf("Pingtunnel link has unsupported cipher")
	}
	if cfg.Congestion != "bb" && cfg.Congestion != "none" {
		return cfg, fmt.Errorf("Pingtunnel link has unsupported congestion control")
	}
	return cfg, nil
}

// Pingtunnel's ICMP socket uses ip4:icmp. Do not publish IPv6 literals or
// non-host strings that its native client cannot use as a server address.
func validPingtunnelServer(server string) bool {
	if server == "" || len(server) > 253 || strings.ContainsAny(server, "/:@?#[]\\ \t\r\n") {
		return false
	}
	if ip := net.ParseIP(server); ip != nil {
		return ip.To4() != nil && !ip.IsUnspecified() && !ip.IsLoopback()
	}
	hostname := strings.TrimSuffix(server, ".")
	if hostname == "" || strings.EqualFold(hostname, "localhost") ||
		strings.HasSuffix(strings.ToLower(hostname), ".localhost") {
		return false
	}
	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') &&
				!(char >= '0' && char <= '9') && char != '-' {
				return false
			}
		}
	}
	return true
}
