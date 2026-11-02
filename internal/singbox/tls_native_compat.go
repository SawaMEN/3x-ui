package singbox

import (
	"fmt"
	"maps"
)

// Final serialization also sees native inbounds and template overlays, which
// never pass through the Xray translators. Work on copies so normalization
// cannot rewrite the saved template or the configuration used for rollback.
func normalizeRuntimeTLSConfig(cfg *configJSON) error {
	for _, section := range []struct {
		name  string
		nodes *[]map[string]any
	}{
		{"inbounds", &cfg.Inbounds},
		{"outbounds", &cfg.Outbounds},
		{"endpoints", &cfg.Endpoints},
		{"services", &cfg.Services},
		{"http_clients", &cfg.HTTPClients},
		{"certificate_providers", &cfg.CertificateProviders},
	} {
		if *section.nodes == nil {
			continue
		}
		nodes := *section.nodes
		changed := false
		for i, node := range *section.nodes {
			label := fmt.Sprintf("%s[%d] %q", section.name, i, rawString(node, "tag"))
			value, modified, err := normalizeRuntimeTLSValue(node, label)
			if err != nil {
				return err
			}
			if modified {
				if !changed {
					nodes = append([]map[string]any(nil), nodes...)
					changed = true
				}
				nodes[i] = value.(map[string]any)
			}
		}
		*section.nodes = nodes
	}
	for _, section := range []struct {
		name string
		node *map[string]any
	}{
		{"dns", &cfg.DNS}, {"ntp", &cfg.NTP},
	} {
		if *section.node == nil {
			continue
		}
		value, _, err := normalizeRuntimeTLSValue(*section.node, section.name)
		if err != nil {
			return err
		}
		*section.node = value.(map[string]any)
	}
	return nil
}

// Copy only ancestors of rewritten TLS fields. All other subtrees are read-only
// and go directly to json.Marshal; the persisted configuration stays untouched.
func normalizeRuntimeTLSValue(value any, path string) (any, bool, error) {
	switch v := value.(type) {
	case map[string]any:
		out := v
		changed := false
		for key, child := range v {
			switch child.(type) {
			case map[string]any, []any, []map[string]any:
			default:
				continue
			}
			childPath := path + "." + key
			normalized, modified, err := normalizeRuntimeTLSValue(child, childPath)
			if err != nil {
				return nil, false, err
			}
			if key == "tls" {
				if tls, ok := normalized.(map[string]any); ok && tls["cipher_suites"] != nil {
					if !modified {
						tls = maps.Clone(tls)
					}
					if err := normalizeNativeTLSCipherSuites(tls, childPath); err != nil {
						return nil, false, err
					}
					normalized, modified = tls, true
				}
			}
			if modified {
				if !changed {
					out = maps.Clone(v)
					changed = true
				}
				out[key] = normalized
			}
		}
		return out, changed, nil
	case []any:
		out := v
		changed := false
		for i, child := range v {
			switch child.(type) {
			case map[string]any, []any, []map[string]any:
			default:
				continue
			}
			normalized, modified, err := normalizeRuntimeTLSValue(child, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, false, err
			}
			if modified {
				if !changed {
					out = append([]any(nil), v...)
					changed = true
				}
				out[i] = normalized
			}
		}
		return out, changed, nil
	case []map[string]any:
		out := v
		changed := false
		for i, child := range v {
			normalized, modified, err := normalizeRuntimeTLSValue(child, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, false, err
			}
			if modified {
				if !changed {
					out = append([]map[string]any(nil), v...)
					changed = true
				}
				out[i] = normalized.(map[string]any)
			}
		}
		return out, changed, nil
	default:
		return value, false, nil
	}
}

func normalizeNativeTLSCipherSuites(tls map[string]any, label string) error {
	value := tls["cipher_suites"]
	if value == nil {
		return nil
	}
	var names []string
	switch v := value.(type) {
	case string:
		names = []string{v} // sing-box's Listable also permits a scalar.
	case []string:
		names = v
	case []any:
		names = make([]string, len(v))
		for i, entry := range v {
			name, ok := entry.(string)
			if !ok {
				return fmt.Errorf("%s.cipher_suites[%d] must be a string", label, i)
			}
			names[i] = name
		}
	default:
		return fmt.Errorf("%s.cipher_suites must be a string or an array of strings", label)
	}
	// An explicitly empty list means core defaults.
	if len(names) == 0 {
		return nil
	}
	suites, err := compatibleTLSCipherSuites(names, label)
	if err != nil {
		return err
	}
	tls["cipher_suites"] = suites
	return nil
}
