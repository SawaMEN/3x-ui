package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/util/link"
)

var errNoSupportedSubscriptionConnections = errors.New("В подписке нет поддерживаемых подключений")

// Reuse the native editor codecs so new protocols are not silently discarded.
// Import only connections, never a provider's listeners, DNS or routing.
func parseOutboundSubscription(body []byte) ([]link.Outbound, []string, error) {
	rows, identities, _, err := parseOutboundSubscriptionReport(body, false)
	return rows, identities, err
}

// Independent invalid links are reported without discarding valid connections.
// Invalid JSON documents and ambiguous duplicate tags remain atomic failures.
func parseOutboundSubscriptionReport(body []byte, tolerant bool) (result []link.Outbound, resultIDs []string, warnings []string, resultErr error) {
	rows, identities, err := parseOutboundSubscriptionEntries(body, tolerant, &warnings)
	return rows, identities, warnings, err
}

func parseOutboundSubscriptionEntries(body []byte, tolerant bool, warnings *[]string) ([]link.Outbound, []string, error) {
	warn := func(message string) {
		if len(*warnings) < 64 {
			*warnings = append(*warnings, message)
		}
	}
	nativeInfo, nativeErr := os.Stat(singbox.GetBinaryPath())
	useKernel := nativeErr == nil && !nativeInfo.IsDir()

	text := strings.TrimSpace(string(body))
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		compact := strings.Join(strings.Fields(text), "")
		if decoded, err := encoding.DecodeString(compact); err == nil {
			text = strings.TrimSpace(string(decoded))
			break
		}
	}
	var out []link.Outbound
	var identities []string
	seen := map[string]int{}
	add := func(row map[string]any, identity string) error {
		switch row["protocol"] {
		case "singbox:masque-server", "singbox:openvpn-server", "singbox:tunnel_server":
			if tolerant {
				warn("Серверный профиль из подписки пропущен: он не является исходящим подключением")
			} else {
				return fmt.Errorf("subscription cannot import server listeners as cascade clients")
			}
		}
		if len(out) >= 4096 {
			return fmt.Errorf("subscription contains too many connections")
		}
		if identity == "" {
			copy := map[string]any{}
			for key, value := range row {
				if key != "tag" {
					copy[key] = value
				}
			}
			data, err := json.Marshal(copy)
			if err != nil {
				return err
			}
			identity = fmt.Sprintf("native:%x", sha256.Sum256(data))
		}
		base := identity
		if count := seen[base]; count > 0 {
			identity = fmt.Sprintf("%s#%d", base, count)
		}
		seen[base]++
		out = append(out, link.Outbound(row))
		identities = append(identities, identity)
		return nil
	}
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		var value any
		if err := json.Unmarshal([]byte(text), &value); err != nil {
			return nil, nil, fmt.Errorf("invalid subscription JSON")
		}
		var rows []any
		switch v := value.(type) {
		case []any:
			rows = v
		case map[string]any:
			if v["type"] != nil || v["protocol"] != nil {
				rows = []any{v}
			} else {
				for _, section := range []string{"outbounds", "endpoints"} {
					if v[section] == nil {
						continue
					}
					list, ok := v[section].([]any)
					if !ok {
						return nil, nil, fmt.Errorf("subscription %s must be an array", section)
					}
					rows = append(rows, list...)
				}
			}
		}
		objects := make([]map[string]any, 0, len(rows))
		for _, value := range rows {
			if row, ok := value.(map[string]any); ok {
				objects = append(objects, row)
			}
		}
		if err := normalizeSubscriptionTags(objects); err != nil {
			return nil, nil, err
		}
		for i, value := range rows {
			row, ok := value.(map[string]any)
			if !ok {
				if tolerant {
					warn(fmt.Sprintf("Профиль %d: некорректный объект, пропущен", i+1))
					continue
				}
				return nil, nil, fmt.Errorf("subscription connection %d must be an object", i+1)
			}
			normalized, err := singbox.NormalizeOutboundInput(row)
			if err != nil {
				if tolerant {
					warn(fmt.Sprintf("Профиль %d: некорректные параметры, пропущен", i+1))
					if tag, ok := row["tag"].(string); ok && strings.TrimSpace(tag) != "" && tag == strings.TrimSpace(tag) {
						if err := add(map[string]any{"tag": tag, "protocol": "singbox:hinvalid", "settings": map[string]any{"source": row}}, ""); err != nil {
							return nil, nil, err
						}
					}
					continue
				}
				return nil, nil, err
			}
			if err := add(normalized, ""); err != nil {
				return nil, nil, err
			}
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		text = strings.ReplaceAll(text, `\n`, "\n")
		for lineIndex, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
				continue
			}
			if ctx.Err() != nil {
				return nil, nil, fmt.Errorf("subscription parsing timed out")
			}
			if !useKernel && !strings.Contains(line, " -> ") {
				if parsed, err := link.ParseLink(line); err == nil && parsed != nil {
					if err := add(map[string]any(parsed.Outbound), parsed.Identity); err != nil {
						return nil, nil, err
					}
					continue
				}
			}
			bundle, err := ParseNativeOutboundImportContext(ctx, line)
			if err != nil {
				if tolerant {
					kind := strings.ToLower(strings.SplitN(line, ":", 2)[0])
					if len(kind) > 32 || strings.ContainsAny(kind, " /@?\t") {
						kind = "неизвестный"
					}
					warn(fmt.Sprintf("Строка %d: протокол %s или его параметры не поддерживаются ядром; узел пропущен", lineIndex+1, kind))
					continue
				}
				return nil, nil, fmt.Errorf("subscription line %d: unsupported or invalid connection", lineIndex+1)
			}
			rows := append(bundle.Outbounds, bundle.Endpoints...)
			if err := normalizeSubscriptionTags(rows); err != nil {
				return nil, nil, err
			}
			// Namespace independent bundles before merging their detour graphs.
			names := map[string]string{}
			for i, row := range rows {
				tag, _ := row["tag"].(string)
				if names[tag] != "" {
					return nil, nil, fmt.Errorf("subscription line %d contains duplicate tags", lineIndex+1)
				}
				names[tag] = fmt.Sprintf("%s § line-%d-%d", tag, lineIndex, i)
			}
			for _, row := range rows {
				normalized, err := singbox.NormalizeOutboundInput(row)
				if err != nil {
					return nil, nil, err
				}
				identityRow := map[string]any{}
				for key, value := range normalized {
					if key != "tag" {
						identityRow[key] = value
					}
				}
				data, _ := json.Marshal(identityRow)
				identity := fmt.Sprintf("native:%x", sha256.Sum256(data))
				retagNativeShareReferences(normalized, names)
				normalized["tag"] = names[row["tag"].(string)]
				if err := add(normalized, identity); err != nil {
					return nil, nil, err
				}
			}
		}
	}
	if len(out) == 0 {
		if len(*warnings) > 0 {
			return nil, nil, fmt.Errorf("%w: %s", errNoSupportedSubscriptionConnections, strings.Join(*warnings, "; "))
		}
		return nil, nil, fmt.Errorf("subscription contains no supported valid connections")
	}
	return out, identities, nil
}

// Provider labels are optional, unlike tags in an editable panel config.
func normalizeSubscriptionTags(rows []map[string]any) error {
	names := map[string]string{}
	used := map[string]bool{}
	for _, row := range rows {
		original, _ := row["tag"].(string)
		tag := strings.TrimSpace(original)
		if tag == "" {
			continue
		}
		if used[tag] {
			return fmt.Errorf("duplicate subscription connection tag %q", tag)
		}
		used[tag] = true
		names[original] = tag
	}
	for i, row := range rows {
		original, _ := row["tag"].(string)
		tag := strings.TrimSpace(original)
		if tag == "" {
			tag = fmt.Sprintf("profile-%d", i+1)
			for used[tag] {
				tag += "-node"
			}
			used[tag] = true
		}
		retagNativeShareReferences(row, names)
		row["tag"] = tag
	}
	return nil
}
