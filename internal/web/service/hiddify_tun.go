package service

import (
	"encoding/json"
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Apply after the native template: an explicit template cannot silently undo
// the bind needed to prevent a default TUN route recapturing core sockets.
func configureHiddifyTunRouting(cfg *singbox.Config, inbounds []*model.Inbound) error {
	iface := ""
	for _, ib := range inbounds {
		if !ib.Enable || ib.NodeID != nil || ib.Protocol != model.Tun {
			continue
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
			return err
		}
		selected, err := hiddify.TunOutboundInterface(settings)
		if err != nil {
			return err
		}
		if iface != "" && iface != selected {
			return fmt.Errorf("TUN listeners require the same outbound interface")
		}
		iface = selected
	}
	if iface == "" {
		return nil
	}

	virtual := map[string]bool{}
	for _, ib := range cfg.Inbounds {
		if ib["type"] != "tun" {
			continue
		}
		name, _ := ib["interface_name"].(string)
		if virtual[name] {
			return fmt.Errorf("duplicate TUN interface %q", name)
		}
		virtual[name] = true
	}
	for _, ob := range cfg.Outbounds {
		if bind, _ := ob["bind_interface"].(string); virtual[bind] {
			return fmt.Errorf("outbound %q binds to managed TUN interface %q", ob["tag"], bind)
		}
	}
	if cfg.Route == nil {
		cfg.Route = map[string]any{}
	}
	if iface == "auto" {
		if value, exists := cfg.Route["auto_detect_interface"]; exists && value != true {
			return fmt.Errorf("TUN requires route.auto_detect_interface")
		}
		if value, _ := cfg.Route["default_interface"].(string); value != "" {
			return fmt.Errorf("TUN auto interface conflicts with route.default_interface")
		}
		cfg.Route["auto_detect_interface"] = true
	} else {
		if value, _ := cfg.Route["default_interface"].(string); value != "" && value != iface {
			return fmt.Errorf("TUN outbound interface conflicts with route.default_interface")
		}
		if value, _ := cfg.Route["auto_detect_interface"].(bool); value {
			return fmt.Errorf("TUN explicit interface conflicts with route.auto_detect_interface")
		}
		cfg.Route["default_interface"] = iface
	}
	return nil
}

func configureHiddifyGatewayDNS(cfg *singbox.Config) error {
	gateway := false
	autoTun := false
	for _, inbound := range cfg.Inbounds {
		if inbound["tag"] == "in-tproxy" && inbound["type"] == "tproxy" {
			gateway = true
		}
		if inbound["type"] == "tun" && inbound["auto_route"] == true {
			autoTun = true
		}
	}
	if !gateway {
		return nil
	}
	if autoTun {
		return fmt.Errorf("Gateway TPROXY and automatic TUN routing cannot be enabled together")
	}
	if cfg.Route == nil {
		cfg.Route = map[string]any{}
	}
	cfg.Route["rules"] = append([]any{map[string]any{"inbound": []string{"in-tproxy"}, "port": 53, "action": "hijack-dns"}}, singBoxRouteRules(cfg.Route)...)
	return nil
}
