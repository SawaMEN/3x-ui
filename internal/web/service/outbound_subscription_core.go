package service

import (
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// filterSubscriptionOutbounds keeps only subscription members that the selected
// core can actually consume. The full, unfiltered subscription is persisted by
// fetchAndStore so switching cores can restore profiles that are incompatible
// with the currently selected core.
func filterSubscriptionOutbounds(label string, members []any) []any {
	kept, _, _ := filterSubscriptionOutboundsDetailed(label, members)
	return kept
}

// filterSubscriptionOutboundsDetailed is the diagnostic form of
// filterSubscriptionOutbounds. It returns human-readable rejection reasons so
// the UI can say that a profile is unsupported by the selected core instead of
// silently deleting it from the stored subscription.
func filterSubscriptionOutboundsDetailed(label string, members []any) (kept []any, rejected []string, coreName string) {
	core, _ := (&SettingService{}).GetCoreType()
	if core != CoreTypeSingBox {
		kept, rejected = filterOutboundsRejectedByCore(label, members)
		return kept, rejected, "xray"
	}

	coreName = "sing-box"
	kept = make([]any, 0, len(members))
	for _, raw := range members {
		ob, ok := raw.(map[string]any)
		if !ok {
			reason := "invalid outbound JSON: expected an object"
			logger.Warningf("%s: unsupported sing-box subscription member: %s", label, reason)
			rejected = append(rejected, reason)
			continue
		}

		protocol, _ := ob["protocol"].(string)
		protocol = strings.ToLower(strings.TrimSpace(protocol))
		tag, _ := ob["tag"].(string)
		if protocol == "" {
			reason := fmt.Sprintf("%s: outbound protocol is empty", tag)
			logger.Warningf("%s: unsupported sing-box outbound %q: %s", label, tag, reason)
			rejected = append(rejected, reason)
			continue
		}

		var err error
		switch protocol {
		case "loopback":
			err = fmt.Errorf("Xray loopback cannot be translated to sing-box")
		case "amneziawg": // The runtime and probe builders supply a SOCKS bridge.
		case "wireguard":
			_, err = singbox.TranslateXrayWireGuardEndpoint(ob)
		default:
			// TranslateXrayOutbound deliberately passes unknown native sing-box
			// protocol names through. This keeps JSON import forward-compatible;
			// the installed sing-box remains the final authority for new types.
			_, err = singbox.TranslateXrayOutbound(ob)
		}
		if err != nil {
			reason := fmt.Sprintf("%s [%s]: %v", tag, protocol, err)
			logger.Warningf("%s: unsupported sing-box outbound %q: %v", label, tag, err)
			rejected = append(rejected, reason)
			continue
		}
		kept = append(kept, raw)
	}
	return kept, rejected, coreName
}
