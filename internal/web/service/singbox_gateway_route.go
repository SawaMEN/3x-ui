package service

import "encoding/json"

// Starting Gateway from an empty native template creates only its sniff rule.
// That tiny patch must not replace the routes generated from normal settings.
func mergeGatewayOnlyRoute(base, patch json.RawMessage) (json.RawMessage, bool) {
	var route map[string]any
	if json.Unmarshal(patch, &route) != nil || len(route) != 1 {
		return nil, false
	}
	rules, ok := route["rules"].([]any)
	if !ok || len(rules) != 1 {
		return nil, false
	}
	rule, ok := rules[0].(map[string]any)
	if !ok || len(rule) != 3 || rule["action"] != "sniff" {
		return nil, false
	}
	inbound, ok := rule["inbound"].([]any)
	if !ok || len(inbound) != 1 || inbound[0] != "in-tproxy" {
		return nil, false
	}
	sniffer, ok := rule["sniffer"].([]any)
	if !ok || len(sniffer) != 3 || sniffer[0] != "http" || sniffer[1] != "tls" || sniffer[2] != "quic" {
		return nil, false
	}
	var current map[string]any
	if len(base) > 0 && string(base) != "null" {
		if json.Unmarshal(base, &current) != nil {
			return nil, false
		}
	}
	if current == nil {
		current = map[string]any{}
	}
	existing, _ := current["rules"].([]any)
	current["rules"] = append([]any{rule}, existing...)
	raw, err := json.Marshal(current)
	return raw, err == nil
}
