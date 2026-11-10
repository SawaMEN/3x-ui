package singbox

import (
	"maps"
	"strings"
)

// TranslatePanelEndpoint stores tunnel clients in endpoints while retaining
// their tags in the shared panel connection list for routing and detours.
func TranslatePanelEndpoint(raw map[string]any) (map[string]any, bool, error) {
	protocol := strings.ToLower(strings.TrimSpace(rawString(raw, "protocol")))
	if protocol == "wireguard" {
		endpoint, err := TranslateXrayWireGuardEndpoint(raw)
		return endpoint, true, err
	}
	kind := strings.TrimPrefix(protocol, nativeSingBoxProtocolPrefix)
	if !strings.HasPrefix(protocol, nativeSingBoxProtocolPrefix) || !isKnownSingBoxEndpoint(kind) {
		return nil, false, nil
	}
	normalized, err := NormalizeOutboundInput(raw)
	if err != nil {
		return nil, true, err
	}
	endpoint := maps.Clone(rawObject(normalized, "settings"))
	if endpoint == nil {
		endpoint = map[string]any{}
	}
	endpoint["type"], endpoint["tag"] = kind, normalized["tag"]
	if err := applyXrayOutboundCompatibility(endpoint, normalized, rawObject(normalized, "streamSettings")); err != nil {
		return nil, true, err
	}
	return endpoint, true, validateSingBoxEndpoint(endpoint)
}
