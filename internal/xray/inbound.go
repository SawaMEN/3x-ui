package xray

import (
	"encoding/json"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/util/json_util"
)

// InboundConfig is the legacy Xray wire representation of the panel inbound.
type InboundConfig coremodel.InboundConfig

// InboundFromPanel adapts a panel inbound to the Xray wire representation.
func InboundFromPanel(inbound *coremodel.InboundConfig) *InboundConfig {
	if inbound == nil {
		return nil
	}
	wire := InboundConfig(*inbound)
	return &wire
}

// MarshalJSON strips panel-only VLESS fields from the wire config sent to
// xray-core. VLESS encryption stores the client-side "encryption" value next to
// the server-side "decryption" value so subscriptions can be generated from the
// same inbound row, but xray-core's inbound only consumes decryption.
//
// xray-core also treats an explicitly present empty fallbacks array as
// configured. With VLESS encryption enabled (decryption != "none"), that makes
// Build reject the inbound even though there are no actual fallbacks. Omit only
// the empty array in that mode; non-empty fallbacks are intentionally preserved
// so the core reports the invalid combination instead of silently discarding an
// operator configuration.
func (c InboundConfig) MarshalJSON() ([]byte, error) {
	type inboundConfigAlias InboundConfig
	wire := inboundConfigAlias(c)
	if c.Protocol == "vless" {
		wire.Settings = sanitizeVlessInboundSettings(c.Settings)
	}
	return json.Marshal(wire)
}

func sanitizeVlessInboundSettings(settings json_util.RawMessage) json_util.RawMessage {
	if len(settings) == 0 {
		return settings
	}

	var parsed map[string]any
	if err := json.Unmarshal(settings, &parsed); err != nil {
		return settings
	}

	changed := false
	if _, ok := parsed["encryption"]; ok {
		delete(parsed, "encryption")
		changed = true
	}

	decryption, _ := parsed["decryption"].(string)
	if decryption != "" && decryption != "none" {
		if fallbacks, ok := parsed["fallbacks"].([]any); ok && len(fallbacks) == 0 {
			delete(parsed, "fallbacks")
			changed = true
		}
	}

	if !changed {
		return settings
	}
	out, err := json.Marshal(parsed)
	if err != nil {
		return settings
	}
	return json_util.RawMessage(out)
}

// Equals compares the panel fields without changing Xray wire serialization.
func (c *InboundConfig) Equals(other *InboundConfig) bool {
	return (*coremodel.InboundConfig)(c).Equals((*coremodel.InboundConfig)(other))
}
