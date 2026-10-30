package singbox

import (
	"crypto/tls"
	"fmt"
	"strings"
)

// Hiddify resolves cipher names through tls.CipherSuites, excluding the
// legacy RSA/RC4/CBC-SHA256 suites in tls.InsecureCipherSuites. Old panel
// presets mixed both lists. Keep their supported subset without changing the
// saved Xray configuration, but never widen an explicit list to defaults.
func translateTLSCipherSuites(out, in map[string]any, label string) error {
	raw := strings.TrimSpace(rawString(in, "cipherSuites"))
	if raw == "" {
		return nil
	}
	supported, insecure := map[string]bool{}, map[string]bool{}
	for _, suite := range tls.CipherSuites() {
		supported[suite.Name] = true
	}
	for _, suite := range tls.InsecureCipherSuites() {
		insecure[suite.Name] = true
	}
	values := []string{}
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ':' || r == ',' }) {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if !supported[name] {
			if insecure[name] {
				continue
			}
			return fmt.Errorf("%s: unknown TLS cipher suite %q; select a supported cipher or clear cipherSuites to use core defaults", label, name)
		}
		if !seen[name] {
			values = append(values, name)
			seen[name] = true
		}
	}
	if len(values) == 0 {
		return fmt.Errorf("%s: cipherSuites contains no supported TLS ciphers; replace legacy ciphers or clear cipherSuites to use core defaults", label)
	}
	out["cipher_suites"] = values
	return nil
}
