package sub

import (
	"maps"
	"strings"
)

// Match Xray's extra precedence: only host/path/mode survive outside extra.
// Return an independent map because subscription generation removes server fields.
func effectiveXhttpOptions(raw map[string]any) map[string]any {
	if extra, ok := raw["extra"].(map[string]any); ok {
		out := maps.Clone(extra)
		for _, key := range []string{"host", "path", "mode"} {
			delete(out, key)
			if value, exists := raw[key]; exists {
				out[key] = value
			}
		}
		return normalizeXhttpHost(out)
	}
	return normalizeXhttpHost(maps.Clone(raw))
}

func normalizeXhttpHost(options map[string]any) map[string]any {
	if headers, ok := options["headers"].(map[string]any); ok {
		copy := maps.Clone(headers)
		for name, value := range copy {
			if strings.EqualFold(name, "host") {
				if host, _ := options["host"].(string); host == "" {
					options["host"] = value
				}
				delete(copy, name)
			}
		}
		options["headers"] = copy
	}
	return options
}
