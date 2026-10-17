package singbox

import (
	"encoding/json"
	"fmt"
	"strings"
)

// normalizeNativeShareOptionTypes repairs legacy URI-decoder shapes before the
// installed kernel validates the outbound. Native JSON with correct field types
// stays unchanged.
func normalizeNativeShareOptionTypes(out map[string]any, protocol, tag string) error {
	if protocol == "hysteria" {
		// Hysteria v1 uses auth (byte array) OR auth_str (plain string).
		// Emitting a string under auth makes sing-box fail JSON unmarshalling.
		if auth, ok := out["auth"].(string); ok {
			if auth != "" {
				if current, exists := out["auth_str"].(string); !exists || current == "" {
					out["auth_str"] = auth
				}
			}
			delete(out, "auth")
		}
	}
	if protocol != "naive" {
		return nil
	}
	raw, ok := out["extra_headers"].(string)
	if !ok {
		return nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		delete(out, "extra_headers")
		return nil
	}
	headers := map[string]any{}
	if strings.HasPrefix(raw, "{") {
		if err := json.Unmarshal([]byte(raw), &headers); err != nil || headers == nil {
			return fmt.Errorf("outbound %q naive extra_headers must be a JSON object", tag)
		}
	} else if strings.Contains(raw, ":") {
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			name, value, found := strings.Cut(line, ":")
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			if !found || name == "" || strings.ContainsAny(name, "\r\n\t ") || strings.ContainsAny(value, "\r\n") {
				return fmt.Errorf("outbound %q naive extra_headers contains invalid HTTP header", tag)
			}
			headers[name] = value
		}
	} else {
		// Some Naive URI converters use a bare host as extra_headers.
		if strings.ContainsAny(raw, "\r\n\t ") {
			return fmt.Errorf("outbound %q naive extra_headers must be a header object", tag)
		}
		headers["Host"] = raw
	}
	if len(headers) == 0 {
		delete(out, "extra_headers")
	} else {
		out["extra_headers"] = headers
	}
	return nil
}
