package amneziawg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
)

// UsesNativeOutbound is explicit: existing SOCKS-bridge profiles keep their
// resolver and lifecycle behavior until the operator selects the native path.
func UsesNativeOutbound(raw []byte) bool {
	var v struct {
		Settings struct {
			Native bool `json:"native"`
		} `json:"settings"`
	}
	return json.Unmarshal(raw, &v) == nil && v.Settings.Native
}

// NativeOutbound targets the AWG endpoint shipped in SawaMEN/hiddify-sing-box,
// whose obfuscation options are nested under awg (unlike amnezia-box master).
func NativeOutbound(raw []byte) (map[string]any, error) {
	var wrapper map[string]any
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}
	tag, _ := wrapper["tag"].(string)
	if err := ValidateAmneziaWGOutbound(tag, raw); err != nil {
		return nil, err
	}
	data, _ := outboundSettingsOf(raw)
	var s OutboundSettings
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&s); err != nil {
		return nil, err
	}
	if s.DNS != "" {
		return nil, fmt.Errorf("native AWG uses core DNS: clear the per-bridge DNS field and configure DNS in the core")
	}
	peers := make([]map[string]any, 0, len(s.Peers))
	for _, p := range s.Peers {
		host, port, err := net.SplitHostPort(p.Endpoint)
		if err != nil {
			return nil, err
		}
		n, _ := strconv.Atoi(port)
		peers = append(peers, map[string]any{"address": host, "port": n, "public_key": p.PublicKey,
			"preshared_key": p.PresharedKey, "allowed_ips": p.AllowedIPs, "persistent_keepalive_interval": p.KeepAlive})
	}
	options := map[string]any{
		"jc": s.Jc, "jmin": s.Jmin, "jmax": s.Jmax, "s1": s.S1, "s2": s.S2, "s3": s.S3, "s4": s.S4,
		"h1": s.H1, "h2": s.H2, "h3": s.H3, "h4": s.H4, "i1": s.I1, "i2": s.I2, "i3": s.I3, "i4": s.I4, "i5": s.I5,
		"header_protection_key": s.HeaderProtectionKey, "content_padding_addition": s.ContentPaddingAddition,
		"rekey_after_time": s.RekeyAfterTime, "rekey_timeout": s.RekeyTimeout, "reject_after_time": s.RejectAfterTime,
		"keepalive_timeout": s.KeepaliveTimeout, "max_handshake_attempts": s.MaxHandshakeAttempts,
		"random_trailers": s.RandomTrailers, "disable_cookies": s.DisableCookies,
	}
	wrapper["protocol"] = "singbox:awg"
	wrapper["settings"] = map[string]any{"private_key": s.SecretKey, "address": s.Address,
		"mtu": EffectiveMTU(s.MTU, s.S4), "listen_port": s.ListenPort, "awg": options, "peers": peers}
	return wrapper, nil
}
