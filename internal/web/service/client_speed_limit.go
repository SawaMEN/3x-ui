package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/clientlimit"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Wrap terminal routes in place so filtering, outbound choice and rule order survive shaping.
func injectClientSpeedLimits(cfg *singbox.Config, clients map[int][]model.Client, inbounds []*model.Inbound) error {
	hasLimits := false
	for _, rows := range clients {
		for _, c := range rows {
			if c.Enable && c.SpeedLimit != nil && c.SpeedLimit.Enabled {
				hasLimits = true
				break
			}
		}
		if hasLimits {
			break
		}
	}
	if !hasLimits {
		return nil
	}
	active := map[string]map[string]bool{}
	for _, inbound := range cfg.Inbounds {
		raw, err := json.Marshal(inbound["users"])
		if err != nil {
			return err
		}
		var users []struct {
			Name     string `json:"name"`
			Username string `json:"username"`
		}
		if err := json.Unmarshal(raw, &users); err != nil {
			return err
		}
		tag, _ := inbound["tag"].(string)
		active[tag] = map[string]bool{}
		for _, user := range users {
			active[tag][user.Name] = true
			active[tag][user.Username] = true
		}
	}
	limits := map[string]*model.ClientSpeedLimit{}
	tags := map[string][]string{}
	aliases := map[string][]string{}
	for _, ib := range inbounds {
		if ib == nil || !ib.Enable || ib.NodeID != nil {
			continue
		}
		for _, c := range clients[ib.Id] {
			if c.SpeedLimit == nil || !c.SpeedLimit.Enabled || !c.Enable {
				continue
			}
			if err := c.SpeedLimit.Validate(); err != nil {
				return err
			}
			if !model.SupportsClientSpeedLimit(ib.Protocol) {
				return fmt.Errorf("client %q: speed limiting is not supported for %s", c.Email, ib.Protocol)
			}
			tag := ib.Tag
			alias := ""
			switch ib.Protocol {
			case model.Mieru:
				alias = "mieru:" + strings.TrimSpace(c.Email)
			case model.MASQUE:
				alias = "masque:" + tag + ":" + c.Email
			case model.Sudoku:
				key := c.SudokuPrivateKey
				var settings struct {
					Clients []model.Client `json:"clients"`
				}
				if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
					return err
				}
				for _, stored := range settings.Clients {
					if stored.Email == c.Email {
						key = stored.SudokuPrivateKey
						break
					}
				}
				raw, err := hex.DecodeString(strings.TrimSpace(key))
				if err != nil || (len(raw) != 32 && len(raw) != 64) {
					return fmt.Errorf("client %q has no valid Sudoku key", c.Email)
				}
				digest := sha256.Sum256(raw)
				alias = "sudoku:" + hex.EncodeToString(digest[:8])
			}
			if alias != "" {
				aliases[c.Email] = append(aliases[c.Email], alias)
				limits[c.Email] = c.SpeedLimit
				continue
			}
			if !active[tag][c.Email] {
				continue
			}
			limits[c.Email] = c.SpeedLimit
			tags[c.Email] = append(tags[c.Email], tag)
		}
	}
	if len(limits) == 0 {
		return nil
	}
	for _, listener := range append(slices.Clone(cfg.Inbounds), cfg.Endpoints...) {
		port := fmt.Sprint(listener["listen_port"])
		if port == fmt.Sprint(clientlimit.ListenPort) || port == fmt.Sprint(clientlimit.BridgePort) {
			return fmt.Errorf("client speed limiter reserved port conflicts with inbound %v", listener["tag"])
		}
		if strings.HasPrefix(fmt.Sprint(listener["tag"]), clientlimit.InternalPrefix) {
			return fmt.Errorf("reserved client speed limiter tag %v", listener["tag"])
		}
	}
	for _, ob := range cfg.Outbounds {
		if strings.HasPrefix(fmt.Sprint(ob["tag"]), clientlimit.InternalPrefix) {
			return fmt.Errorf("reserved client speed limiter outbound tag %v", ob["tag"])
		}
	}
	emails := make([]string, 0, len(limits))
	for email := range limits {
		emails = append(emails, email)
	}
	slices.Sort(emails)
	var returns, wrapped []map[string]any
	var accounts []map[string]any
	outbounds := map[string]string{}
	for _, ob := range cfg.Outbounds {
		if tag, _ := ob["tag"].(string); tag != "" {
			outbounds[tag], _ = ob["type"].(string)
		}
	}
	for _, ep := range cfg.Endpoints {
		if tag, _ := ep["tag"].(string); tag != "" {
			outbounds[tag], _ = ep["type"].(string)
		}
	}
	bridgeTag := clientlimit.InternalPrefix + "return"
	routes := map[string]string{}
	proxyFor := func(email, target string) (string, error) {
		if _, ok := outbounds[target]; !ok {
			return "", fmt.Errorf("speed limiter references unknown outbound %q", target)
		}
		limit := limits[email]
		key := fmt.Sprintf("%s\x00%s\x00%d/%d", email, target, limit.UploadMbps, limit.DownloadMbps)
		if tag := routes[key]; tag != "" {
			return tag, nil
		}
		// Route IDs are non-secret; authentication uses the random process password.
		// Include rates so config comparison detects edits to an existing cap.
		id := fmt.Sprintf("%s%d_%d_%d", clientlimit.InternalPrefix, len(routes), limit.UploadMbps, limit.DownloadMbps)
		routes[key] = id
		cfg.SpeedLimitPlan.Routes = append(cfg.SpeedLimitPlan.Routes, clientlimit.Route{ID: id, Email: email, UploadMbps: limit.UploadMbps, DownloadMbps: limit.DownloadMbps, Aliases: aliases[email]})
		cfg.Outbounds = append(cfg.Outbounds, map[string]any{"type": "socks", "tag": id, "server": "127.0.0.1", "server_port": clientlimit.ListenPort, "version": "5", "udp_over_tcp": map[string]any{"enabled": true, "version": 2}, "username": id, "password": clientlimit.Default.Password()})
		accounts = append(accounts, map[string]any{"username": id, "password": clientlimit.Default.Password()})
		returns = append(returns, map[string]any{"inbound": []string{bridgeTag}, "auth_user": []string{id}, "action": "route", "outbound": target})
		return id, nil
	}
	matchUser := func(email string) map[string]any {
		return map[string]any{"auth_user": []string{email}, "inbound": tags[email], "network": []string{"tcp", "udp"}}
	}
	for _, value := range singBoxRouteRules(cfg.Route) {
		rule, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("speed limiter: malformed routing rule")
		}
		action, _ := rule["action"].(string)
		target, _ := rule["outbound"].(string)
		if (action == "" || action == "route" || action == "bypass") && target != "" && outbounds[target] != "block" {
			for _, email := range emails {
				if len(tags[email]) == 0 {
					continue
				}
				proxy, err := proxyFor(email, target)
				if err != nil {
					return err
				}
				predicate := maps.Clone(rule)
				wrappedRule := map[string]any{"type": "logical", "mode": "and", "action": "route", "outbound": proxy}
				for _, key := range speedLimitRouteActionFields {
					if value, ok := predicate[key]; ok {
						wrappedRule[key] = value
						delete(predicate, key)
					}
				}
				delete(predicate, "action")
				delete(predicate, "outbound")
				wrappedRule["outbound"] = proxy
				wrappedRule["rules"] = []map[string]any{predicate, matchUser(email)}
				wrapped = append(wrapped, wrappedRule)
			}
		}
		wrapped = append(wrapped, rule)
	}
	final, _ := cfg.Route["final"].(string)
	if final == "" && len(cfg.Outbounds) > 0 {
		final, _ = cfg.Outbounds[0]["tag"].(string)
	}
	for _, email := range emails {
		if len(aliases[email]) > 0 {
			if _, err := proxyFor(email, final); err != nil {
				return err
			}
		}
	}
	if outbounds[final] != "block" {
		for _, email := range emails {
			if len(tags[email]) == 0 {
				continue
			}
			proxy, err := proxyFor(email, final)
			if err != nil {
				return err
			}
			rule := matchUser(email)
			rule["action"] = "route"
			rule["outbound"] = proxy
			wrapped = append(wrapped, rule)
		}
	}
	if len(cfg.SpeedLimitPlan.Routes) == 0 {
		return nil
	}
	cfg.Inbounds = append(cfg.Inbounds, map[string]any{"type": "socks", "tag": bridgeTag, "listen": "127.0.0.1", "listen_port": clientlimit.BridgePort, "users": accounts})
	if cfg.SidecarTrafficTags == nil {
		cfg.SidecarTrafficTags = map[string]bool{}
	}
	cfg.SidecarTrafficTags[bridgeTag] = true
	cfg.Route["rules"] = append(returns, wrapped...)
	return nil
}

var speedLimitRouteActionFields = []string{
	"override_address", "override_port", "network_strategy", "fallback_network_type", "fallback_delay",
	"udp_disable_domain_unmapping", "udp_connect", "udp_timeout", "tls_fragment", "tls_fragment_fallback_delay", "tls_record_fragment", "tls_spoof", "tls_spoof_method",
}

func encodeSpeedLimit(limit *model.ClientSpeedLimit) string {
	b, _ := json.Marshal(limit)
	return string(b)
}
