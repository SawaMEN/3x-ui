package hiddify

import "github.com/SawaMEN/3x-ui/v3/internal/singbox"

// TranslateSniffingRule uses the panel build's modern sniff action extension.
// Keep exclusions, FakeDNS and metadataOnly fail-closed in the shared translator.
func TranslateSniffingRule(raw map[string]any) (map[string]any, error) {
	copy, err := clone(raw)
	if err != nil {
		return nil, err
	}
	sniff := object(copy, "sniffing")
	routeOnly, _ := sniff["routeOnly"].(bool)
	overrides := configNames(sniff["destOverride"])
	sniff["routeOnly"] = true
	rule, err := singbox.TranslateXraySniffingRule(copy)
	if err != nil || rule == nil {
		return rule, err
	}
	if !routeOnly && len(overrides) > 0 {
		rule["override_destination"] = true
	}
	return rule, nil
}
