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
		if err := normalizeHysteriaShareAuth(out, tag); err != nil {
			return err
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

// normalizeHysteriaShareAuth accepts both common URI-decoder shapes: a plain
// string and a one-element string array. The latter otherwise reaches the
// kernel as auth[0] and fails JSON unmarshalling into a uint8.
func normalizeHysteriaShareAuth(out map[string]any, tag string) error {
	value, exists := out["auth"]
	if !exists {
		return nil
	}
	var auth string
	switch typed := value.(type) {
	case string:
		auth = typed
	case []string:
		if len(typed) == 0 {
			delete(out, "auth")
			return nil
		}
		if len(typed) != 1 {
			return fmt.Errorf("hysteria outbound %q requires a single authentication string", tag)
		}
		auth = typed[0]
	case []any:
		if len(typed) == 0 {
			delete(out, "auth")
			return nil
		}
		if first, ok := typed[0].(string); ok {
			if len(typed) != 1 {
				return fmt.Errorf("hysteria outbound %q requires a single authentication string", tag)
			}
			auth = first
		} else {
			// Proper native JSON may contain a byte array; preserve it.
			return nil
		}
	default:
		return nil
	}
	if auth != "" {
		if existing, ok := out["auth_str"].(string); !ok || existing == "" {
			out["auth_str"] = auth
		}
	}
	delete(out, "auth")
	return nil
}
