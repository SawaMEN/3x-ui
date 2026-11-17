package service

import (
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/amneziawg"
	"github.com/SawaMEN/3x-ui/v3/internal/amneziawgnet"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Embedded AWG sends both TCP and UDP to this authenticated SOCKS listener.
// Keep the original inbound tag and peer email so normal routing and core
// accounting apply, including subscription selectors and user-scoped rules.
func injectSingBoxAmneziaWGEgress(cfg *singbox.Config, inbounds []*model.Inbound) error {
	ports := map[int]string{}
	tags := map[string]bool{}
	for _, listeners := range [][]map[string]any{cfg.Inbounds, cfg.Endpoints} {
		for _, listener := range listeners {
			tag, _ := listener["tag"].(string)
			tags[tag] = true
			if port := singBoxInboundPort(listener); port > 0 {
				ports[port] = tag
			}
		}
	}
	// Sidecar public sockets are absent from cfg.Inbounds but still reserve ports.
	for _, ib := range inbounds {
		if ib != nil && ib.Enable && ib.NodeID == nil && ib.Port > 0 {
			ports[ib.Port] = ib.Tag
		}
	}
	for _, ib := range inbounds {
		if ib == nil || !ib.Enable || ib.NodeID != nil || ib.Protocol != model.AmneziaWG {
			continue
		}
		inst, ok := amneziawg.InstanceFromInbound(ib)
		if !ok {
			continue
		}
		disabled := map[string]bool{}
		for _, row := range ib.ClientStats {
			if !row.Enable {
				disabled[row.Email] = true
			}
		}
		users := make([]map[string]any, 0, len(inst.Peers))
		for _, peer := range inst.Peers {
			if disabled[peer.Email] {
				continue
			}
			if peer.Email == "" {
				return fmt.Errorf("AmneziaWG inbound %q has a peer without an email", ib.Tag)
			}
			users = append(users, map[string]any{"username": peer.Email, "password": amneziawgnet.SocksPassword()})
		}
		if len(users) == 0 {
			continue
		}
		if ib.Id <= 0 {
			return fmt.Errorf("AmneziaWG inbound %q has no database ID", ib.Tag)
		}
		port := amneziawgnet.SOCKSPortForInbound(ib.Id)
		if tags[ib.Tag] {
			return fmt.Errorf("AmneziaWG relay tag %q conflicts with a native listener", ib.Tag)
		}
		if owner, taken := ports[port]; taken {
			return fmt.Errorf("AmneziaWG inbound %q relay port %d conflicts with %q", ib.Tag, port, owner)
		}
		cfg.Inbounds = append(cfg.Inbounds, map[string]any{"type": "socks", "tag": ib.Tag, "listen": "127.0.0.1", "listen_port": port, "users": users})
		tags[ib.Tag], ports[port] = true, ib.Tag
		if cfg.Route == nil {
			cfg.Route = map[string]any{}
		}
		// AWG transports IP packets, so recover domains before domain routing.
		cfg.Route["rules"] = append([]any{map[string]any{"inbound": []string{ib.Tag}, "action": "sniff", "sniffer": []string{"http", "tls", "quic"}}}, singBoxRouteRules(cfg.Route)...)
	}
	return nil
}
