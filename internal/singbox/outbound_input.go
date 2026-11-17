package singbox

import (
	"fmt"
	"maps"
	"strings"
)

// NormalizeOutboundInput accepts either the panel wrapper or native Sing-box
// JSON. Native type wins over a protocol option belonging to native settings.
func NormalizeOutboundInput(raw map[string]any) (map[string]any, error) {
	if raw == nil {
		return nil, fmt.Errorf("outbound must be an object")
	}
	out := maps.Clone(raw)
	if value, exists := raw["type"]; exists {
		typ, ok := value.(string)
		if !ok || strings.TrimSpace(typ) == "" {
			return nil, fmt.Errorf("native outbound type must be a non-empty string")
		}
		settings := maps.Clone(raw)
		delete(settings, "type")
		delete(settings, "tag")
		out = map[string]any{"protocol": "singbox:" + strings.ToLower(strings.TrimSpace(typ)), "tag": raw["tag"], "settings": settings}
	}
	protocol, ok := out["protocol"].(string)
	if !ok || strings.TrimSpace(protocol) == "" {
		return nil, fmt.Errorf("outbound protocol must be a non-empty string")
	}
	out["protocol"] = strings.ToLower(strings.TrimSpace(protocol))
	tag, ok := out["tag"].(string)
	if !ok || strings.TrimSpace(tag) == "" || tag != strings.TrimSpace(tag) {
		return nil, fmt.Errorf("outbound tag must be a non-empty string without surrounding whitespace")
	}
	for _, key := range []string{"settings", "streamSettings", "mux", "proxySettings"} {
		if v, exists := out[key]; exists && v != nil {
			if _, ok := v.(map[string]any); !ok {
				return nil, fmt.Errorf("outbound %q %s must be an object", tag, key)
			}
		}
	}
	return out, nil
}
