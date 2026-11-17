package hiddify

import (
	"fmt"
	"strings"
)

// TranslateBalancers preserves dynamic strategy selection in the modified core.
func TranslateBalancers(routing, config map[string]any, available ...map[string]any) ([]map[string]any, error) {
	items, ok := routing["balancers"].([]any)
	if routing["balancers"] != nil && !ok {
		return nil, fmt.Errorf("routing balancers must be an array")
	}
	known := map[string]bool{}
	for _, out := range available {
		if tag, _ := out["tag"].(string); tag != "" {
			known[tag] = true
		}
	}
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			if tag, _ := row["tag"].(string); tag != "" {
				known[tag] = true
			}
		}
	}
	var result []map[string]any
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("balancer must be an object")
		}
		tag, _ := row["tag"].(string)
		if strings.TrimSpace(tag) == "" {
			return nil, fmt.Errorf("balancer tag must not be empty")
		}
		prefixes, err := balancerStrings(row["selector"])
		if err != nil || len(prefixes) == 0 {
			return nil, fmt.Errorf("balancer %q requires selectors", tag)
		}
		var targets []string
		for _, out := range available {
			name, _ := out["tag"].(string)
			protocol, _ := out["protocol"].(string)
			if name == "" || protocol == "dns" || protocol == "singbox:dns" {
				continue
			}
			for _, prefix := range prefixes {
				if strings.HasPrefix(name, prefix) {
					targets = append(targets, name)
					break
				}
			}
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("balancer %q selectors match no outbound", tag)
		}
		strategy := object(row, "strategy")
		kind, _ := strategy["type"].(string)
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind == "" {
			kind = "random"
		}
		switch kind {
		case "random", "roundrobin", "leastping", "leastload":
		default:
			return nil, fmt.Errorf("balancer %q has unknown strategy %q", tag, kind)
		}
		options := map[string]any{"strategy": kind}
		if value, ok := strategy["settings"]; ok && meaningful(value) {
			options["settings"] = value
		}
		fallback, _ := row["fallbackTag"].(string)
		if fallback != "" {
			if !known[fallback] {
				return nil, fmt.Errorf("balancer %q fallback %q does not exist", tag, fallback)
			}
			if fallback == tag {
				return nil, fmt.Errorf("balancer %q cannot use itself as fallback", tag)
			}
			options["fallback"] = fallback
		}
		if kind == "leastping" || kind == "leastload" || fallback != "" {
			if err := addBalancerObservation(options, config, targets, tag); err != nil {
				return nil, err
			}
		}
		result = append(result, map[string]any{"type": "selector", "tag": tag, "outbounds": targets, "default": targets[0], "balancer": options})
	}
	return result, nil
}

func balancerStrings(value any) ([]string, error) {
	if values, ok := value.([]string); ok {
		return values, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("expected string array")
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		s, ok := value.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("invalid selector")
		}
		out = append(out, s)
	}
	return out, nil
}

func addBalancerObservation(out, config map[string]any, targets []string, tag string) error {
	obs := object(config, "observatory")
	burst := false
	if len(obs) == 0 {
		obs = object(config, "burstObservatory")
		burst = len(obs) > 0
	}
	if len(obs) == 0 {
		return fmt.Errorf("balancer %q requires observatory configuration", tag)
	}
	prefixes, err := balancerStrings(obs["subjectSelector"])
	if err != nil || len(prefixes) == 0 {
		return fmt.Errorf("balancer %q requires observatory subject selectors", tag)
	}
	var subjects []string
	for _, target := range targets {
		for _, prefix := range prefixes {
			if strings.HasPrefix(target, prefix) {
				subjects = append(subjects, target)
				break
			}
		}
	}
	if len(subjects) == 0 {
		return fmt.Errorf("balancer %q has no observed candidates", tag)
	}
	out["probe_tags"] = subjects
	if burst {
		health := object(obs, "pingConfig")
		if len(health) == 0 {
			health = object(obs, "healthCheck")
		}
		out["burst"] = true
		for key, native := range map[string]string{"destination": "probe_url", "interval": "interval", "timeout": "timeout", "sampling": "sampling", "httpMethod": "method", "connectivity": "connectivity"} {
			if value, exists := health[key]; exists && meaningful(value) {
				out[native] = value
			}
		}
	} else {
		for key, native := range map[string]string{"probeURL": "probe_url", "probeInterval": "interval", "enableConcurrency": "concurrent"} {
			if value, exists := obs[key]; exists && meaningful(value) {
				out[native] = value
			}
		}
	}
	if out["probe_url"] == nil {
		return fmt.Errorf("balancer %q requires an explicit probe URL", tag)
	}
	return nil
}
