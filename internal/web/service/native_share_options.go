package service

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

//go:embed native_import_options.json
var nativeImportOptionData []byte

var nativeImportOptionTypes = func() map[string]map[string]string {
	var catalog map[string]map[string]string
	if err := json.Unmarshal(nativeImportOptionData, &catalog); err != nil {
		panic(err)
	}
	return catalog
}()

func normalizedOptionName(key string) string {
	var name strings.Builder
	for _, r := range key {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' {
			name.WriteRune(unicode.ToLower(r))
		}
	}
	return name.String()
}

// Apply typed native parameters even when the kernel's legacy URI converter
// predates them. Complex/future settings are carried by options={} or a native
// singbox:// URI without any panel whitelist or scalar coercion.
func applyNativeShareOptions(out map[string]any, query url.Values) error {
	kind, _ := out["type"].(string)
	catalog := map[string]string{}
	for key, value := range nativeImportOptionTypes[kind] {
		catalog[key] = value
	}
	if transport, ok := out["transport"].(map[string]any); ok {
		typ, _ := transport["type"].(string)
		for key, value := range nativeImportOptionTypes["__transport:"+typ] {
			catalog["transport."+key] = value
		}
	}
	aliases := map[string]string{}
	for key := range catalog {
		aliases[normalizedOptionName(key)] = key
	}
	// Standard URI converters often predate new native transport knobs.
	// Resolve unqualified names only within the selected transport's schema.
	if transport, ok := out["transport"].(map[string]any); ok {
		typ, _ := transport["type"].(string)
		for key := range nativeImportOptionTypes["__transport:"+typ] {
			if key != "type" && !strings.Contains(key, ".") {
				name := normalizedOptionName(key)
				if _, exists := aliases[name]; !exists {
					aliases[name] = "transport." + key
				}
			}
		}
	}
	if _, ok := out["tls"].(map[string]any); ok {
		for path := range catalog {
			if strings.HasPrefix(path, "tls.") && strings.Count(path, ".") == 1 {
				name := normalizedOptionName(strings.TrimPrefix(path, "tls."))
				if _, exists := aliases[name]; !exists {
					aliases[name] = path
				}
			}
		}
	}
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := strings.Count(keys[i], "."), strings.Count(keys[j], ".")
		if a != b {
			return a < b
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		values := query[key]
		if len(values) == 0 || key == "type" || key == "tag" || key == "flow" {
			continue
		}
		path, known := aliases[normalizedOptionName(key)]
		if !known {
			continue
		}
		value := values[len(values)-1]
		if path == "packet_encoding" && value == "none" {
			out["packet_encoding"] = ""
			continue
		}
		if value == "" {
			continue
		}
		var decoded any
		switch catalog[path] {
		case "string":
			decoded = value
		case "boolean":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid boolean native option %s", path)
			}
			decoded = b
		case "number":
			n, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("invalid numeric native option %s", path)
			}
			decoded = n
		case "array":
			if strings.HasPrefix(value, "[") {
				if json.Unmarshal([]byte(value), &decoded) != nil {
					return fmt.Errorf("invalid array native option %s", path)
				}
			} else {
				decoded = strings.Split(value, ",")
			}
		case "json":
			if json.Unmarshal([]byte(value), &decoded) != nil {
				decoded = value
			}
		default:
			if !strings.Contains(path, ".") && !strings.HasPrefix(value, "{") && !strings.HasPrefix(value, "[") {
				continue
			}
			if json.Unmarshal([]byte(value), &decoded) != nil {
				return fmt.Errorf("native option %s requires JSON", path)
			}
		}
		target := out
		parts := strings.Split(path, ".")
		for _, part := range parts[:len(parts)-1] {
			child, ok := target[part].(map[string]any)
			if !ok {
				child = map[string]any{}
				target[part] = child
			}
			target = child
		}
		leaf := parts[len(parts)-1]
		if fields, ok := decoded.(map[string]any); ok {
			if current, ok := target[leaf].(map[string]any); ok {
				mergeNativeOptions(current, fields)
				continue
			}
		}
		target[leaf] = decoded
	}
	return nil
}
