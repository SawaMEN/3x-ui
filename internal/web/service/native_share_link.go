package service

import (
	"fmt"
	"strconv"
	"strings"

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
