package service

import (
	"encoding/base64"
	"fmt"
	"net/netip"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// WireGuard terminates locally before the ordinary route chooses an outbound.
// This also supplies the WireGuard target used by VK TURN forwarding.
func singBoxWireGuardInbound(ib *model.Inbound, clients []model.Client) (map[string]any, error) {
	settings, peers, err := (&InboundService{}).getWireguardSettings(ib.Settings)
	if err != nil {
		return nil, err
	}
	key, _ := settings["secretKey"].(string)
	if decoded, err := base64.StdEncoding.DecodeString(key); err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("WireGuard %q has an invalid private key", ib.Tag)
	}
	if ib.Port < 1 || ib.Port > 65535 {
		return nil, fmt.Errorf("WireGuard %q has an invalid listen port", ib.Tag)
	}
	if listen := strings.TrimSpace(ib.Listen); listen != "" && listen != "0.0.0.0" && listen != "::" {
		return nil, fmt.Errorf("WireGuard %q: native endpoint requires an all-interface listen address", ib.Tag)
	}
	disabled := map[string]bool{}
	for _, client := range clients {
		if !client.Enable {
			disabled[client.Email] = true
		}
	}
	for _, row := range ib.ClientStats {
		if !row.Enable {
			disabled[row.Email] = true
		}
	}
	var native []map[string]any
	var allowed []string
	for _, peer := range peers {
		if peer.raw["enable"] == false || disabled[peer.Email] {
			continue
		}
		if decoded, err := base64.StdEncoding.DecodeString(peer.PublicKey); err != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("WireGuard %q has an invalid peer key", ib.Tag)
		}
		for _, cidr := range peer.AllowedIPs {
			if _, err := netip.ParsePrefix(cidr); err != nil {
				return nil, fmt.Errorf("WireGuard %q has invalid peer address %q", ib.Tag, cidr)
			}
		}
		if len(peer.AllowedIPs) == 0 {
			return nil, fmt.Errorf("WireGuard %q has a peer without allowed IPs", ib.Tag)
		}
		row := map[string]any{"public_key": peer.PublicKey, "allowed_ips": peer.AllowedIPs}
		if peer.Email != "" {
			row["name"] = peer.Email
		}
		if peer.PreSharedKey != "" {
			if decoded, err := base64.StdEncoding.DecodeString(peer.PreSharedKey); err != nil || len(decoded) != 32 {
				return nil, fmt.Errorf("WireGuard %q has an invalid preshared key", ib.Tag)
			}
			row["pre_shared_key"] = peer.PreSharedKey
		}
		if peer.KeepAlive > 0 {
			row["persistent_keepalive_interval"] = peer.KeepAlive
		}
		native = append(native, row)
		allowed = append(allowed, peer.AllowedIPs...)
	}
	base := explicitWireguardSubnetBase(ib.Settings)
	if base == "" {
		base = wireguardAllocationBase(allowed, defaultWireguardBase)
	}
	prefix, err := netip.ParsePrefix(base)
	if err != nil {
		return nil, err
	}
	if prefix.Bits() > 30 {
		return nil, fmt.Errorf("WireGuard %q server subnet has no peer address space", ib.Tag)
	}
	address := fmt.Sprintf("%s/%d", prefix.Masked().Addr().Next(), prefix.Bits())
	addresses := []string{address}
	seen6 := map[string]bool{}
	for _, cidr := range allowed {
		peer, _ := netip.ParsePrefix(cidr)
		if peer.IsValid() && peer.Addr().Is6() {
			subnet := netip.PrefixFrom(peer.Addr(), 64).Masked()
			if !seen6[subnet.String()] {
				addresses = append(addresses, fmt.Sprintf("%s/64", subnet.Addr().Next()))
				seen6[subnet.String()] = true
			}
		}
	}
	out := map[string]any{"type": "wireguard", "tag": ib.Tag, "listen_port": ib.Port, "private_key": key, "address": addresses, "peers": native, "system": false}
	if mtu, ok := settings["mtu"].(float64); ok && mtu > 0 {
		out["mtu"] = mtu
	}
	return out, nil
}
