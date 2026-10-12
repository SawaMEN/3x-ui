package hiddify

import (
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func takeReverseExtensions(settings map[string]any, protocol string, inbound bool) ([]map[string]any, error) {
	if strings.TrimPrefix(protocol, "singbox:") != "vless" {
		return nil, nil
	}
	var users []map[string]any
	if inbound {
		clients, _ := settings["clients"].([]any)
		for _, client := range clients {
			if user, ok := client.(map[string]any); ok {
				users = append(users, user)
			}
		}
	} else {
		users = []map[string]any{vlessUser(settings)}
	}
	result := make([]map[string]any, len(users))
	for i, user := range users {
		raw, exists := user["reverse"]
		if !exists || raw == nil {
			continue
		}
		fields, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("reverse must be an object")
		}
		if len(fields) == 0 {
			delete(user, "reverse")
			continue
		}
		tag, ok := fields["tag"].(string)
		if !ok || strings.TrimSpace(tag) == "" || tag != strings.TrimSpace(tag) {
			return nil, fmt.Errorf("reverse tag must be a nonempty string without surrounding whitespace")
		}
		for key, value := range fields {
			if key != "tag" && (key != "sniffing" || inbound) && meaningful(value) {
				return nil, fmt.Errorf("unsupported reverse option %s", key)
			}
		}
		result[i] = fields
		delete(user, "reverse")
	}
	return result, nil
}

func applyReverseExtensions(out map[string]any, reverse []map[string]any, inbound bool) error {
	if len(reverse) == 0 {
		return nil
	}
	if !inbound {
		if reverse[0] != nil {
			out["reverse"] = reverse[0]
		}
		return nil
	}
	users, ok := out["users"].([]map[string]any)
	if !ok || len(users) != len(reverse) {
		return fmt.Errorf("reverse user mapping mismatch")
	}
	for i, r := range reverse {
		if r != nil {
			users[i]["reverse"] = r
		}
	}
	return nil
}

// AddReversePortals creates one native portal per authorized tag and preserves bridge sniffing as route actions.
func AddReversePortals(cfg *singbox.Config) error {
	tags := map[string]string{}
	for _, o := range cfg.Outbounds {
		tag, _ := o["tag"].(string)
		tags[tag], _ = o["type"].(string)
	}
	for _, e := range cfg.Endpoints {
		if tag, ok := e["tag"].(string); ok {
			tags[tag] = "endpoint"
		}
	}
	portals := map[string]bool{}
	for _, in := range cfg.Inbounds {
		if in["type"] != "vless" {
			continue
		}
		users, ok := in["users"].([]map[string]any)
		if !ok {
			raw, _ := in["users"].([]any)
			for _, v := range raw {
				if u, ok := v.(map[string]any); ok {
					users = append(users, u)
				}
			}
		}
		for _, user := range users {
			r, _ := user["reverse"].(map[string]any)
			if r == nil {
				continue
			}
			tag, _ := r["tag"].(string)
			if tag == "" {
				return fmt.Errorf("reverse portal tag required")
			}
			if portals[tag] {
				continue
			}
			if tags[tag] == "vless-reverse" {
				portals[tag] = true
				continue
			}
			if tags[tag] != "" {
				return fmt.Errorf("reverse portal tag %q conflicts with an outbound", tag)
			}
			portals[tag] = true
			tags[tag] = "vless-reverse"
			cfg.Outbounds = append(cfg.Outbounds, map[string]any{"type": "vless-reverse", "tag": tag})
		}
	}
	var sniffRules []any
	for _, o := range cfg.Outbounds {
		r, _ := o["reverse"].(map[string]any)
		if r == nil {
			continue
		}
		if sniff, exists := r["sniffing"]; exists {
			rule, err := TranslateSniffingRule(map[string]any{"tag": r["tag"], "sniffing": sniff})
			if err != nil {
				return err
			}
			if rule != nil {
				sniffRules = append(sniffRules, rule)
			}
			delete(r, "sniffing")
		}
		if cfg.Route != nil && cfg.Route["final"] == o["tag"] {
			return fmt.Errorf("reverse-only outbound %q cannot be the default route", o["tag"])
		}
	}
	if len(sniffRules) > 0 {
		if cfg.Route == nil {
			cfg.Route = map[string]any{}
		}
		switch rules := cfg.Route["rules"].(type) {
		case []any:
			sniffRules = append(sniffRules, rules...)
		case []map[string]any:
			for _, r := range rules {
				sniffRules = append(sniffRules, r)
			}
		}
		cfg.Route["rules"] = sniffRules
	}
	return nil
}
