package singbox

import (
	"fmt"
	"strings"
	"time"
)

// translateHysteriaQUICOptions copies the pinned kernel's common QUIC fields
// from either native snake_case or the panel's camelCase compatibility form.
func translateHysteriaQUICOptions(out, settings map[string]any, protocol string, inbound bool) error {
	tag := rawString(out, "tag")
	for _, pair := range [][2]string{
		{"idle_timeout", "idleTimeout"},
		{"keep_alive_period", "keepAlivePeriod"},
		{"stream_receive_window", "streamReceiveWindow"},
		{"connection_receive_window", "connectionReceiveWindow"},
		{"max_concurrent_streams", "maxConcurrentStreams"},
		{"initial_packet_size", "initialPacketSize"},
		{"disable_path_mtu_discovery", "disablePathMtuDiscovery"},
	} {
		value, exists := settings[pair[0]]
		if !exists {
			value, exists = settings[pair[1]]
		}
		if !exists || value == nil {
			continue
		}
		switch pair[0] {
		case "idle_timeout", "keep_alive_period":
			duration, ok := value.(string)
			if !ok {
				return fmt.Errorf("Hysteria %q %s must be a Go duration string", tag, pair[0])
			}
			if duration != "" {
				if parsed, err := time.ParseDuration(duration); err != nil || parsed < 0 {
					return fmt.Errorf("Hysteria %q has invalid %s %q", tag, pair[0], duration)
				}
			}
		case "stream_receive_window", "connection_receive_window":
			switch value.(type) {
			case string:
			case int, int64, float64:
				if _, err := hysteriaBoundedInteger(value, pair[0], 0, 1<<48); err != nil {
					return fmt.Errorf("Hysteria %q: %w", tag, err)
				}
			default:
				return fmt.Errorf("Hysteria %q %s must be a byte size", tag, pair[0])
			}
		case "max_concurrent_streams":
			if _, err := hysteriaBoundedInteger(value, pair[0], 0, 1_000_000); err != nil {
				return fmt.Errorf("Hysteria %q: %w", tag, err)
			}
		case "initial_packet_size":
			if _, err := hysteriaBoundedInteger(value, pair[0], 0, 65535); err != nil {
				return fmt.Errorf("Hysteria %q: %w", tag, err)
			}
		case "disable_path_mtu_discovery":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("Hysteria %q %s must be a boolean", tag, pair[0])
			}
		}
		out[pair[0]] = value
	}
	if protocol == "hysteria2" {
		for _, pair := range [][2]string{
			{"brutal_debug", "brutalDebug"},
		} {
			value, exists := settings[pair[0]]
			if !exists {
				value, exists = settings[pair[1]]
			}
			if exists && value != nil {
				if _, ok := value.(bool); !ok {
					return fmt.Errorf("Hysteria2 %q %s must be a boolean", tag, pair[0])
				}
				out[pair[0]] = value
			}
		}
	}
	if !inbound {
		value, exists := settings["network"]
		if exists && value != nil {
			switch v := value.(type) {
			case string:
				if v != "" && v != "tcp" && v != "udp" {
					return fmt.Errorf("Hysteria2 outbound %q has invalid network %q", tag, v)
				}
			case []string:
				for _, item := range v {
					if item != "tcp" && item != "udp" {
						return fmt.Errorf("Hysteria2 outbound %q has invalid network %q", tag, item)
					}
				}
			case []any:
				for _, raw := range v {
					item, ok := raw.(string)
					if !ok || (item != "tcp" && item != "udp") {
						return fmt.Errorf("Hysteria2 outbound %q has invalid network list", tag)
					}
				}
			default:
				return fmt.Errorf("Hysteria2 outbound %q network must be tcp, udp, or an array", tag)
			}
			out["network"] = value
		}
	}
	return nil
}

func validateHysteriaHopIntervals(out map[string]any, protocol, tag string) error {
	if protocol != "hysteria" && protocol != "hysteria2" {
		return nil
	}
	var base time.Duration
	for _, field := range []string{"hop_interval", "hop_interval_max"} {
		value, exists := out[field]
		if !exists || value == nil || value == "" {
			continue
		}
		raw, ok := value.(string)
		if !ok {
			return fmt.Errorf("Hysteria outbound %q %s must be a duration string", tag, field)
		}
		parsed, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil || parsed <= 0 {
			return fmt.Errorf("Hysteria outbound %q %s must be a positive duration", tag, field)
		}
		if field == "hop_interval" {
			base = parsed
		} else {
			if base == 0 {
				base = 30 * time.Second
			}
			if parsed < base {
				return fmt.Errorf("Hysteria outbound %q hop_interval_max must not be shorter than hop_interval", tag)
			}
		}
	}
	if protocol == "hysteria" && out["hop_interval_max"] != nil {
		return fmt.Errorf("Hysteria v1 outbound %q does not support hop_interval_max", tag)
	}
	return nil
}
