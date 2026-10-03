package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/util/link"
)

// parseOutboundSubscriptionBody accepts both share-link subscriptions and JSON.
// JSON is intentionally protocol-agnostic: an Xray-shaped object may use any
// non-empty `protocol`, while an official sing-box object may use any non-empty
// `type`. Compatibility is checked later against the selected core, not against
// a hard-coded panel allowlist.
func parseOutboundSubscriptionBody(body []byte) ([]link.Outbound, []string, []string, error) {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return nil, nil, nil, nil
	}

	if decoded, ok := decodeOutboundSubscriptionBase64(text); ok {
		text = strings.TrimSpace(decoded)
	}

	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return parseJSONOutboundSubscription(text)
	}

	lines := strings.FieldsFunc(strings.ReplaceAll(text, `\n`, "\n"), func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	outbounds := make([]link.Outbound, 0, len(lines))
	identities := make([]string, 0, len(lines))
	issues := make([]string, 0)
	seen := map[string]int{}

	profileIndex := 0
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		profileIndex++
		res, err := link.ParseLink(line)
		if err != nil || res == nil {
			scheme := "unknown"
			if pos := strings.Index(line, "://"); pos > 0 {
				scheme = strings.ToLower(strings.TrimSpace(line[:pos]))
			}
			if err == nil {
				err = fmt.Errorf("profile could not be parsed")
			}
			issues = append(issues, fmt.Sprintf("profile %d [%s]: %v", profileIndex, scheme, err))
			continue
		}

		identity := res.Identity
		if n := seen[res.Identity]; n > 0 {
			identity = fmt.Sprintf("%s#%d", res.Identity, n)
		}
		seen[res.Identity]++
		outbounds = append(outbounds, res.Outbound)
		identities = append(identities, identity)
	}

	return outbounds, identities, issues, nil
}

func decodeOutboundSubscriptionBase64(text string) (string, bool) {
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, text)
	if clean == "" {
		return "", false
	}
	padded := clean
	for len(padded)%4 != 0 {
		padded += "="
	}
	if decoded, err := base64.StdEncoding.DecodeString(padded); err == nil {
		return string(decoded), true
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(clean); err == nil {
		return string(decoded), true
	}
	if decoded, err := base64.URLEncoding.DecodeString(padded); err == nil {
		return string(decoded), true
	}
	return "", false
}

func parseJSONOutboundSubscription(text string) ([]link.Outbound, []string, []string, error) {
	var root any
	if err := json.Unmarshal([]byte(text), &root); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid outbound subscription JSON: %w", err)
	}

	var rawMembers []any
	switch value := root.(type) {
	case []any:
		rawMembers = value
	case map[string]any:
		if outbounds, ok := value["outbounds"].([]any); ok {
			rawMembers = outbounds
		} else {
			rawMembers = []any{value}
		}
	default:
		return nil, nil, nil, fmt.Errorf("outbound subscription JSON must be an object or array")
	}

	outbounds := make([]link.Outbound, 0, len(rawMembers))
	identities := make([]string, 0, len(rawMembers))
	issues := make([]string, 0)
	seen := map[string]int{}

	for index, member := range rawMembers {
		raw, ok := member.(map[string]any)
		if !ok {
			issues = append(issues, fmt.Sprintf("JSON profile %d: expected an object", index+1))
			continue
		}

		normalized, err := normalizeJSONOutbound(raw)
		if err != nil {
			issues = append(issues, fmt.Sprintf("JSON profile %d: %v", index+1, err))
			continue
		}

		identityRaw := make(map[string]any, len(normalized))
		for key, value := range normalized {
			if key != "tag" {
				identityRaw[key] = value
			}
		}
		encoded, err := json.Marshal(identityRaw)
		if err != nil {
			issues = append(issues, fmt.Sprintf("JSON profile %d: cannot build stable identity: %v", index+1, err))
			continue
		}
		sum := sha256.Sum256(encoded)
		baseIdentity := fmt.Sprintf("json:%x", sum[:16])
		identity := baseIdentity
		if n := seen[baseIdentity]; n > 0 {
			identity = fmt.Sprintf("%s#%d", baseIdentity, n)
		}
		seen[baseIdentity]++

		outbounds = append(outbounds, link.Outbound(normalized))
		identities = append(identities, identity)
	}

	return outbounds, identities, issues, nil
}

func normalizeJSONOutbound(raw map[string]any) (map[string]any, error) {
	if protocol, ok := raw["protocol"].(string); ok && strings.TrimSpace(protocol) != "" {
		out := make(map[string]any, len(raw))
		for key, value := range raw {
			out[key] = value
		}
		out["protocol"] = strings.ToLower(strings.TrimSpace(protocol))
		return out, nil
	}

	if outboundType, ok := raw["type"].(string); ok && strings.TrimSpace(outboundType) != "" {
		typeName := strings.ToLower(strings.TrimSpace(outboundType))
		settings := make(map[string]any, len(raw))
		for key, value := range raw {
			if key == "type" || key == "tag" {
				continue
			}
			settings[key] = value
		}
		out := map[string]any{
			"protocol": "singbox:" + typeName,
			"settings": settings,
		}
		if tag, ok := raw["tag"].(string); ok {
			out["tag"] = tag
		}
		return out, nil
	}

	return nil, fmt.Errorf("protocol/type is required")
}
