package externalvpn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
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
	if server == "" || strings.ContainsAny(server, "/:@?# \\t\\r\\n") {
		return "", fmt.Errorf("Pingtunnel public server must be a hostname or IP without a port")
	}
	if ip := net.ParseIP(server); ip != nil && ip.IsUnspecified() {
		return "", fmt.Errorf("cannot publish an unspecified Pingtunnel address")
	}
	encrypt := inst.Settings.Encrypt
	if encrypt == "" {
		encrypt = "chacha20"
	}
	congestion := inst.Settings.Congestion
	if congestion == "" {
		congestion = "bb"
	}
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
	if cfg.Type != "client" || !cfg.Socks5 || cfg.Listen == "" || cfg.Server == "" || len(cfg.Password) < 8 {
		return cfg, fmt.Errorf("Pingtunnel link missing native client credentials")
	}
	if cfg.Encrypt != "chacha20" && cfg.Encrypt != "aes128" && cfg.Encrypt != "aes256" {
		return cfg, fmt.Errorf("Pingtunnel link has unsupported cipher")
	}
	return cfg, nil
}
