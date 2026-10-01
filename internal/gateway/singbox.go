package gateway

import "fmt"

// singBoxGatewayInbound returns the transparent-proxy listener used by
// Gateway Mode when sing-box is the active core. Leaving network unset makes
// sing-box listen for both TCP and UDP, matching the firewall TPROXY rules.
func singBoxGatewayInbound() map[string]any {
	return map[string]any{
		"type":        "tproxy",
		"tag":         inboundTag,
		"listen":      "0.0.0.0",
		"listen_port": inboundPort,
	}
}

func singBoxPort(value any) (int, bool) {
	switch port := value.(type) {
	case int:
		return port, true
	case int32:
		return int(port), true
	case int64:
		return int(port), true
	case float64:
		converted := int(port)
		if port != float64(converted) {
			return 0, false
		}
		return converted, true
	default:
		return 0, false
	}
}

func isSingBoxGatewayInbound(item any) bool {
	inbound, ok := item.(map[string]any)
	if !ok || itemTag(item) != inboundTag {
		return false
	}
	inboundType, _ := inbound["type"].(string)
	listen, _ := inbound["listen"].(string)
	port, portOK := singBoxPort(inbound["listen_port"])
	return inboundType == "tproxy" && listen == "0.0.0.0" && portOK && port == inboundPort
}

func hasSingBoxGatewayInbound(cfg map[string]any) bool {
	inbounds, ok := cfg["inbounds"].([]any)
	if !ok {
		return false
	}
	for _, item := range inbounds {
		if isSingBoxGatewayInbound(item) {
			return true
		}
	}
	return false
}

func validateSingBoxGatewayConflicts(cfg map[string]any) error {
	inbounds, err := arrayField(cfg, "inbounds")
	if err != nil {
		return err
	}
	for _, item := range inbounds {
		if isSingBoxGatewayInbound(item) {
			continue
		}
		if itemTag(item) == inboundTag {
			return fmt.Errorf("sing-box inbound tag %q is already used by a non-Gateway inbound", inboundTag)
		}

		inbound, ok := item.(map[string]any)
		if !ok {
			continue
		}
		port, ok := singBoxPort(inbound["listen_port"])
		if !ok || port != inboundPort {
			continue
		}
		tag := itemTag(item)
		if tag == "" {
			tag = "<untagged>"
		}
		return fmt.Errorf("sing-box inbound %q already uses Gateway port %d", tag, inboundPort)
	}
	return nil
}

func removeSingBoxGatewayConfig(cfg map[string]any) (bool, error) {
	inbounds, err := arrayField(cfg, "inbounds")
	if err != nil {
		return false, err
	}
	filtered := make([]any, 0, len(inbounds))
	removed := false
	for _, item := range inbounds {
		if isSingBoxGatewayInbound(item) {
			removed = true
			continue
		}
		filtered = append(filtered, item)
	}
	if removed {
		cfg["inbounds"] = filtered
	}
	return removed, nil
}

func applySingBoxGatewayConfig(cfg map[string]any) error {
	if err := validateSingBoxGatewayConflicts(cfg); err != nil {
		return err
	}
	return replaceTaggedItem(cfg, "inbounds", inboundTag, singBoxGatewayInbound())
}
