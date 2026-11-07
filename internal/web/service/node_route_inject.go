package service

import (
    "encoding/json"
    "fmt"

    "github.com/SawaMEN/3x-ui/v3/internal/logger"
    "github.com/SawaMEN/3x-ui/v3/internal/singbox"
    "github.com/SawaMEN/3x-ui/v3/internal/xray"
)

// A missing, disabled or incompatible destination must not prevent the whole
// VPN core from starting. Keep its tag resolvable and drop traffic instead of
// ever falling back to a direct connection.
func blockedNodeRouteOutbound(id int, core string) map[string]any {
    if core == "xray" {
        return map[string]any{"protocol": "blackhole", "tag": nodeRouteTag(id), "settings": map[string]any{}}
    }
    return map[string]any{"type": "block", "tag": nodeRouteTag(id)}
}

// injectXrayNodeRoutes materializes only destinations referenced by saved
// rules. No configuration secret is stored in the editor or sent to browsers.
func injectXrayNodeRoutes(cfg *xray.Config) error {
    if cfg == nil || len(cfg.RouterConfig) == 0 {
        return nil
    }
    var routing struct {
        Rules []map[string]any `json:"rules"`
    }
    if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
        return err
    }
    ids := nodeRouteIDs(routing.Rules, "outboundTag")
    if len(ids) == 0 {
        return nil
    }
    var outbounds []map[string]any
    if len(cfg.OutboundConfigs) > 0 {
        if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
            return fmt.Errorf("invalid xray outbounds: %w", err)
        }
    }
    used := make(map[string]bool, len(outbounds))
    for _, outbound := range outbounds {
        if tag, ok := outbound["tag"].(string); ok {
            used[tag] = true
        }
    }
    for _, id := range ids {
        tag := nodeRouteTag(id)
        if used[tag] {
            return fmt.Errorf("managed node route tag %q conflicts with an existing outbound", tag)
        }
        ob, err := resolveNodeRouteOutbound(id, "xray")
        if err != nil {
            logger.Warningf("node route %d: outbound unavailable, blocking traffic: %v", id, err)
            ob = blockedNodeRouteOutbound(id, "xray")
        }
        outbounds = append(outbounds, ob)
        used[tag] = true
    }
    raw, err := json.Marshal(outbounds)
    if err != nil {
        return err
    }
    cfg.OutboundConfigs = raw
    return nil
}

// injectSingBoxNodeRoutes is called after the native template and subscription
// overlays are merged. It supports outbound + endpoint destination namespaces.
func injectSingBoxNodeRoutes(cfg *singbox.Config) error {
    if cfg == nil {
        return nil
    }
    rules := singBoxRouteRules(cfg.Route)
    parsedRules := make([]map[string]any, 0, len(rules))
    for _, rule := range rules {
        if item, ok := rule.(map[string]any); ok {
            parsedRules = append(parsedRules, item)
        }
    }
    ids := nodeRouteIDs(parsedRules, "outbound")
    if len(ids) == 0 {
        return nil
    }
    used := make(map[string]bool)
    for _, outbound := range cfg.Outbounds {
        if tag, ok := outbound["tag"].(string); ok {
            used[tag] = true
        }
    }
    for _, endpoint := range cfg.Endpoints {
        if tag, ok := endpoint["tag"].(string); ok {
            used[tag] = true
        }
    }
    for _, id := range ids {
        tag := nodeRouteTag(id)
        if used[tag] {
            return fmt.Errorf("managed node route tag %q conflicts with an existing outbound", tag)
        }
        outbound, err := resolveNodeRouteOutbound(id, "sing-box")
        if err != nil {
            logger.Warningf("node route %d: outbound unavailable, blocking traffic: %v", id, err)
            outbound = blockedNodeRouteOutbound(id, "sing-box")
        }
        if outbound["type"] == "wireguard" || outbound["type"] == "awg" {
            cfg.Endpoints = append(cfg.Endpoints, outbound)
        } else {
            cfg.Outbounds = append(cfg.Outbounds, outbound)
        }
        used[tag] = true
    }
    return nil
}
