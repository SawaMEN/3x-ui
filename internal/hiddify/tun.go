package hiddify

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

var tunInterfaceName = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,15}$`)

func translateTun(raw map[string]any) (map[string]any, error) {
	settings := object(raw, "settings")
	for key, value := range settings {
		switch key {
		case "name", "mtu", "gateway", "dns", "userLevel", "autoSystemRoutingTable", "autoOutboundsInterface":
		case "clients":
			if meaningful(value) {
				return nil, fmt.Errorf("TUN does not authenticate clients")
			}
		default:
			if meaningful(value) {
				return nil, fmt.Errorf("TUN setting %q has no native mapping", key)
			}
		}
	}
	if meaningful(settings["userLevel"]) {
		return nil, fmt.Errorf("TUN userLevel has no native mapping")
	}
	for key, value := range object(raw, "streamSettings") {
		if key == "network" && (value == "tcp" || value == "") {
			continue
		}
		if key == "security" && (value == "none" || value == "") {
			continue
		}
		if meaningful(value) {
			return nil, fmt.Errorf("TUN stream setting %q has no native mapping", key)
		}
	}
	name, _ := settings["name"].(string)
	if name == "" {
		name = "xray0"
	}
	if !tunInterfaceName.MatchString(name) {
		return nil, fmt.Errorf("invalid TUN interface name %q", name)
	}
	out := map[string]any{"type": "tun", "tag": raw["tag"], "interface_name": name, "stack": "system", "dns_mode": "native"}
	if value, exists := settings["mtu"]; exists {
		n, ok := value.(float64)
		if !ok || n < 0 || n > 65535 || n != float64(int(n)) {
			return nil, fmt.Errorf("invalid TUN MTU")
		}
		if n > 0 {
			out["mtu"] = int(n)
		}
	}
	for _, field := range []struct {
		source, target string
		prefix         bool
	}{{"gateway", "address", true}, {"autoSystemRoutingTable", "route_address", true}, {"dns", "dns_address", false}} {
		value := settings[field.source]
		if value == nil {
			continue
		}
		entries, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("TUN %s must be an array", field.source)
		}
		addresses := make([]string, 0, len(entries))
		for _, entry := range entries {
			text, ok := entry.(string)
			if !ok {
				return nil, fmt.Errorf("TUN %s must contain addresses", field.source)
			}
			if field.prefix {
				if _, err := netip.ParsePrefix(text); err != nil {
					return nil, fmt.Errorf("TUN %s: %w", field.source, err)
				}
			} else {
				if _, err := netip.ParseAddr(text); err != nil {
					return nil, fmt.Errorf("TUN DNS: %w", err)
				}
			}
			addresses = append(addresses, text)
		}
		if len(addresses) > 0 {
			out[field.target] = addresses
		}
	}
	if out["address"] == nil {
		return nil, fmt.Errorf("TUN gateway requires at least one interface CIDR")
	}
	if out["route_address"] != nil {
		out["auto_route"] = true
	}
	iface, err := TunOutboundInterface(settings)
	if err != nil {
		return nil, err
	}
	if iface == name {
		return nil, fmt.Errorf("TUN outbound interface must differ from its virtual interface")
	}
	return out, nil
}

// Route options apply to the complete configuration, not the TUN listener.
func TunOutboundInterface(settings map[string]any) (string, error) {
	value := settings["autoOutboundsInterface"]
	if value == nil {
		return "auto", nil
	}
	iface, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("TUN outbound interface must be a string")
	}
	iface = strings.TrimSpace(iface)
	if iface == "" || iface == "auto" {
		return "auto", nil
	}
	if !tunInterfaceName.MatchString(iface) {
		return "", fmt.Errorf("invalid TUN outbound interface %q", iface)
	}
	return iface, nil
}
