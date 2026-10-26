package service

import (
	"fmt"
	"maps"
	"net"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func routedSidecar(protocol model.Protocol) bool {
	return protocol == model.Mieru || protocol == model.TrustTunnel || protocol == model.Pingtunnel
}

func usesCoreSidecarRelay(ib *model.Inbound) bool {
	return ib != nil && ib.NodeID == nil && (routedSidecar(ib.Protocol) || ib.Protocol == model.VKTurnProxy || ib.Protocol == model.AmneziaWG)
}

// Inject after user templates, so a saved editor snapshot cannot erase the
// path between standalone protocol servers and the subscription outbounds.
func injectSingBoxSidecarEgress(cfg *singbox.Config, inbounds []*model.Inbound) error {
	ports, tags := map[int]string{}, map[string]bool{}
	for _, rows := range [][]map[string]any{cfg.Inbounds, cfg.Endpoints} {
		for _, row := range rows {
			tag, _ := row["tag"].(string)
			tags[tag] = true
			if port := singBoxInboundPort(row); port > 0 {
				ports[port] = tag
			}
		}
	}
	for _, ib := range inbounds {
		if ib != nil && ib.Enable && ib.NodeID == nil && ib.Port > 0 {
			ports[ib.Port] = ib.Tag
		}
		if ib != nil && ib.Enable && ib.NodeID == nil && ib.Protocol == model.Mieru {
			for _, binding := range mieru.PortBindingsFromInbound(ib) {
				if binding.Port > 0 {
					ports[binding.Port] = ib.Tag
				}
				if lo, hi, ok := strings.Cut(binding.PortRange, "-"); ok {
					first, _ := strconv.Atoi(lo)
					last, _ := strconv.Atoi(hi)
					for port := first; port <= last && port <= 65535; port++ {
						if port > 0 {
							ports[port] = ib.Tag
						}
					}
				}
			}
		}
	}
	add := func(ib *model.Inbound, row map[string]any, port int) error {
		if ib.Id <= 0 {
			return fmt.Errorf("sidecar %q has no database ID", ib.Tag)
		}
		if tags[ib.Tag] {
			return fmt.Errorf("sidecar relay tag %q already exists", ib.Tag)
		}
		if owner, ok := ports[port]; ok {
			return fmt.Errorf("sidecar %q relay port %d conflicts with %q", ib.Tag, port, owner)
		}
		row["tag"], row["listen"], row["listen_port"] = ib.Tag, "127.0.0.1", port
		cfg.Inbounds = append(cfg.Inbounds, row)
		tags[ib.Tag] = true
		ports[port] = ib.Tag
		return nil
	}
	blockVK := func(ib *model.Inbound) error {
		if err := add(ib, map[string]any{"type": "direct", "network": "udp", "override_address": "127.0.0.1", "override_port": 9}, sidecaregress.Port(ib.Id)+1); err != nil {
			return err
		}
		if cfg.Route == nil {
			cfg.Route = map[string]any{}
		}
		cfg.Route["rules"] = append([]any{map[string]any{"inbound": []string{ib.Tag}, "action": "reject"}}, singBoxRouteRules(cfg.Route)...)
		return nil
	}
	for _, ib := range inbounds {
		if ib == nil || !ib.Enable || ib.NodeID != nil {
			continue
		}
		if routedSidecar(ib.Protocol) {
			row := map[string]any{"type": "socks"}
			if ib.Protocol == model.Mieru {
				row["users"] = []map[string]any{{"username": mieru.SocksUsername(), "password": mieru.SocksPassword()}}
			}
			if err := add(ib, row, sidecaregress.Port(ib.Id)); err != nil {
				return err
			}
			if ib.Protocol == model.Mieru || ib.Protocol == model.TrustTunnel {
				if cfg.SidecarTrafficTags == nil {
					cfg.SidecarTrafficTags = map[string]bool{}
				}
				cfg.SidecarTrafficTags[ib.Tag] = true // standalone per-user counters own accounting
			}
			if cfg.Route == nil {
				cfg.Route = map[string]any{}
			}
			cfg.Route["rules"] = append([]any{map[string]any{"inbound": []string{ib.Tag}, "action": "sniff", "sniffer": []string{"http", "tls", "quic"}}}, singBoxRouteRules(cfg.Route)...)
		}
		if ib.Protocol != model.VKTurnProxy {
			continue
		}
		settings, err := (&InboundService{}).getVKTurnProxySettings(ib.Settings)
		if err != nil {
			return err
		}
		// Terminate a local nested VPN on a dedicated copy of its listener.
		// This keeps VK's own inbound tag, so routing it does not reroute the
		// direct clients of the original WireGuard/Hysteria listener.
		if settings.Forward.Type != VKTurnProxyForwardExternal {
			id := settings.Forward.Hysteria2InboundID
			if settings.Forward.Type == VKTurnProxyForwardWireGuardInbound {
				id = settings.Forward.WireGuardInboundID
			}
			var target *model.Inbound
			for _, candidate := range inbounds {
				if candidate != nil && candidate.Id == id {
					target = candidate
					break
				}
			}
			if target == nil || !target.Enable || target.NodeID != nil {
				if err := blockVK(ib); err != nil {
					return err
				}
				continue
			}
			port := sidecaregress.Port(ib.Id) + 1
			found := false
			for _, listener := range cfg.Inbounds {
				if listener["tag"] == target.Tag && listener["type"] == "hysteria2" {
					if err := add(ib, maps.Clone(listener), port); err != nil {
						return err
					}
					found = true
					break
				}
			}
			if !found {
				for _, endpoint := range cfg.Endpoints {
					if endpoint["tag"] == target.Tag && endpoint["type"] == "wireguard" {
						if ib.Id <= 0 || tags[ib.Tag] {
							return fmt.Errorf("VK TURN %q has a conflicting relay identity", ib.Tag)
						}
						if owner, taken := ports[port]; taken {
							return fmt.Errorf("VK TURN %q relay port conflicts with %q", ib.Tag, owner)
						}
						copy := maps.Clone(endpoint)
						copy["tag"], copy["listen_port"] = ib.Tag, port
						cfg.Endpoints = append(cfg.Endpoints, copy)
						tags[ib.Tag], ports[port] = true, ib.Tag
						found = true
						break
					}
				}
			}
			if !found {
				if err := blockVK(ib); err != nil {
					return err
				}
				continue
			}
			if cfg.Route == nil {
				cfg.Route = map[string]any{}
			}
			cfg.Route["rules"] = append([]any{map[string]any{"inbound": []string{ib.Tag}, "action": "sniff", "sniffer": []string{"http", "tls", "quic"}}}, singBoxRouteRules(cfg.Route)...)
			continue
		}

		host := strings.TrimSpace(settings.Forward.Host)
		port := settings.Forward.Port
		if host == "" || port < 1 || port > 65535 {
			return fmt.Errorf("VK TURN %q has an invalid forward target", ib.Tag)
		}
		if ip := net.ParseIP(host); (ip != nil && ip.IsLoopback()) || strings.EqualFold(host, "localhost") {
			return fmt.Errorf("VK TURN %q: use a local inbound target instead of an external loopback address", ib.Tag)
		}
		if err := add(ib, map[string]any{"type": "direct", "network": "udp", "override_address": host, "override_port": port}, sidecaregress.Port(ib.Id)+1); err != nil {
			return err
		}
	}
	return nil
}
