package hiddify

import (
	"fmt"
	"strings"
)

func takeInboundTCPFinalMask(stream map[string]any, protocol, label string) (map[string]any, error) {
	value := stream["finalmask"]
	if value == nil {
		return nil, nil
	}
	mask, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: finalmask must be an object", label)
	}
	value = mask["tcp"]
	if value == nil {
		return nil, nil
	}
	masks, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: finalmask.tcp must be an array", label)
	}
	if len(masks) == 0 {
		return nil, nil
	}
	network := strings.ToLower(strings.TrimSpace(stringValue(stream["network"])))
	switch protocol {
	case "vless", "vmess", "trojan", "shadowsocks", "http", "socks", "mixed", "anytls", "shadowtls", "snell", "naive":
	default:
		return nil, fmt.Errorf("%s: TCP FinalMask requires a supported TCP listener", label)
	}
	switch network {
	case "", "tcp", "raw", "ws", "httpupgrade", "http", "grpc", "xhttp":
	default:
		return nil, fmt.Errorf("%s: TCP FinalMask cannot wrap %q transport", label, network)
	}
	if strings.EqualFold(stringValue(stream["security"]), "reality") {
		return nil, fmt.Errorf("%s: TCP FinalMask is incompatible with REALITY; use TLS or remove TCP masks", label)
	}
	for i, value := range masks {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: finalmask.tcp[%d] must be an object", label, i)
		}
		switch item["type"] {
		case "fragment", "sudoku", "header-custom", "xmc":
		default:
			return nil, fmt.Errorf("%s: unsupported finalmask.tcp[%d] type %q", label, i, item["type"])
		}
	}
	delete(mask, "tcp")
	return map[string]any{"tcp": masks}, nil
}

func stringValue(v any) string { s, _ := v.(string); return s }
