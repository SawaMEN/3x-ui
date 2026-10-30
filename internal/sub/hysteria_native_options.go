package sub

import (
	"strconv"
	"strings"
)

// Native obfs is distinct from Xray's finalmask. Both emitters must read the
// native field or a configured Salamander/Gecko server gets unusable clients.
func hysteriaNativeURIOptions(stream map[string]any, version int) map[string]string {
	options := map[string]string{}
	hy, _ := stream["hysteriaSettings"].(map[string]any)
	if version == 1 {
		for key, aliases := range map[string][]string{"up_mbps": {"up_mbps", "upMbps", "up"}, "down_mbps": {"down_mbps", "downMbps", "down"}} {
			for _, alias := range aliases {
				if n, ok := hy[alias].(float64); ok && n > 0 {
					options[key] = strconv.FormatFloat(n, 'f', -1, 64)
					break
				}
			}
		}
		if password, ok := hy["obfs"].(string); ok && password != "" {
			options["obfs"] = password
		}
		return options
	}
	obfs, _ := hy["obfs"].(map[string]any)
	kind, _ := obfs["type"].(string)
	kind = strings.ToLower(kind)
	password, _ := obfs["password"].(string)
	if (kind != "salamander" && kind != "gecko") || password == "" {
		return options
	}
	options["obfs"], options["obfs-password"] = kind, password
	if kind == "gecko" {
		for key, aliases := range map[string][]string{"minPacketSize": {"min_packet_size", "minPacketSize"}, "maxPacketSize": {"max_packet_size", "maxPacketSize"}} {
			for _, alias := range aliases {
				if n, ok := obfs[alias].(float64); ok && n >= geckoMinPacketSize && n <= geckoMaxPacketSize {
					options[key] = strconv.Itoa(int(n))
					break
				}
			}
		}
	}
	return options
}
