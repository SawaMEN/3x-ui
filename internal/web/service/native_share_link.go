package service

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	sharelink "github.com/SawaMEN/3x-ui/v3/internal/util/link"

	"github.com/enfein/mieru/v3/pkg/appctl"
)

// ParseNativeMieruShareLink decodes a profile without applying it to the host.
// A native outbound represents one server; reject lossy conversions.
func ParseNativeMieruShareLink(link string) (map[string]any, error) {
	cfg, err := appctl.ParseURLClientConfig(strings.TrimSpace(link))
	if err != nil {
		return nil, err
	}
	if len(cfg.GetProfiles()) != 1 {
		return nil, fmt.Errorf("Mieru import requires exactly one profile")
	}
	p := cfg.GetProfiles()[0]
	if len(p.GetServers()) != 1 {
		return nil, fmt.Errorf("Mieru import requires exactly one server; use a mierus link for that server")
	}
	if p.GetDialer() != nil {
		return nil, fmt.Errorf("Mieru proxy dialer must be configured explicitly as a sing-box detour")
	}
	if p.GetTrafficPattern() != nil {
		return nil, fmt.Errorf("traffic-pattern is not supported by the pinned sing-box core")
	}
	user := p.GetUser()
	if user.GetName() == "" || user.GetPassword() == "" || user.GetHashedPassword() != "" || len(user.GetName()) > 64 || len(user.GetPassword()) > 64 {
		return nil, fmt.Errorf("Mieru requires a plain user name and password of 1–64 bytes")
	}
	server := p.GetServers()[0]
	host := server.GetDomainName()
	if host == "" {
		host = server.GetIpAddress()
	}
	if host == "" {
		return nil, fmt.Errorf("Mieru server is empty")
	}
	bindings := []map[string]any{}
	for _, b := range server.GetPortBindings() {
		protocol := b.GetProtocol().String()
		if protocol != "TCP" && protocol != "UDP" {
			return nil, fmt.Errorf("invalid Mieru transport")
		}
		item := map[string]any{"protocol": protocol}
		if r := b.GetPortRange(); r != "" {
			parts := strings.Split(r, "-")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid Mieru port range")
			}
			first, e1 := strconv.Atoi(parts[0])
			last, e2 := strconv.Atoi(parts[1])
			if e1 != nil || e2 != nil || first < 1 || last > 65535 || first > last || b.GetPort() != 0 {
				return nil, fmt.Errorf("invalid Mieru port range")
			}
			item["portRange"] = r
		} else {
			if b.GetPort() < 1 || b.GetPort() > 65535 {
				return nil, fmt.Errorf("invalid Mieru port")
			}
			item["port"] = b.GetPort()
		}
		bindings = append(bindings, item)
	}
	if len(bindings) == 0 {
		return nil, fmt.Errorf("Mieru port bindings are empty")
	}
	mtu := p.GetMtu()
	if mtu != 0 && (mtu < 1280 || mtu > 1500) {
		return nil, fmt.Errorf("Mieru MTU must be 1280–1500")
	}
	mux := strings.TrimPrefix(p.GetMultiplexing().GetLevel().String(), "MULTIPLEXING_")
	if mux != "DEFAULT" && mux != "LOW" && mux != "MIDDLE" && mux != "HIGH" {
		return nil, fmt.Errorf("Mieru multiplexing %s is not supported by the pinned sing-box core", mux)
	}
	handshake := p.GetHandshakeMode().String()
	if handshake != "HANDSHAKE_DEFAULT" && handshake != "HANDSHAKE_STANDARD" && handshake != "HANDSHAKE_NO_WAIT" {
		return nil, fmt.Errorf("invalid Mieru handshake mode")
	}
	tag := p.GetProfileName()
	if tag == "" {
		tag = host
	}
	return map[string]any{"type": "mieru", "tag": tag, "server": host, "username": user.GetName(), "password": user.GetPassword(), "portBindings": bindings, "mtu": mtu, "multiplexing": mux, "handshake_mode": strings.TrimPrefix(handshake, "HANDSHAKE_")}, nil
}

// ParseNativeOutboundShareLink reuses the subscription parser and the runtime
// translator so transport/security options have the same meaning in both UIs.
func parseLegacyNativeOutboundShareLink(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 64<<10 {
		return nil, fmt.Errorf("connection link too large")
	}
	if strings.HasPrefix(raw, "mieru://") || strings.HasPrefix(raw, "mierus://") {
		return ParseNativeMieruShareLink(raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid connection link: %w", err)
	}
	switch u.Scheme {
	case "anytls", "tuic", "hysteria", "shadowtls", "ssh", "naive+https":
		if u.Hostname() == "" || u.User == nil {
			return nil, fmt.Errorf("server and credentials are required")
		}
		port := 443
		if u.Scheme == "ssh" {
			port = 22
		}
		if u.Port() != "" {
			port, err = strconv.Atoi(u.Port())
			if err != nil || port < 1 || port > 65535 {
				return nil, fmt.Errorf("invalid server port")
			}
		}
		kind := u.Scheme
		if kind == "naive+https" {
			kind = "naive"
		}
		tag := u.Fragment
		if tag == "" {
			tag = u.Hostname()
		}
		out := map[string]any{"type": kind, "tag": tag, "server": u.Hostname(), "server_port": port}
		q := u.Query()
		allowed := map[string]bool{"sni": true, "alpn": true, "insecure": true, "allowInsecure": true, "security": true}
		switch kind {
		case "tuic":
			for _, key := range []string{"congestion_control", "congestion-control", "udp_relay_mode", "udp-relay-mode"} {
				allowed[key] = true
			}
		case "anytls":
			for _, key := range []string{"fp", "idle_session_timeout", "idle_session_check_interval"} {
				allowed[key] = true
			}
		case "shadowtls":
			allowed["version"] = true
			allowed["fp"] = true
		case "hysteria":
			for _, key := range []string{"auth", "up_mbps", "down_mbps", "upmbps", "downmbps", "peer"} {
				allowed[key] = true
			}
		}
		for key := range q {
			if !allowed[key] {
				return nil, fmt.Errorf("unsupported %s link option %q; configure it in JSON", kind, key)
			}
		}
		if q.Get("security") != "" && q.Get("security") != "tls" {
			return nil, fmt.Errorf("%s link requires TLS", kind)
		}
		password, _ := u.User.Password()
		switch kind {
		case "tuic":
			out["uuid"] = u.User.Username()
			out["password"] = password
			for _, key := range []string{"congestion_control", "udp_relay_mode"} {
				if q.Get(key) != "" {
					out[key] = q.Get(key)
				} else if q.Get(strings.ReplaceAll(key, "_", "-")) != "" {
					out[key] = q.Get(strings.ReplaceAll(key, "_", "-"))
				}
			}
		case "ssh":
			out["user"] = u.User.Username()
			out["password"] = password
		case "naive":
			out["username"] = u.User.Username()
			out["password"] = password
		case "hysteria":
			out["auth_str"] = u.User.Username()
			if q.Get("auth") != "" {
				out["auth_str"] = q.Get("auth")
			}
			for _, key := range []string{"up_mbps", "down_mbps"} {
				speed := q.Get(key)
				if speed == "" {
					speed = q.Get(strings.ReplaceAll(key, "_", ""))
				}
				n, e := strconv.Atoi(speed)
				if e != nil || n <= 0 {
					return nil, fmt.Errorf("Hysteria requires positive %s", key)
				}
				out[key] = n
			}
		default:
			out["password"] = u.User.Username()
			if password != "" {
				out["password"] = u.User.Username() + ":" + password
			}
			if kind == "shadowtls" {
				out["version"] = 3
				if q.Get("version") != "" {
					n, e := strconv.Atoi(q.Get("version"))
					if e != nil {
						return nil, fmt.Errorf("invalid ShadowTLS version")
					}
					out["version"] = n
				}
			}
		}
		if kind != "ssh" {
			sni := q.Get("sni")
			if sni == "" {
				sni = q.Get("peer")
				if sni == "" {
					sni = u.Hostname()
				}
			}
			tls := map[string]any{"enabled": true, "server_name": sni}
			if q.Get("alpn") != "" {
				tls["alpn"] = strings.Split(q.Get("alpn"), ",")
			}
			if q.Get("insecure") == "1" || q.Get("insecure") == "true" || q.Get("allowInsecure") == "1" || q.Get("allowInsecure") == "true" {
				tls["insecure"] = true
			}
			if q.Get("fp") != "" {
				tls["utls"] = map[string]any{"enabled": true, "fingerprint": q.Get("fp")}
			}
			out["tls"] = tls
		}
		if kind == "anytls" {
			for _, key := range []string{"idle_session_timeout", "idle_session_check_interval"} {
				if q.Get(key) != "" {
					out[key] = q.Get(key)
				}
			}
		}
		return singbox.TranslateXrayOutbound(map[string]any{"protocol": "singbox:" + kind, "tag": tag, "settings": out})
	}
	parsed, err := sharelink.ParseLink(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Outbound["protocol"] == "wireguard" || parsed.Outbound["protocol"] == "amneziawg" {
		return nil, fmt.Errorf("WireGuard and AmneziaWG must be imported as endpoints, not outbounds")
	}
	if tag, _ := parsed.Outbound["tag"].(string); strings.TrimSpace(tag) == "" {
		parsed.Outbound["tag"] = fmt.Sprint(parsed.Outbound["protocol"]) + "-proxy"
	}
	return singbox.TranslateXrayOutbound(map[string]any(parsed.Outbound))
}
