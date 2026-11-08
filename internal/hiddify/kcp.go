package hiddify

import (
	"encoding/json"
	"fmt"
	"strings"
)

func translateKCP(stream map[string]any) (map[string]any, error) {
	transport := map[string]any{"type": "kcp"}
	options := object(stream, "kcpSettings")
	fields := map[string]string{"mtu": "mtu", "tti": "tti", "uplinkCapacity": "uplink_capacity", "downlinkCapacity": "downlink_capacity", "cwndMultiplier": "cwnd_multiplier", "maxSendingWindow": "max_sending_window"}
	for key, value := range options {
		if native, ok := fields[key]; ok {
			data, _ := json.Marshal(value)
			var n uint32
			if json.Unmarshal(data, &n) != nil {
				return nil, fmt.Errorf("invalid mKCP %s", key)
			}
			transport[native] = n
			continue
		}
		switch key {
		case "seed":
			if _, ok := value.(string); !ok {
				return nil, fmt.Errorf("mKCP seed must be a string")
			}
			transport["seed"] = value
		case "header":
			header, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("mKCP header must be an object")
			}
			for field, v := range header {
				if field != "type" && meaningful(v) {
					return nil, fmt.Errorf("unsupported mKCP header.%s", field)
				}
			}
			kind, ok := header["type"].(string)
			if !ok && header["type"] != nil {
				return nil, fmt.Errorf("mKCP header.type must be a string")
			}
			transport["header"] = strings.ToLower(strings.TrimSpace(kind))
		default:
			if meaningful(value) {
				return nil, fmt.Errorf("mKCP setting %q has no equivalent in the current transport", key)
			}
		}
	}
	mask := object(stream, "finalmask")
	if meaningful(mask["tcp"]) {
		return nil, fmt.Errorf("mKCP supports UDP finalmask only")
	}
	if meaningful(mask["udp"]) {
		if meaningful(transport["seed"]) || meaningful(transport["header"]) && transport["header"] != "none" {
			return nil, fmt.Errorf("mKCP seed/header cannot be combined with explicit UDP masks")
		}
		transport["final_mask"] = mask
	} else if _, seed := options["seed"]; seed || options["header"] != nil {
		transport["legacy"] = true
	}
	delete(stream, "finalmask")
	delete(stream, "kcpSettings")
	stream["network"] = "tcp"
	return transport, nil
}
