package singbox

import (
	"crypto/tls"
	"fmt"
	"strings"
)

// Translate the operator's explicit cipher suites without silently changing
// the TLS policy. Go's tls.CipherSuites excludes legacy RSA key-exchange,
// CBC-SHA256, RC4 and 3DES suites from its safe defaults, but the modified
// sing-box core accepts those entries when they are explicitly configured.
// An unset list continues to use secure defaults.
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
	for _, suite := range tls.InsecureCipherSuites() {
		known[suite.Name] = true
	}

	values := make([]string, 0, len(names))
	seen := make(map[string]bool)
	for _, part := range names {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if !known[name] {
			return nil, fmt.Errorf("%s: unknown TLS cipher suite %q; select a cipher supported by the Go TLS runtime", label, name)
		}
		if !seen[name] {
			values = append(values, name)
			seen[name] = true
		}
	}
	return values, nil
}
