package singbox

import (
	"crypto/tls"
	"fmt"
	"strings"
)

// Translate an explicit TLS policy into suites accepted by the pinned core.
// crypto/tls.InsecureCipherSuites includes legacy RSA key-exchange suites that
// the pinned sing-box constructor does not recognize. Reject those up front
// instead of emitting a config that passes Go tests but cannot start.
// An unset list continues to use the core's secure defaults.
func translateTLSCipherSuites(out, in map[string]any, label string) error {
	raw := strings.TrimSpace(rawString(in, "cipherSuites"))
	if raw == "" {
		return nil
	}
	names := strings.FieldsFunc(raw, func(r rune) bool { return r == ':' || r == ',' })
	values, err := compatibleTLSCipherSuites(names, label)
	if err != nil {
		return err
	}
	out["cipher_suites"] = values
	return nil
}

func compatibleTLSCipherSuites(names []string, label string) ([]string, error) {
	known := make(map[string]bool)
	for _, suite := range tls.CipherSuites() {
		known[suite.Name] = true
	}
	// The installed sing-box core rejects legacy RSA key-exchange suites even
	// though Go still exposes them as insecure names for compatibility.
	legacy := make(map[string]bool)
	for _, suite := range tls.InsecureCipherSuites() {
		legacy[suite.Name] = true
	}

	values := make([]string, 0, len(names))
	seen := make(map[string]bool)
	for _, part := range names {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if legacy[name] {
			return nil, fmt.Errorf("%s: TLS cipher suite %q is unsupported by the pinned sing-box core; select an ECDHE or TLS 1.3 suite", label, name)
		}
		if !known[name] {
			return nil, fmt.Errorf("%s: unknown TLS cipher suite %q; select a cipher supported by the pinned sing-box core", label, name)
		}
		if !seen[name] {
			values = append(values, name)
			seen[name] = true
		}
	}
	return values, nil
}
