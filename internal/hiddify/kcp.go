package hiddify

import (
	"encoding/json"
	"fmt"
	"strings"
)

func translateKCP(stream map[string]any) (map[string]any, map[string]any, error) {
	transport := map[string]any{"type": "kcp"}
	if value := stream["kcpSettings"]; value != nil {
		if _, ok := value.(map[string]any); !ok {
			return nil, nil, fmt.Errorf("mKCP settings must be an object")
		}
	}
	var seed, headerType string
	options := object(stream, "kcpSettings")
	fields := map[string]string{"mtu": "mtu", "tti": "tti", "uplinkCapacity": "uplink_capacity", "downlinkCapacity": "downlink_capacity", "cwndMultiplier": "cwnd_multiplier", "maxSendingWindow": "max_sending_window"}
	for key, value := range options {
		if native, ok := fields[key]; ok {
			data, _ := json.Marshal(value)
			var n uint32
			if json.Unmarshal(data, &n) != nil {
				return nil, nil, fmt.Errorf("invalid mKCP %s", key)
			}
			transport[native] = n
			continue
		}
		switch key {
		case "seed":
			if _, ok := value.(string); !ok {
				return nil, nil, fmt.Errorf("mKCP seed must be a string")
			}
			seed = value.(string)
		case "header":
			header, ok := value.(map[string]any)
			if !ok {
				return nil, nil, fmt.Errorf("mKCP header must be an object")
			}
			for field, v := range header {
				if field != "type" && meaningful(v) {
					return nil, nil, fmt.Errorf("unsupported mKCP header.%s", field)
				}
			}
			kind, ok := header["type"].(string)
			if !ok && header["type"] != nil {
				return nil, nil, fmt.Errorf("mKCP header.type must be a string")
			}
			headerType = strings.ToLower(strings.TrimSpace(kind))
			switch headerType {
			case "", "none", "dns", "dtls", "srtp", "utp", "wechat", "wireguard":
			case "wechat-video":
				headerType = "wechat"
			default:
				return nil, nil, fmt.Errorf("unsupported mKCP header type %q", kind)
			}
		default:
			if meaningful(value) {
				return nil, nil, fmt.Errorf("mKCP setting %q has no equivalent in the current transport", key)
			}
		}
	}
	if value := stream["finalmask"]; value != nil {
		if _, ok := value.(map[string]any); !ok {
			return nil, nil, fmt.Errorf("mKCP finalmask must be an object")
		}
	}
	mask := object(stream, "finalmask")
	if value := mask["udp"]; value != nil {
		if _, ok := value.([]any); !ok {
			return nil, nil, fmt.Errorf("mKCP finalmask.udp must be an array")
		}
	}
	var nativeMask map[string]any
	if meaningful(mask["tcp"]) {
		return nil, nil, fmt.Errorf("mKCP supports UDP finalmask only")
	}
	if meaningful(mask["udp"]) {
		if seed != "" || headerType != "" && headerType != "none" {
			return nil, nil, fmt.Errorf("mKCP seed/header cannot be combined with explicit UDP masks")
		}
		nativeMask = mask
	} else if _, hasSeed := options["seed"]; hasSeed || options["header"] != nil {
		// Legacy mKCP always has the checksum/encryption layer, even with an
		// empty seed. The header wraps that layer on the wire and comes last.
		udp := []any{map[string]any{"type": "mkcp-legacy", "settings": map[string]any{"value": seed}}}
		if headerType != "" && headerType != "none" {
			udp = append(udp, map[string]any{"type": "mkcp-legacy", "settings": map[string]any{"header": headerType}})
		}
		nativeMask = map[string]any{"udp": udp}
	}
	delete(stream, "finalmask")
	delete(stream, "kcpSettings")
	stream["network"] = "tcp"
	return transport, nativeMask, nil
}
