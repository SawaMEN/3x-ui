package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/xray/geodata"
)

// The template retains references so database updates take effect on the next
// build. Only the runtime receives the expanded, native sing-box matchers.
var nativeGeoFields = []struct {
	panel, legacy string
	matchers      []string
}{
	{"routingGeoDomains", "domain", []string{"domain", "domain_suffix", "domain_keyword", "domain_regex"}},
	{"routingGeoIPs", "ip", []string{"ip_cidr", "ip_is_private"}},
	{"routingGeoSourceIPs", "sourceIP", []string{"source_ip_cidr", "source_ip_is_private"}},
}

func nativeGeoTokens(value any) ([]string, error) {
	switch value := value.(type) {
	case []string:
		for _, token := range value {
			if token == "" {
				return nil, fmt.Errorf("geodata matchers must contain non-empty strings")
			}
		}
		return value, nil
	case string:
		if value != "" {
			return []string{value}, nil
		}
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			token, ok := item.(string)
			if !ok || token == "" {
				return nil, fmt.Errorf("geodata matchers must contain non-empty strings")
			}
			out = append(out, token)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("geodata matchers must be a string or an array of strings")
	}
	return nil, nil
}

func materializeNativeGeoRule(rule map[string]any, store *geodata.Store) error {
	if rule["enabled"] == false {
		return nil
	}
	for _, field := range nativeGeoFields {
		value, exists := rule[field.panel]
		if !exists {
			continue
		}
		tokens, err := nativeGeoTokens(value)
		if err != nil {
			return fmt.Errorf("%s: %w", field.panel, err)
		}
		delete(rule, field.panel)
		if len(tokens) == 0 {
			continue
		}
		if rule["type"] == "logical" {
			return fmt.Errorf("%s belongs inside a logical rule's child", field.panel)
		}
		legacy := map[string]any{"rules": []any{map[string]any{field.legacy: tokens, "outboundTag": "geo-target"}}}
		translated, err := singbox.TranslateXrayRoutingWithGeoData(legacy, store)
		if err != nil {
			return fmt.Errorf("%s: %w", field.panel, err)
		}
		matches, _ := translated["rules"].([]map[string]any)
		if len(matches) != 1 {
			return fmt.Errorf("%s resolved to an empty matcher", field.panel)
		}
		matched := false
		for _, key := range field.matchers {
			value, ok := matches[0][key]
			if !ok {
				continue
			}
			matched = true
			if flag, ok := value.(bool); ok {
				if flag {
					rule[key] = true
				}
				continue
			}
			incoming, err := nativeGeoTokens(value)
			if err != nil {
				return err
			}
			var current []string
			if existing, ok := rule[key]; ok {
				current, err = nativeGeoTokens(existing)
				if err != nil {
					return fmt.Errorf("%s: %w", key, err)
				}
			}
			rule[key] = append(current, incoming...)
		}
		if !matched {
			return fmt.Errorf("%s resolved to an empty matcher", field.panel)
		}
	}
	if children, ok := rule["rules"].([]any); ok {
		for i, child := range children {
			if object, ok := child.(map[string]any); ok {
				if err := materializeNativeGeoRule(object, store); err != nil {
					return fmt.Errorf("rules[%d]: %w", i, err)
				}
			}
		}
	}
	return nil
}

func materializeNativeGeoData(template string) (string, error) {
	if !strings.Contains(template, "routingGeo") {
		return template, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(template), &cfg); err != nil {
		return "", err
	}
	store := geodata.NewStore(assetDir())
	for i, rule := range nativeCatalogRules(cfg) {
		if err := materializeNativeGeoRule(rule, store); err != nil {
			return "", fmt.Errorf("route.rules[%d]: %w", i, err)
		}
	}
	raw, err := json.Marshal(cfg)
	return string(raw), err
}

func nativeTemplateReplacesRouting(template string) bool {
	var cfg map[string]json.RawMessage
	if json.Unmarshal([]byte(template), &cfg) != nil {
		return false
	}
	route, exists := cfg["route"]
	if !exists || string(route) == "null" {
		return false
	}
	var object map[string]any
	if json.Unmarshal(route, &object) != nil || object == nil {
		return false
	}
	_, gatewayOnly := mergeGatewayOnlyRoute(nil, route)
	return !gatewayOnly
}
