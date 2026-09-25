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
	var translatedDNS, translatedRoute map[string]any
	if template != nil {
		if rawDNS, ok := template["dns"].(map[string]any); ok {
			var err error
			translatedDNS, err = singbox.TranslateXrayDNS(rawDNS)
			if err != nil {
				return "", fmt.Errorf("%w: translate DNS: %w", errSubscriptionFormatUnsupported, err)
			}
		}
		if rawRouting, ok := template["routing"].(map[string]any); ok {
			var err error
			translatedRoute, err = singbox.TranslateXrayRouting(rawRouting)
			if err != nil {
				return "", fmt.Errorf("%w: translate routing: %w", errSubscriptionFormatUnsupported, err)
			}
		}
	}
	// A standalone profile must be able to resolve the proxy's server even
	// when the panel has no custom DNS template. Keep the local resolver
	// separate from the user's DNS rule and final server.
	if translatedDNS == nil {
		translatedDNS = map[string]any{}
	}
	servers, _ := translatedDNS["servers"].([]map[string]any)
	servers = append(servers, map[string]any{"type": "local", "tag": "panel-local"})
	translatedDNS["servers"] = servers
	if translatedRoute == nil {
		translatedRoute = map[string]any{}
	}
	translatedRoute["default_domain_resolver"] = "panel-local"

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

		cfg["dns"] = translatedDNS
		cfg["route"] = translatedRoute

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
