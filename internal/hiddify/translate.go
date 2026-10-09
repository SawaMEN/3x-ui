package hiddify

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

const TargetVersion = "v5.0.0"

// TranslateInbound preserves Hiddify extensions without changing the sing-box adapter.
func TranslateInbound(raw map[string]any) (map[string]any, error) {
	return translate(raw, true)
}

func TranslateOutbound(raw map[string]any) (map[string]any, error) {
	return translate(raw, false)
}

func translate(raw map[string]any, inbound bool) (map[string]any, error) {
	copy, err := clone(raw)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"settings", "streamSettings"} {
		if value := copy[key]; value != nil {
			if _, ok := value.(map[string]any); !ok {
				return nil, fmt.Errorf("hiddify %q: %s must be an object", copy["tag"], key)
			}
		}
	}
	stream := object(copy, "streamSettings")
	settings := object(copy, "settings")
	protocol, _ := copy["protocol"].(string)
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	label := fmt.Sprintf("hiddify %s %q", direction(inbound), copy["tag"])
	if err := rejectProtectionLoss(settings, stream, label); err != nil {
		return nil, err
	}
	var transport map[string]any
	if stream["network"] == "xhttp" {
		if value := stream["xhttpSettings"]; value != nil {
			if _, ok := value.(map[string]any); !ok {
				return nil, fmt.Errorf("%s: xhttpSettings must be an object", label)
			}
		}
		if protocol != "vless" && protocol != "vmess" && protocol != "trojan" {
			return nil, fmt.Errorf("%s: XHTTP is unsupported for %s", label, protocol)
		}
		transport, err = translateXHTTP(object(stream, "xhttpSettings"), inbound, label)
		if err != nil {
			return nil, err
		}
		stream["network"] = "tcp"
		delete(stream, "xhttpSettings")
	}
	var encryption any
	if protocol == "vless" {
		if inbound {
			encryption = settings["decryption"]
			delete(settings, "decryption")
		} else {
			user := vlessUser(settings)
			encryption = user["encryption"]
			delete(user, "encryption")
		}
		if encryption != nil {
			if _, ok := encryption.(string); !ok {
				return nil, fmt.Errorf("%s: VLESS encryption/decryption must be a string", label)
			}
		}
	}
	var mask map[string]any
	if !inbound && stream["finalmask"] != nil {
		var ok bool
		mask, ok = stream["finalmask"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: finalmask must be an object", label)
		}
		for key, value := range mask {
			if key != "tcp" && key != "udp" && meaningful(value) {
				return nil, fmt.Errorf("%s: finalmask.%s has no native mapping", label, key)
			}
		}
		delete(mask, "quicParams")
		delete(stream, "finalmask")
	}
	var out map[string]any
	if inbound {
		out, err = singbox.TranslateXrayInbound(copy)
	} else {
		out, err = singbox.TranslateXrayOutbound(copy)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	if encryption != nil && encryption != "" && encryption != "none" {
		key := "encryption"
		if inbound {
			key = "decryption"
		}
		out[key] = encryption
	}
	if transport != nil {
		out["transport"] = transport
	}
	if mask != nil {
		out["final_mask"] = mask
	}
	return out, nil
}

func TranslateMASQUEEndpoint(raw map[string]any) (map[string]any, error) {
	return singbox.TranslateMASQUEEndpoint(raw)
}

func rejectProtectionLoss(settings, stream map[string]any, label string) error {
	options := []map[string]any{settings, object(stream, "realitySettings"), vlessUser(settings)}
	clients, _ := settings["clients"].([]any)
	for _, client := range clients {
		if object, ok := client.(map[string]any); ok {
			options = append(options, object)
		}
	}
	for _, options := range options {
		for _, key := range []string{"mldsa65Seed", "mldsa65Verify", "visionSeed", "testseed", "seed", "reverse", "testpre"} {
			if meaningful(options[key]) {
				return fmt.Errorf("%s: %s cannot be preserved by %s", label, key, TargetVersion)
			}
		}
	}
	return nil
}

func vlessUser(settings map[string]any) map[string]any {
	vnext, _ := settings["vnext"].([]any)
	if len(vnext) > 0 {
		if server, ok := vnext[0].(map[string]any); ok {
			users, _ := server["users"].([]any)
			if len(users) > 0 {
				user, _ := users[0].(map[string]any)
				return user
			}
		}
	}
	return settings
}

func clone(raw map[string]any) (map[string]any, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(data, &out)
	if err == nil && out == nil {
		err = fmt.Errorf("hiddify configuration must be an object")
	}
	return out, err
}

func object(raw map[string]any, key string) map[string]any {
	value, _ := raw[key].(map[string]any)
	if value == nil {
		value = map[string]any{}
	}
	return value
}

func meaningful(value any) bool {
	if value == nil || value == "" || value == false {
		return false
	}
	switch value := value.(type) {
	case float64:
		return value != 0
	case map[string]any:
		return len(value) > 0
	case []any:
		return len(value) > 0
	default:
		return true
	}
}

func direction(inbound bool) string {
	if inbound {
		return "inbound"
	}
	return "outbound"
}
