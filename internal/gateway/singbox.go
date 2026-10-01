package gateway

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

func applySingBoxGatewayConfig(cfg map[string]any) error {
	return replaceTaggedItem(cfg, "inbounds", inboundTag, singBoxGatewayInbound())
}
