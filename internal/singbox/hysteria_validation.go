package singbox

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Validate the Hysteria-specific options before passing them to the pinned
// sing-box kernel. Invalid QUIC/Gecko options must fail configuration checks,
// not become runtime errors after the previous working config is replaced.
func validateHysteriaOutboundOptions(out map[string]any, protocol, tag string) error {
	if protocol != "hysteria" && protocol != "hysteria2" {
		return nil
	}
	if err := validateHysteriaServerPorts(out["server_ports"], tag); err != nil {
		return err
	}
	if err := validateHysteriaHopIntervals(out, protocol, tag); err != nil {
		return err
	}
	if err := validateRuntimeHysteriaOutbound(out, protocol); err != nil {
		return fmt.Errorf("Hysteria outbound %q: %w", tag, err)
	}
	if stringSliceLength(out["server_ports"]) > 0 && rawInt(out, "server_port") != 0 {
		return fmt.Errorf("Hysteria outbound %q cannot combine server_port with server_ports", tag)
	}
	for _, field := range []string{"up_mbps", "down_mbps"} {
		if value, exists := out[field]; exists {
			if _, err := hysteriaBoundedInteger(value, field, 0, 1_000_000); err != nil {
				return fmt.Errorf("Hysteria outbound %q: %w", tag, err)
			}
		}
	}
	if protocol == "hysteria" {
		return nil
	}
	if rawString(out, "password") == "" {
		return fmt.Errorf("Hysteria2 outbound %q requires a password", tag)
	}
	if profile := rawString(out, "bbr_profile"); profile != "" {
		switch profile {
		case "standard", "conservative", "aggressive":
		default:
			return fmt.Errorf("Hysteria2 outbound %q has invalid bbr_profile %q", tag, profile)
		}
	}
	if isMeaningfulCompatValue(out["ignore_client_bandwidth"]) {
		return fmt.Errorf("Hysteria2 outbound %q cannot use inbound-only ignore_client_bandwidth", tag)
	}
	if value, exists := out["obfs"]; exists && value != nil {
		obfs, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("Hysteria2 outbound %q obfs must be an object", tag)
		}
		if err := validateHysteria2Obfs(obfs, tag); err != nil {
			return err
		}
	}
	return nil
}

func validateHysteriaServerPorts(value any, tag string) error {
	if value == nil {
		return nil
	}
	var ports []string
	switch v := value.(type) {
	case string:
		ports = []string{v}
	case []string:
		ports = v
	case []any:
		for _, item := range v {
			port, ok := item.(string)
			if !ok {
				return fmt.Errorf("Hysteria outbound %q server_ports must contain strings", tag)
			}
			ports = append(ports, port)
		}
	default:
		return fmt.Errorf("Hysteria outbound %q server_ports must be a string or string array", tag)
	}
	for _, entry := range ports {
		parts := strings.Split(entry, ":")
		if len(parts) < 1 || len(parts) > 2 || strings.TrimSpace(entry) != entry {
			return fmt.Errorf("Hysteria outbound %q has invalid server_ports entry %q", tag, entry)
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil || start < 1 || start > 65535 {
			return fmt.Errorf("Hysteria outbound %q has invalid server_ports entry %q", tag, entry)
		}
		if len(parts) == 2 {
			end, err := strconv.Atoi(parts[1])
			if err != nil || end < start || end > 65535 {
				return fmt.Errorf("Hysteria outbound %q has invalid server_ports range %q", tag, entry)
			}
		}
	}
	return nil
}

func hysteriaBoundedInteger(value any, name string, minimum, maximum int64) (int64, error) {
	var number int64
	switch v := value.(type) {
	case int:
		number = int64(v)
	case int64:
		number = v
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || v < float64(minimum) || v > float64(maximum) {
			return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
		}
		number = int64(v)
	default:
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}
	if number < minimum || number > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}
	return number, nil
}

func validateHysteria2Obfs(obfs map[string]any, tag string) error {
	kind := strings.ToLower(strings.TrimSpace(rawString(obfs, "type")))
	switch kind {
	case "salamander", "gecko":
	default:
		return fmt.Errorf("Hysteria2 %q has invalid obfs type %q", tag, kind)
	}
	if strings.TrimSpace(rawString(obfs, "password")) == "" {
		return fmt.Errorf("Hysteria2 %q %s obfs requires a password", tag, kind)
	}
	minimum, maximum := int64(0), int64(0)
	for _, entry := range []struct {
		canonical string
		legacy    string
		target    *int64
	}{
		{"min_packet_size", "minPacketSize", &minimum},
		{"max_packet_size", "maxPacketSize", &maximum},
	} {
		value, exists := obfs[entry.canonical]
		if !exists {
			value, exists = obfs[entry.legacy]
		}
		if !exists {
			continue
		}
		if kind != "gecko" {
			return fmt.Errorf("Hysteria2 %q %s obfs does not support %s", tag, kind, entry.canonical)
		}
		n, err := hysteriaBoundedInteger(value, entry.canonical, 0, 2048)
		if err != nil {
			return fmt.Errorf("Hysteria2 %q: %w", tag, err)
		}
		*entry.target = n
	}
	if kind == "gecko" {
		// Defaults in the pinned Hysteria2 kernel are 512..1200 bytes.
		// Validate explicitly configured bounds against the corresponding
		// implicit default as well as against each other.
		if minimum == 0 {
			minimum = 512
		}
		if maximum == 0 {
			maximum = 1200
		}
		if minimum > maximum {
			return fmt.Errorf("Hysteria2 %q Gecko min_packet_size exceeds max_packet_size", tag)
		}
	}
	return nil
}

func validateHysteria2Masquerade(masquerade map[string]any, tag string) error {
	switch strings.ToLower(strings.TrimSpace(rawString(masquerade, "type"))) {
	case "":
		return nil
	case "proxy":
		value := strings.TrimSpace(rawString(masquerade, "url"))
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("Hysteria2 inbound %q masquerade requires a valid HTTP(S) proxy URL", tag)
		}
	case "file":
		if strings.TrimSpace(rawString(masquerade, "dir")) == "" && strings.TrimSpace(rawString(masquerade, "directory")) == "" {
			return fmt.Errorf("Hysteria2 inbound %q file masquerade requires a directory", tag)
		}
	case "string":
		value, exists := masquerade["statusCode"]
		if !exists {
			value, exists = masquerade["status_code"]
		}
		if exists {
			status, err := hysteriaBoundedInteger(value, "masquerade.status_code", 0, 599)
			if err != nil || (status > 0 && status < 100) {
				return fmt.Errorf("Hysteria2 inbound %q masquerade status_code must be 0 or 100..599", tag)
			}
		}
	default:
		return fmt.Errorf("Hysteria2 inbound %q has an invalid masquerade type", tag)
	}
	return nil
}

func normalizeHysteria2MasqueradeHeaders(value any, tag string) (map[string][]string, error) {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Hysteria2 inbound %q masquerade headers must be an object", tag)
	}
	normalized := make(map[string][]string, len(raw))
	for name, rawValues := range raw {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("Hysteria2 inbound %q masquerade contains an empty header name", tag)
		}
		switch values := rawValues.(type) {
		case string:
			normalized[name] = []string{values}
		case []string:
			normalized[name] = values
		case []any:
			headerValues := make([]string, 0, len(values))
			for _, item := range values {
				v, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("Hysteria2 inbound %q masquerade header %q must contain strings", tag, name)
				}
				headerValues = append(headerValues, v)
			}
			normalized[name] = headerValues
		default:
			return nil, fmt.Errorf("Hysteria2 inbound %q masquerade header %q has an invalid value", tag, name)
		}
	}
	return normalized, nil
}

var hysteria2RealmIDPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")

// The Realm URI and identifier have strict formats in the pinned kernel.
// Validate these before attempting STUN, DNS resolution or hole punching.
func validateHysteria2RealmOptions(realm map[string]any, tag string) error {
	rawURL := strings.TrimSpace(rawString(realm, "server_url"))
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("Hysteria2 outbound %q Realm requires a valid HTTP(S) server_url", tag)
	}
	if !hysteria2RealmIDPattern.MatchString(rawString(realm, "realm_id")) {
		return fmt.Errorf("Hysteria2 outbound %q has invalid realm_id (expected 1..64 alphanumeric, hyphen or underscore characters)", tag)
	}
	var servers []string
	switch values := realm["stun_servers"].(type) {
	case string:
		servers = []string{values}
	case []string:
		servers = values
	case []any:
		for _, value := range values {
			server, ok := value.(string)
			if !ok {
				return fmt.Errorf("Hysteria2 outbound %q Realm STUN servers must be strings", tag)
			}
			servers = append(servers, server)
		}
	default:
		return fmt.Errorf("Hysteria2 outbound %q Realm STUN servers must be a string or string array", tag)
	}
	if len(servers) == 0 {
		return fmt.Errorf("Hysteria2 outbound %q Realm needs at least one STUN server", tag)
	}
	for _, server := range servers {
		if strings.TrimSpace(server) == "" || strings.TrimSpace(server) != server {
			return fmt.Errorf("Hysteria2 outbound %q Realm contains an empty or malformed STUN server", tag)
		}
	}
	return nil
}

// Prevent invalid Hysteria2 credentials from reaching the kernel. A generated
// name-only user otherwise has an empty password and can be misrepresented
// as an active account. Duplicate credentials cannot safely map to two users.
func validateHysteria2InboundUsers(inbound map[string]any, protocol string) error {
	if protocol != "hysteria2" {
		return nil
	}
	users, ok := inbound["users"].([]map[string]any)
	if !ok {
		return nil
	}
	tag := rawString(inbound, "tag")
	seen := make(map[string]int, len(users))
	userIndex := 0
	for _, user := range users {
		userIndex++
		password := rawString(user, "password")
		if strings.TrimSpace(password) == "" {
			return fmt.Errorf("Hysteria2 inbound %q user %d requires a non-empty authentication password", tag, userIndex)
		}
		if previous, exists := seen[password]; exists {
			return fmt.Errorf("Hysteria2 inbound %q user %d duplicates the authentication password of user %d", tag, userIndex, previous)
		}
		seen[password] = userIndex
	}
	return nil
}
