package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Telemt stays independent of the selected core; only its loopback egress
// bridge belongs to that core. Validate the final native target set too.
func injectSingBoxMtprotoEgress(cfg *singbox.Config, inbound *model.Inbound) error {
	if inbound == nil || !inbound.Enable || inbound.NodeID != nil || !mtprotoRoutesThroughXray(inbound) {
		return nil
	}
	var settings struct {
		Port     int    `json:"routeXrayPort"`
		Outbound string `json:"outboundTag"`
		FallbackOutboundTags []string `json:"fallbackOutboundTags"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return err
	}
	if settings.Port < 1 || settings.Port > 65535 {
		return fmt.Errorf("MTProto inbound %q has no valid egress bridge port", inbound.Tag)
	}
	for _, ib := range cfg.Inbounds {
		if ib["tag"] == inbound.Tag {
			return fmt.Errorf("MTProto bridge tag %q conflicts with a native inbound", inbound.Tag)
		}
	}
	settings.Outbound = strings.TrimSpace(settings.Outbound)
	if len(settings.FallbackOutboundTags) > 0 {
		if settings.Outbound == "" || len(settings.FallbackOutboundTags) > 3 {
			return fmt.Errorf("MTProto inbound %q has invalid fallback outbound settings", inbound.Tag)
		}
		tags := []string{settings.Outbound}
		used := map[string]bool{settings.Outbound: true}
		for _, raw := range settings.FallbackOutboundTags {
			tag := strings.TrimSpace(raw)
			if tag == "" || used[tag] {
				return fmt.Errorf("MTProto inbound %q fallback targets must be distinct", inbound.Tag)
			}
			used[tag] = true
			tags = append(tags, tag)
		}
		known := make(map[string]bool, len(cfg.Outbounds))
		for _, outbound := range cfg.Outbounds {
			if tag, ok := outbound["tag"].(string); ok && tag != "" {
				known[tag] = true
			}
		}
		for _, tag := range tags {
			if !known[tag] {
				return fmt.Errorf("MTProto inbound %q fallback target %q is not a Sing-box outbound", inbound.Tag, tag)
			}
		}
		failoverTag := "__telemt_route_auto_" + fmt.Sprint(inbound.Id)
		if known[failoverTag] {
			return fmt.Errorf("MTProto failover tag %q conflicts with a configured outbound", failoverTag)
		}
		// Large tolerance prioritizes the first live entry rather than
		// choosing the lowest-latency route. Failed HTTP checks have no delay.
		cfg.Outbounds = append(cfg.Outbounds, map[string]any{
			"type": "urltest", "tag": failoverTag, "outbounds": tags,
			"url": "https://core.telegram.org/", "interval": "2m", "tolerance": 30000,
			"interrupt_exist_connections": false,
		})
		settings.Outbound = failoverTag
	}
	if settings.Outbound != "" {
		found := false
		for _, targets := range [][]map[string]any{cfg.Outbounds, cfg.Endpoints} {
			for _, target := range targets {
				if target["tag"] == settings.Outbound {
					found = true
				}
			}
		}
		if !found {
			return fmt.Errorf("MTProto inbound %q selects missing sing-box outbound %q", inbound.Tag, settings.Outbound)
		}
		if cfg.Route == nil {
			cfg.Route = map[string]any{}
		}
		if err := prependSingBoxMtprotoRule(cfg, map[string]any{"inbound": []string{inbound.Tag}, "action": "route", "outbound": settings.Outbound}); err != nil {
			return err
		}
	}
	cfg.Inbounds = append(cfg.Inbounds, map[string]any{"type": "socks", "tag": inbound.Tag, "listen": "127.0.0.1", "listen_port": settings.Port})
	return nil
}

func prependSingBoxMtprotoRule(cfg *singbox.Config, rule map[string]any) error {
	data, err := json.Marshal(cfg.Route["rules"])
	if err != nil {
		return err
	}
	var rules []map[string]any
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}
	cfg.Route["rules"] = append([]map[string]any{rule}, rules...)
	return nil
}
