package service

import (
	"encoding/json"
	"fmt"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func injectSingBoxMieruEgress(cfg *singbox.Config, inbounds []*model.Inbound) error {
	for _, ib := range inbounds {
		if ib == nil || !ib.Enable || ib.NodeID != nil || ib.Protocol != model.Mieru {
			continue
		}
		port := mieru.SOCKSPortForInbound(ib.Id)
		for _, existing := range cfg.Inbounds {
			if existing["tag"] == ib.Tag || fmt.Sprint(existing["listen_port"]) == fmt.Sprint(port) {
				return fmt.Errorf("Mieru inbound %q egress bridge conflicts with an existing listener on port %d", ib.Tag, port)
			}
		}
		for _, public := range inbounds {
			if public == nil || !public.Enable || public.NodeID != nil {
				continue
			}
			if public.Port == port {
				return fmt.Errorf("Mieru egress port %d conflicts with inbound %q", port, public.Tag)
			}
			for _, binding := range mieru.PortBindingsFromInbound(public) {
				if binding.Port == port {
					return fmt.Errorf("Mieru egress port %d conflicts with Mieru listener", port)
				}
				var first, last int
				if _, err := fmt.Sscanf(binding.PortRange, "%d-%d", &first, &last); err == nil && port >= first && port <= last {
					return fmt.Errorf("Mieru egress port %d conflicts with Mieru listener range", port)
				}
			}
		}
		var sniffing any
		if ib.Sniffing != "" {
			if err := json.Unmarshal([]byte(ib.Sniffing), &sniffing); err != nil {
				return fmt.Errorf("Mieru inbound %q sniffing: %w", ib.Tag, err)
			}
		}
		sniff, err := hiddify.TranslateSniffingRule(map[string]any{"tag": ib.Tag, "sniffing": sniffing})
		if err != nil {
			return err
		}
		if sniff != nil {
			if cfg.Route == nil {
				cfg.Route = map[string]any{}
			}
			cfg.Route["rules"] = append([]any{sniff}, singBoxRouteRules(cfg.Route)...)
		}
		cfg.Inbounds = append(cfg.Inbounds, map[string]any{
			"type": "socks", "tag": ib.Tag, "listen": "127.0.0.1", "listen_port": port,
			"users": []map[string]any{{"username": mieru.SocksUsername(), "password": mieru.SocksPassword()}},
		})
	}
	return nil
}
