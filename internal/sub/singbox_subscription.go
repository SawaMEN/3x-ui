package sub

import (
	"encoding/json"
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// buildSeparatedSingBoxSubscription emits one native sing-box document per
// subscription proxy. This mirrors Hiddify's client model: every inbound/proxy
// becomes an independent profile instead of combining VLESS and other protocols
// into one selector/urltest document.
func buildSeparatedSingBoxSubscription(template map[string]any, proxies []map[string]any, alwaysReturnArray bool) (string, error) {
	if len(proxies) == 0 {
		return "", nil
	}

	configs := make([]json.RawMessage, 0, len(proxies))
	for _, proxy := range proxies {
		if proxy == nil {
			continue
		}

		cfg := map[string]any{
			"$schema": "https://sing-box.sagernet.org/schema.json",
			"outbounds": []any{
				proxy,
				map[string]any{"type": "direct", "tag": "direct"},
				map[string]any{"type": "block", "tag": "blocked"},
			},
		}

		if template != nil {
			if rawDNS, ok := template["dns"].(map[string]any); ok {
				dns, err := singbox.TranslateXrayDNS(rawDNS)
				if err != nil {
					return "", fmt.Errorf("%w: translate DNS: %v", errSubscriptionFormatUnsupported, err)
				}
				if len(dns) > 0 {
					cfg["dns"] = dns
				}
			}
			if rawRouting, ok := template["routing"].(map[string]any); ok {
				route, err := singbox.TranslateXrayRouting(rawRouting)
				if err != nil {
					return "", fmt.Errorf("%w: translate routing: %v", errSubscriptionFormatUnsupported, err)
				}
				if len(route) > 0 {
					cfg["route"] = route
				}
			}
		}

		encoded, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal sing-box subscription profile: %w", err)
		}
		configs = append(configs, encoded)
	}

	if len(configs) == 0 {
		return "", nil
	}
	if len(configs) == 1 && !alwaysReturnArray {
		return string(configs[0]), nil
	}

	encoded, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal sing-box subscription array: %w", err)
	}
	return string(encoded), nil
}
