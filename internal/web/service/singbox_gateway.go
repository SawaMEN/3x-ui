package service

import (
	"encoding/json"
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Ordinary template inbounds must not revive panel-managed disabled clients.
// The dedicated Gateway listener is the exception: it lives in the template,
// not in the inbound database, and must reach the actual generated config.
func mergeSingBoxGatewayInbound(cfg *singbox.Config, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var inbounds []map[string]any
	if err := json.Unmarshal(raw, &inbounds); err != nil {
		return fmt.Errorf("parse native Gateway inbounds: %w", err)
	}
	var candidate map[string]any
	for _, inbound := range inbounds {
		if inbound["tag"] != "in-tproxy" {
			continue
		}
		if candidate != nil {
			return fmt.Errorf("duplicate native Gateway inbound")
		}
		if inbound["type"] != "tproxy" || inbound["listen"] != "0.0.0.0" || inbound["listen_port"] != float64(52345) {
			return fmt.Errorf("native Gateway tag is occupied by an incompatible inbound")
		}
		if network, present := inbound["network"]; present && network != "" {
			return fmt.Errorf("Gateway requires both TCP and UDP")
		}
		candidate = inbound
	}
	if candidate == nil {
		return nil
	}
	for _, inbound := range cfg.Inbounds {
		if inbound["tag"] == "in-tproxy" || inbound["listen_port"] == 52345 || inbound["listen_port"] == float64(52345) {
			return fmt.Errorf("generated inbound conflicts with Gateway tag or port")
		}
	}
	cfg.Inbounds = append(cfg.Inbounds, candidate)
	return nil
}
