package service

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"strings"
)

// normalizeShareOutboundECH converts public ECHConfigList material carried in
// connection URIs to the PEM format required by sing-box. Hysteria2 links
// carry ECH as base64, while the native Hiddify/sing-box client requires
// -----BEGIN ECH CONFIGS-----. Never drop ECH on an invalid value: doing so
// would silently expose the inner SNI on connections that requested ECH.
func normalizeShareOutboundECH(out map[string]any) error {
	tls, ok := out["tls"].(map[string]any)
	if !ok {
		return nil
	}
	ech, ok := tls["ech"].(map[string]any)
	if !ok || ech["enabled"] == false {
		return nil
	}
	value, exists := ech["config"]
	if !exists || value == nil {
		return nil // DNS-discovered ECH: no static material to normalize.
	}
	var lines []string
	switch config := value.(type) {
	case string:
		lines = []string{config}
	case []string:
		lines = config
	case []any:
		lines = make([]string, 0, len(config))
		for _, item := range config {
			line, ok := item.(string)
			if !ok {
				return fmt.Errorf("outbound ECH config requires PEM text lines")
			}
			lines = append(lines, line)
		}
	default:
		return fmt.Errorf("outbound ECH config must contain PEM text lines")
	}
	raw := strings.TrimSpace(strings.Join(lines, "\n"))
	if raw == "" {
		return nil // Explicit dynamic ECH, queried from DNS.
	}

	var configList []byte
	if strings.HasPrefix(raw, "-----BEGIN ") {
		block, rest := pem.Decode([]byte(raw))
		if block == nil || (block.Type != "ECH CONFIGS" && block.Type != "ECHCONFIG") || strings.TrimSpace(string(rest)) != "" {
			return fmt.Errorf("outbound ECH config must contain exactly one ECH CONFIGS public PEM block")
		}
		configList = block.Bytes
	} else {
		if strings.Contains(raw, "://") {
			return fmt.Errorf("outbound ECH domain+DoH settings cannot be used as an ECH config; configure native ECH DNS discovery instead")
		}
		// Only whitespace between base64 lines is ignorable; never remove
		// punctuation or change the supplied ECHConfigList bytes.
		encoded := strings.Join(strings.Fields(raw), "")
		var err error
		for _, encoding := range []*base64.Encoding{
			base64.StdEncoding.Strict(), base64.RawStdEncoding.Strict(),
			base64.URLEncoding.Strict(), base64.RawURLEncoding.Strict(),
		} {
			if configList, err = encoding.DecodeString(encoded); err == nil {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("outbound ECH config is not valid base64 or ECH CONFIGS PEM")
		}
	}
	// ECHConfigList starts with a uint16 length covering all following
	// ECHConfig entries. Reject truncated or mismatched data before handing
	// it to the native core; the core validates individual ECHConfig entries.
	if len(configList) < 4 || len(configList) > 65537 ||
		int(binary.BigEndian.Uint16(configList[:2])) != len(configList)-2 {
		return fmt.Errorf("outbound ECH config is not a valid ECHConfigList")
	}
	ech["config"] = []string{strings.TrimSuffix(string(pem.EncodeToMemory(&pem.Block{
		Type:  "ECH CONFIGS",
		Bytes: configList,
	})), "\n")}
	return nil
}
