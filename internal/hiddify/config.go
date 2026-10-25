package hiddify

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/protocolserver"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// FinalizeServerConfig owns Hiddify's server policy after template merging.
// Validate the actual normalized wire graph, not the intermediate Xray schema.
func FinalizeServerConfig(cfg *singbox.Config, unmeteredTags ...string) error {
	if cfg == nil {
		return fmt.Errorf("hiddify server configuration is nil")
	}
	cfg.Services = nil // The pinned profile exposes stats through V2Ray and Clash.
	data, err := cfg.Marshal()
	if err != nil {
		return fmt.Errorf("hiddify server configuration: %w", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	var normalized singbox.Config
	if err := json.Unmarshal(data, &normalized); err != nil {
		return err
	}
	ConfigureStats(&normalized, unmeteredTags...)
	cfg.Experimental = normalized.Experimental
	return ValidateConfigReferences(wire)
}

// ConfigureStats includes native template users and endpoint peers, and produces
// stable sets even when one canonical user is attached to several listeners.
func ConfigureStats(cfg *singbox.Config, unmeteredTags ...string) {
	users, tags, outbounds := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, listener := range append(slices.Clone(cfg.Inbounds), cfg.Endpoints...) {
		// Sidecars account their own traffic. Their routing bridges must not
		// charge the same inbound bytes a second time.
		tag, _ := listener["tag"].(string)
		if slices.Contains(unmeteredTags, tag) {
			continue
		}
		if tag, _ := listener["tag"].(string); tag != "" {
			tags[tag] = true
		}
		for _, key := range []string{"users", "peers"} {
			for _, user := range configObjects(listener[key]) {
				name, _ := user["name"].(string)
				if name == "" {
					name, _ = user["username"].(string)
				}
				if name != "" && !strings.HasPrefix(name, protocolserver.InternalUserPrefix) {
					users[name] = true
				}
			}
		}
	}
	for _, outbound := range cfg.Outbounds {
		if tag, _ := outbound["tag"].(string); tag != "" {
			outbounds[tag] = true
		}
	}
	if cfg.Experimental == nil {
		cfg.Experimental = map[string]any{}
	}
	cfg.Experimental["v2ray_api"] = map[string]any{"listen": "127.0.0.1:10086", "stats": map[string]any{
		"enabled": true, "inbounds": sortedNames(tags), "outbounds": sortedNames(outbounds), "users": sortedNames(users),
	}}
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func configObjects(value any) []map[string]any {
	if objects, ok := value.([]map[string]any); ok {
		return objects
	}
	var objects []map[string]any
	if items, ok := value.([]any); ok {
		for _, item := range items {
			if object, ok := item.(map[string]any); ok {
				objects = append(objects, object)
			}
		}
	}
	return objects
}

func configNames(value any) []string {
	switch value := value.(type) {
	case string:
		if value != "" {
			return []string{value}
		}
	case []string:
		return value
	case []any:
		var names []string
		for _, item := range value {
			if name, ok := item.(string); ok && name != "" {
				names = append(names, name)
			}
		}
		return names
	}
	return nil
}

// ValidateConfigReferences rejects broken references before publication. Schema,
// certificate contents and protocol construction are checked by the pinned core.
func ValidateConfigReferences(cfg map[string]any) error {
	if cfg == nil {
		return fmt.Errorf("hiddify configuration must be an object")
	}
	outbounds, listeners, dns, ruleSets := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	add := func(set map[string]bool, name, kind string) error {
		if name == "" {
			return nil
		}
		if set[name] {
			return fmt.Errorf("hiddify duplicate %s tag %q", kind, name)
		}
		set[name] = true
		return nil
	}
	for _, key := range []string{"outbounds", "endpoints"} {
		for _, item := range configObjects(cfg[key]) {
			name, _ := item["tag"].(string)
			if err := add(outbounds, name, "outbound/endpoint"); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"inbounds", "endpoints"} {
		for _, item := range configObjects(cfg[key]) {
			name, _ := item["tag"].(string)
			if err := add(listeners, name, "listener"); err != nil {
				return err
			}
		}
	}
	resolver, _ := cfg["dns"].(map[string]any)
	for _, item := range configObjects(resolver["servers"]) {
		name, _ := item["tag"].(string)
		if err := add(dns, name, "DNS server"); err != nil {
			return err
		}
	}
	route, _ := cfg["route"].(map[string]any)
	for _, item := range configObjects(route["rule_set"]) {
		name, _ := item["tag"].(string)
		if err := add(ruleSets, name, "rule set"); err != nil {
			return err
		}
	}
	require := func(set map[string]bool, value any, path, kind string) error {
		for _, name := range configNames(value) {
			if !set[name] {
				return fmt.Errorf("sing-box %s references missing %s %q", path, kind, name)
			}
		}
		return nil
	}
	if err := require(outbounds, route["final"], "route.final", "outbound"); err != nil {
		return err
	}
	if err := require(dns, resolver["final"], "dns.final", "DNS server"); err != nil {
		return err
	}
	var walk func(any, string, bool) error
	walk = func(value any, path string, dnsRule bool) error {
		switch value := value.(type) {
		case map[string]any:
			// Header and hosts dictionaries contain user data, not config references.
			if strings.HasSuffix(path, ".headers") || strings.HasSuffix(path, ".predefined") {
				return nil
			}
			// Sort fields so errors and validation order are deterministic.
			for _, key := range sortedMapKeys(value) {
				item := value[key]
				location := path + "." + key
				var err error
				switch key {
				case "detour", "download_detour", "external_ui_download_detour":
					if key == "detour" && strings.HasPrefix(path, "inbounds[") && !strings.Contains(path, ".") {
						err = require(listeners, item, location, "inbound")
					} else {
						err = require(outbounds, item, location, "outbound")
					}
				case "rule_set":
					if path != "route" {
						err = require(ruleSets, item, location, "rule set")
					}
				case "domain_resolver", "default_domain_resolver":
					if object, ok := item.(map[string]any); ok {
						item = object["server"]
					}
					err = require(dns, item, location, "DNS server")
				case "outbound":
					if !dnsRule && strings.HasPrefix(path, "route.rules[") {
						err = require(outbounds, item, location, "outbound")
					}
				case "server":
					if dnsRule {
						err = require(dns, item, location, "DNS server")
					}
				}
				if err != nil {
					return err
				}
				if err := walk(value[key], location, dnsRule); err != nil {
					return err
				}
			}
		case []any:
			for i, item := range value {
				if err := walk(item, fmt.Sprintf("%s[%d]", path, i), dnsRule); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, key := range []string{"outbounds", "endpoints", "inbounds", "route", "experimental", "dns"} {
		if key == "dns" {
			continue
		}
		if err := walk(cfg[key], key, false); err != nil {
			return err
		}
	}
	if err := walk(resolver["servers"], "dns.servers", false); err != nil {
		return err
	}
	if err := walk(resolver["rules"], "dns.rules", true); err != nil {
		return err
	}
	// Groups and dialer detours share the outbound namespace. Reject cycles.
	edges := map[string][]string{}
	for _, key := range []string{"outbounds", "endpoints"} {
		for _, item := range configObjects(cfg[key]) {
			name, _ := item["tag"].(string)
			for _, field := range []string{"outbounds", "detour"} {
				edges[name] = append(edges[name], configNames(item[field])...)
			}
			if err := require(outbounds, item["outbounds"], name+".outbounds", "outbound"); err != nil {
				return err
			}
			if selection, _ := item["default"].(string); selection != "" && !slices.Contains(configNames(item["outbounds"]), selection) {
				return fmt.Errorf("hiddify selector %q default %q is not a member", name, selection)
			}
		}
	}
	if err := validateDependencyCycles(outbounds, edges, "outbound"); err != nil {
		return err
	}
	listenerEdges := map[string][]string{}
	for _, item := range configObjects(cfg["inbounds"]) {
		name, _ := item["tag"].(string)
		listenerEdges[name] = configNames(item["detour"])
	}
	if err := validateDependencyCycles(listeners, listenerEdges, "inbound"); err != nil {
		return err
	}
	dnsEdges := map[string][]string{}
	for _, item := range configObjects(resolver["servers"]) {
		name, _ := item["tag"].(string)
		members := configNames(item["servers"])
		if err := require(dns, item["servers"], name+".servers", "DNS server"); err != nil {
			return err
		}
		dependency := item["domain_resolver"]
		if object, ok := dependency.(map[string]any); ok {
			dependency = object["server"]
		}
		dnsEdges[name] = append(members, configNames(dependency)...)
	}
	if err := validateDependencyCycles(dns, dnsEdges, "DNS server"); err != nil {
		return err
	}
	dnsTypes := map[string]string{}
	for _, item := range configObjects(resolver["servers"]) {
		name, _ := item["tag"].(string)
		dnsTypes[name], _ = item["type"].(string)
	}
	for _, name := range sortedNames(dns) {
		if dnsTypes[name] != "group" && dnsTypes[name] != "multi" {
			continue
		}
		for _, member := range dnsEdges[name] {
			switch dnsTypes[member] {
			case "group", "multi", "fakeip":
				return fmt.Errorf("hiddify DNS group %q cannot contain %s server %q", name, dnsTypes[member], member)
			}
		}
	}
	return nil
}

func validateDependencyCycles(names map[string]bool, edges map[string][]string, kind string) error {
	state := map[string]int{}
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == 1 {
			return fmt.Errorf("sing-box %s dependency cycle at %q", kind, name)
		}
		if state[name] == 2 {
			return nil
		}
		state[name] = 1
		for _, next := range edges[name] {
			if err := visit(next); err != nil {
				return err
			}
		}
		state[name] = 2
		return nil
	}
	for _, name := range sortedNames(names) {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func sortedMapKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
