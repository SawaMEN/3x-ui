package mtproto

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func ValidDomain(domain string) bool {
	domain = strings.TrimSpace(domain)
	if len(domain) == 0 || len(domain) > 253 || net.ParseIP(domain) != nil {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

// Validate native settings before saving, rather than accepting a config that
// Telemt rejects later. Legacy fields remain compatible with existing imports.
func ValidateSettings(settings string) error {
	var s struct {
		TLSMask              *bool          `json:"tlsMask"`
		TLSEmulation         *bool          `json:"tlsEmulation"`
		LegacyModes          *bool          `json:"allowLegacyModes"`
		ProxyListener        bool           `json:"proxyProtocolListener"`
		RouteThroughCore     bool           `json:"routeThroughXray"`
		Debug                bool           `json:"debug"`
		Domain               string         `json:"fakeTlsDomain"`
		MaskHost             string         `json:"maskHost"`
		MaskPort             int            `json:"maskPort"`
		MaskProxyProtocol    int            `json:"maskProxyProtocol"`
		Trusted              []string       `json:"proxyProtocolTrustedCidrs"`
		PreferIP             string         `json:"preferIp"`
		SNIAction            string         `json:"unknownSniAction"`
		Connections          int            `json:"throttleMaxConnections"`
		Public4              string         `json:"publicIpv4"`
		Public6              string         `json:"publicIpv6"`
		Clients              []model.Client `json:"clients"`
		OutboundTag          string         `json:"outboundTag"`
		FallbackOutboundTags []string       `json:"fallbackOutboundTags"`
	}
	if err := json.Unmarshal([]byte(settings), &s); err != nil {
		return fmt.Errorf("MTProto settings: %w", err)
	}
	if s.Domain != "" && !ValidDomain(s.Domain) {
		return fmt.Errorf("MTProto FakeTLS SNI must be a DNS hostname")
	}
	if s.MaskHost != "" {
		// Brackets are permitted only as one matching pair around an IP
		// literal. strings.Trim(host, "[]") silently accepts malformed
		// values such as "[[::1]]" or "127.0.0.1]".
		maskIP := s.MaskHost
		if strings.HasPrefix(maskIP, "[") && strings.HasSuffix(maskIP, "]") {
			maskIP = maskIP[1 : len(maskIP)-1]
		}
		if !ValidDomain(s.MaskHost) && net.ParseIP(maskIP) == nil {
			return fmt.Errorf("MTProto mask host must be a hostname or IP without a port")
		}
	}
	if s.MaskPort < 0 || s.MaskPort > 65535 {
		return fmt.Errorf("MTProto mask port must be 1–65535 or 0 for automatic")
	}
	if s.MaskProxyProtocol < 0 || s.MaskProxyProtocol > 2 {
		return fmt.Errorf("MTProto mask PROXY protocol must be 0, 1 or 2")
	}
	for _, cidr := range s.Trusted {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("MTProto invalid trusted PROXY CIDR %q", cidr)
		}
	}
	switch s.PreferIP {
	case "", "prefer-ipv4", "prefer-ipv6", "only-ipv4", "only-ipv6":
	default:
		return fmt.Errorf("MTProto invalid IP preference")
	}
	switch s.SNIAction {
	case "", "mask", "drop", "accept", "reject_handshake":
	default:
		return fmt.Errorf("MTProto invalid unknown-SNI policy")
	}
	if s.Connections < 0 {
		return fmt.Errorf("MTProto connection limit cannot be negative")
	}
	if len(s.FallbackOutboundTags) > 3 {
		return fmt.Errorf("MTProto supports at most three fallback outbounds")
	}
	if len(s.FallbackOutboundTags) > 0 && (!s.RouteThroughCore || strings.TrimSpace(s.OutboundTag) == "") {
		return fmt.Errorf("MTProto fallback requires core routing and a primary outbound")
	}
	seen := map[string]bool{strings.TrimSpace(s.OutboundTag): true}
	for _, name := range s.FallbackOutboundTags {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return fmt.Errorf("MTProto fallback outbound must be a distinct non-empty tag")
		}
		seen[name] = true
	}

	for _, client := range s.Clients {
		if err := client.SpeedLimit.Validate(); err != nil {
			return fmt.Errorf("MTProto user %q: %w", client.Email, err)
		}
	}
	if s.Public4 != "" && validAnnounceIP(s.Public4, false) == "" {
		return fmt.Errorf("MTProto invalid public IPv4 address")
	}
	if s.Public6 != "" && validAnnounceIP(s.Public6, true) == "" {
		return fmt.Errorf("MTProto invalid public IPv6 address")
	}
	return nil
}
