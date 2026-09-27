package sub

import (
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// nativeMieruOutbounds builds Hiddify's native sing-box Mieru outbound
// shape. Stock sing-box subscriptions still reject Mieru; this is only
// consumed by the Hiddify+ShadowTLS structured subscription path.
func nativeMieruOutbounds(subReq *SubService, inbound *model.Inbound, client model.Client) []map[string]any {
	if subReq == nil || inbound == nil || inbound.Protocol != model.Mieru || client.Password == "" {
		return nil
	}
	settings := subReq.linkSettings(inbound)
	entries := mieruShareEntries(settings, inbound.Port)
	if len(entries) == 0 {
		return nil
	}
	multiplexing, _ := settings["multiplexing"].(string)
	if strings.TrimSpace(multiplexing) == "" {
		multiplexing = "MULTIPLEXING_LOW"
	}
	handshakeMode, _ := settings["handshakeMode"].(string)
	if strings.TrimSpace(handshakeMode) == "" {
		handshakeMode = "HANDSHAKE_STANDARD"
	}
	endpoints := subReq.shareEndpointsForInbound(inbound)
	out := make([]map[string]any, 0, len(endpoints))
	for _, endpoint := range endpoints {
		publicEntries := entries
		if endpoint.ep != nil {
			publicEntries = make([]struct {
				port     string
				protocol string
			}, 0, len(entries))
			seenProtocols := make(map[string]struct{})
			for _, entry := range entries {
				if _, exists := seenProtocols[entry.protocol]; exists {
					continue
				}
				seenProtocols[entry.protocol] = struct{}{}
				publicEntries = append(publicEntries, struct {
					port     string
					protocol string
				}{port: strconv.Itoa(endpoint.Port), protocol: entry.protocol})
			}
		}
		bindings := make([]any, 0, len(publicEntries))
		for _, entry := range publicEntries {
			binding := map[string]any{"protocol": entry.protocol}
			if strings.Contains(entry.port, "-") {
				binding["port_range"] = entry.port
			} else {
				port, err := strconv.Atoi(entry.port)
				if err != nil || port < 1025 || port > 65535 {
					continue
				}
				binding["port"] = port
			}
			bindings = append(bindings, binding)
		}
		if len(bindings) == 0 || strings.TrimSpace(endpoint.Address) == "" {
			continue
		}
		out = append(out, map[string]any{
			"type":           "mieru",
			"server":         endpoint.Address,
			"port_bindings":  bindings,
			"username":       client.Email,
			"password":       client.Password,
			"multiplexing":   multiplexing,
			"handshake_mode": handshakeMode,
		})
	}
	return out
}
