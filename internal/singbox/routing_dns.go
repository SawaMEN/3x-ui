package singbox

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// TranslateXrayRouting provides a compile-safe compatibility layer for the
// panel's Xray routing configuration. Unsupported legacy matchers are rejected
// instead of being emitted as invalid sing-box fields.
func TranslateXrayRouting(raw map[string]any) (map[string]any, error) {
	out := map[string]any{}
	rulesRaw, _ := raw["rules"].([]any)
	rules := make([]map[string]any, 0, len(rulesRaw))
	for i, item := range rulesRaw {
		xr, ok := item.(map[string]any)
		if !ok {
			continue
		}
		r := map[string]any{}
		inboundTags := compatStringSlice(xr["inboundTag"])
		if len(inboundTags) > 0 {
			r["inbound"] = inboundTags
		}
		if domains := compatStringSlice(xr["domain"]); len(domains) > 0 {
			if err := translateCompatDomains(r, domains); err != nil {
				return nil, fmt.Errorf("routing rule %d: %w", i, err)
			}
		}
		if ips := compatStringSlice(xr["ip"]); len(ips) > 0 {
			cidrs := make([]string, 0, len(ips))
			for _, ip := range ips {
				if strings.EqualFold(ip, "geoip:private") {
					// Xray's geoip:private is a semantic matcher for private
					// addresses. sing-box has the native equivalent.
					r["ip_is_private"] = true
					continue
				}
				if net.ParseIP(ip) == nil {
					if _, _, err := net.ParseCIDR(ip); err != nil {
						return nil, fmt.Errorf("routing rule %d: unsupported IP matcher %q", i, ip)
					}
				}
				cidrs = append(cidrs, ip)
			}
			if len(cidrs) > 0 {
				r["ip_cidr"] = cidrs
			}
		}
		if port := compatString(xr["port"]); port != "" {
			r["port"] = port
		}
		if sourcePort := compatString(xr["sourcePort"]); sourcePort != "" {
			r["source_port"] = sourcePort
		}
		if network := compatString(xr["network"]); network != "" {
			parts := strings.Split(network, ",")
			validated := make([]string, 0, len(parts))
			for _, part := range parts {
				part = strings.TrimSpace(strings.ToLower(part))
				if part == "" {
					continue
				}
				if part != "tcp" && part != "udp" {
					return nil, fmt.Errorf("routing rule %d: unsupported network %q", i, network)
				}
				validated = append(validated, part)
			}
			switch len(validated) {
			case 0:
				return nil, fmt.Errorf("routing rule %d: unsupported network %q", i, network)
			case 1:
				r["network"] = validated[0]
			default:
				r["network"] = validated
			}
		}
		if users := compatStringSlice(xr["user"]); len(users) > 0 {
			r["user"] = users
		}
		if protocols := compatStringSlice(xr["protocol"]); len(protocols) > 0 {
			r["protocol"] = protocols
		}
		if outbound := compatString(xr["outboundTag"]); outbound != "" {
			// The stock Xray template contains an internal "api" inbound/rule used
			// only by Xray's own API. sing-box has no matching Xray API outbound,
			// so carrying the rule over references a nonexistent outbound and makes
			// sing-box reject the complete configuration.
			if outbound == "api" && containsCompatString(inboundTags, "api") {
				continue
			}
			r["action"] = "route"
			r["outbound"] = outbound
		} else if balancer := compatString(xr["balancerTag"]); balancer != "" {
			// The compatibility layer materializes Xray balancers as sing-box
			// selector outbounds with the same tag.
			r["action"] = "route"
			r["outbound"] = balancer
		} else {
			return nil, fmt.Errorf("routing rule %d has no outboundTag", i)
		}
		rules = append(rules, r)
	}
	if len(rules) > 0 {
		out["rules"] = rules
	}
	return out, nil
}

func TranslateXrayDomainStrategy(value string) string {
	switch strings.TrimSpace(value) {
	case "UseIPv4", "UseIPv4v6":
		return "prefer_ipv4"
	case "UseIPv6", "UseIPv6v4":
		return "prefer_ipv6"
	case "ForceIPv4":
		return "ipv4_only"
	case "ForceIPv6":
		return "ipv6_only"
	default:
		return ""
	}
}

// TranslateXrayBalancers converts the simple selector portion of Xray
// balancers into native sing-box selector outbounds. Health/least-load
// strategies are not directly equivalent, so the first configured selector
// is used as the deterministic default unless Xray's fallbackTag is itself
// one of the selected outbounds.
func TranslateXrayBalancers(raw map[string]any) ([]map[string]any, error) {
	items, _ := raw["balancers"].([]any)
	if len(items) == 0 {
		return nil, nil
	}
	result := make([]map[string]any, 0, len(items))
	for i, item := range items {
		balancer, ok := item.(map[string]any)
		if !ok {
			continue
		}
		tag := compatString(balancer["tag"])
		if tag == "" {
			return nil, fmt.Errorf("balancer %d has an empty tag", i)
		}
		selectors := compatStringSlice(balancer["selector"])
		if len(selectors) == 0 {
			return nil, fmt.Errorf("balancer %q has no selectors", tag)
		}
		defaultTag := selectors[0]
		fallback := compatString(balancer["fallbackTag"])
		if containsCompatString(selectors, fallback) {
			defaultTag = fallback
		}
		result = append(result, map[string]any{
			"type":      "selector",
			"tag":       tag,
			"outbounds": selectors,
			"default":   defaultTag,
		})
	}
	return result, nil
}

// RewriteWireGuardRoutes converts route references to Xray WireGuard
// outbound tags into sing-box route targets that point to endpoints.
func RewriteDNSOutboundRoutes(route map[string]any, dnsTags map[string]struct{}) {
	if route == nil || len(dnsTags) == 0 {
		return
	}
	if final := compatString(route["final"]); final != "" {
		if _, ok := dnsTags[final]; ok {
			route["final"] = "direct"
			rule := map[string]any{"action": "hijack-dns"}
			rules, _ := route["rules"].([]map[string]any)
			rules = append([]map[string]any{rule}, rules...)
			route["rules"] = rules
		}
	}
	rules, _ := route["rules"].([]map[string]any)
	for _, rule := range rules {
		outbound := compatString(rule["outbound"])
		if outbound == "" {
			continue
		}
		if _, ok := dnsTags[outbound]; ok {
			delete(rule, "outbound")
			rule["action"] = "hijack-dns"
		}
	}
}

func RewriteWireGuardRoutes(route map[string]any, endpointTags map[string]struct{}) {
	if len(endpointTags) == 0 || route == nil {
		return
	}
	rules, _ := route["rules"].([]map[string]any)
	if final := compatString(route["final"]); final != "" {
		if _, ok := endpointTags[final]; ok {
			route["final"] = "direct"
			rules = append([]map[string]any{{"action": "route", "endpoint": final}}, rules...)
		}
	}
	for _, rule := range rules {
		outbound := compatString(rule["outbound"])
		if outbound == "" {
			continue
		}
		if _, ok := endpointTags[outbound]; ok {
			delete(rule, "outbound")
			rule["action"] = "route"
			rule["endpoint"] = outbound
		}
	}
	route["rules"] = rules
}

func containsCompatString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func translateCompatDomains(dst map[string]any, domains []string) error {
	var exact, suffix, keyword, regex []string
	for _, value := range domains {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "!") || strings.HasPrefix(value, "geosite:") || strings.HasPrefix(value, "ext:") || strings.HasPrefix(value, "dotless:") {
			return fmt.Errorf("unsupported domain matcher %q", value)
		}
		switch {
		case strings.HasPrefix(value, "regexp:"):
			regex = append(regex, strings.TrimPrefix(value, "regexp:"))
		case strings.HasPrefix(value, "domain:"):
			suffix = append(suffix, strings.TrimPrefix(value, "domain:"))
		case strings.HasPrefix(value, "keyword:"):
			keyword = append(keyword, strings.TrimPrefix(value, "keyword:"))
		case strings.HasPrefix(value, "full:"):
			exact = append(exact, strings.TrimPrefix(value, "full:"))
		default:
			exact = append(exact, value)
		}
	}
	if len(exact) > 0 {
		dst["domain"] = exact
	}
	if len(suffix) > 0 {
		dst["domain_suffix"] = suffix
	}
	if len(keyword) > 0 {
		dst["domain_keyword"] = keyword
	}
	if len(regex) > 0 {
		dst["domain_regex"] = regex
	}
	return nil
}

// TranslateXrayDNS converts the common Xray DNS server forms used by the
// panel to the generic sing-box DNS object without depending on sing-box
// internal Go packages.
func TranslateXrayDNS(raw map[string]any) (map[string]any, error) {
	out := map[string]any{}
	serversRaw, _ := raw["servers"].([]any)
	servers := make([]map[string]any, 0, len(serversRaw))
	for i, item := range serversRaw {
		var address string
		switch value := item.(type) {
		case string:
			address = strings.TrimSpace(value)
		case map[string]any:
			address = strings.TrimSpace(compatString(value["address"]))
		default:
			continue
		}
		if address == "" {
			continue
		}
		server := map[string]any{"tag": fmt.Sprintf("dns-%d", i+1)}
		if strings.EqualFold(address, "localhost") || strings.EqualFold(address, "local") {
			server["type"] = "local"
		} else {
			if strings.HasPrefix(address, "https://") || strings.HasPrefix(address, "h3://") {
				server["type"] = "https"
				defaultPort := 443
				if strings.HasPrefix(address, "h3://") {
					server["type"] = "h3"
				}
				if u, err := url.Parse(address); err == nil && u.Hostname() != "" {
					server["server"] = u.Hostname()
					server["server_port"] = parseCompatPort(u.Port(), defaultPort)
					if u.EscapedPath() != "" {
						server["path"] = u.EscapedPath()
					}
				} else {
					return nil, fmt.Errorf("DNS server %d has invalid address %q", i, address)
				}
			} else if strings.HasPrefix(address, "tls://") {
				server["type"] = "tls"
				if u, err := url.Parse(address); err == nil && u.Hostname() != "" {
					server["server"] = u.Hostname()
					server["server_port"] = parseCompatPort(u.Port(), 853)
				} else {
					return nil, fmt.Errorf("DNS server %d has invalid TLS address %q", i, address)
				}
			} else if strings.HasPrefix(address, "quic://") {
				server["type"] = "quic"
				if u, err := url.Parse(address); err == nil && u.Hostname() != "" {
					server["server"] = u.Hostname()
					server["server_port"] = parseCompatPort(u.Port(), 853)
				} else {
					return nil, fmt.Errorf("DNS server %d has invalid QUIC address %q", i, address)
				}
			} else {
				server["type"] = "udp"
				clean := strings.TrimPrefix(address, "udp://")
				if u, err := url.Parse("udp://" + clean); err == nil && u.Hostname() != "" {
					server["server"] = u.Hostname()
					server["server_port"] = parseCompatPort(u.Port(), 53)
				} else {
					return nil, fmt.Errorf("DNS server %d has invalid UDP address %q", i, address)
				}
			}
		}
		servers = append(servers, server)
	}
	if len(servers) > 0 {
		out["servers"] = servers
		out["final"] = servers[0]["tag"]
	}
	if clientIP := compatString(raw["clientIp"]); clientIP != "" {
		if net.ParseIP(clientIP) == nil {
			return nil, fmt.Errorf("DNS has invalid clientIp %q", clientIP)
		}
		out["client_subnet"] = clientIP
	}
	return out, nil
}

func compatString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func compatStringSlice(v any) []string {
	switch values := v.(type) {
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if s := compatString(value); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return values
	case string:
		if s := strings.TrimSpace(values); s != "" {
			return []string{s}
		}
	}
	return nil
}

func parseCompatPort(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return fallback
	}
	return port
}
