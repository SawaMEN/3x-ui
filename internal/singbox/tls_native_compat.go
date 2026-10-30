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
		nodes := make([]map[string]any, len(*section.nodes))
		for i, node := range *section.nodes {
			label := fmt.Sprintf("%s[%d] %q", section.name, i, rawString(node, "tag"))
			value, err := normalizeRuntimeTLSValue(node, label)
			if err != nil {
				return err
			}
			nodes[i] = value.(map[string]any)
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
		value, err := normalizeRuntimeTLSValue(*section.node, section.name)
		if err != nil {
			return err
		}
		*section.node = value.(map[string]any)
	}
	return nil
}

func normalizeRuntimeTLSValue(value any, path string) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		out := maps.Clone(v)
		for key, child := range v {
			normalized, err := normalizeRuntimeTLSValue(child, path+"."+key)
			if err != nil {
				return nil, err
			}
			if key == "tls" {
				if tls, ok := normalized.(map[string]any); ok {
					if err := normalizeNativeTLSCipherSuites(tls, path+".tls"); err != nil {
						return nil, err
					}
				}
			}
			out[key] = normalized
		}
		return out, nil
	case []any:
		if v == nil {
			return value, nil
		}
		out := make([]any, len(v))
		for i, child := range v {
			normalized, err := normalizeRuntimeTLSValue(child, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			out[i] = normalized
		}
		return out, nil
	case []map[string]any:
		if v == nil {
			return value, nil
		}
		out := make([]map[string]any, len(v))
		for i, child := range v {
			normalized, err := normalizeRuntimeTLSValue(child, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			out[i] = normalized.(map[string]any)
		}
		return out, nil
	default:
		return value, nil
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
