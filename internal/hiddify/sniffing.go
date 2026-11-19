package hiddify

import "github.com/SawaMEN/3x-ui/v3/internal/singbox"

// TranslateSniffingRule uses the panel build's modern sniff action extension.
// Keep exclusions, FakeDNS and metadataOnly fail-closed in the shared translator.
func TranslateSniffingRule(raw map[string]any) (map[string]any, error) {
	return translateSniffingRule(raw, false)
}

// TranslateSniffingRuleWithoutFakeDNS handles Xray's default sniffing list when
// no FakeDNS pool or DNS server exists. FakeDNS is not a packet sniffer in
// sing-box; removing the inactive option preserves HTTP/TLS/QUIC sniffing.
// Callers must verify the Xray DNS template before using this compatibility
// path. The stored inbound is never modified.
func TranslateSniffingRuleWithoutFakeDNS(raw map[string]any) (map[string]any, error) {
	return translateSniffingRule(raw, true)
}

func translateSniffingRule(raw map[string]any, ignoreInactiveFakeDNS bool) (map[string]any, error) {
	copy, err := clone(raw)
	if err != nil {
		return nil, err
	}
	sniff := object(copy, "sniffing")
	routeOnly, _ := sniff["routeOnly"].(bool)
	overrides := configNames(sniff["destOverride"])
	translatedOverrides := overrides
	if ignoreInactiveFakeDNS {
		translatedOverrides = make([]string, 0, len(overrides))
		for _, protocol := range overrides {
			if protocol != "fakedns" && protocol != "fakedns+others" {
				translatedOverrides = append(translatedOverrides, protocol)
			}
		}
		// The shared sing-box decoder consumes JSON-shaped []any, not []string.
		// Keeping the original slice type here made every HTTP/TLS/QUIC
		// sniffer disappear after removing FakeDNS.
		jsonOverrides := make([]any, 0, len(translatedOverrides))
		for _, protocol := range translatedOverrides {
			jsonOverrides = append(jsonOverrides, protocol)
		}
		sniff["destOverride"] = jsonOverrides
	}
	sniff["routeOnly"] = true
	rule, err := singbox.TranslateXraySniffingRule(copy)
	if err != nil || rule == nil {
		return rule, err
	}
	if ignoreInactiveFakeDNS && len(overrides) > 0 && len(translatedOverrides) == 0 {
		// The only requested sniffer was FakeDNS, which is inactive. Do not
		// produce an unfiltered sing-box sniff action as its replacement.
		return nil, nil
	}
	if !routeOnly && len(translatedOverrides) > 0 {
		rule["override_destination"] = true
	}
	return rule, nil
}
