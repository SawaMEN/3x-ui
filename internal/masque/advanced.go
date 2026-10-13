package masque

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var memorySize = regexp.MustCompile(`(?i)^[0-9]+(?:\.[0-9]+)?\s*(?:[kmgtpe]i?b?)?$`)

func validateAdvanced(options map[string]any) error {
	for key, value := range options {
		if value == nil || value == "" {
			delete(options, key)
			continue
		}
		valid := false
		switch key {
		case "udp_timeout", "idle_timeout", "keep_alive_period":
			text, ok := value.(string)
			duration, err := time.ParseDuration(text)
			valid = ok && err == nil && duration >= 0
		case "stream_receive_window", "connection_receive_window":
			text, ok := value.(string)
			valid = ok && memorySize.MatchString(strings.TrimSpace(text))
		case "bind_interface":
			text, ok := value.(string)
			valid = ok && strings.TrimSpace(text) != ""
		case "max_concurrent_streams", "initial_packet_size", "udp_nat_max", "routing_mark":
			number, ok := value.(float64)
			valid = ok && number >= 0 && number <= 4294967295 && number == float64(uint64(number))
			if key == "initial_packet_size" {
				valid = valid && number >= 1200 && number <= 65527
			}
		case "disable_path_mtu_discovery", "udp_fragment", "tcp_fast_open", "tcp_multi_path", "reuse_addr":
			_, valid = value.(bool)
		default:
			return fmt.Errorf("unsupported MASQUE advanced option %q", key)
		}
		if !valid {
			return fmt.Errorf("invalid MASQUE advanced option %q", key)
		}
	}
	return nil
}
