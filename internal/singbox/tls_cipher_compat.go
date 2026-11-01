package singbox

import (
	"crypto/tls"
	"fmt"
	"strings"
)

// The patched panel core accepts modern and explicitly configured legacy
// suites. Insecure suites are never enabled by default; this capability
// requires the explicit-cipher-suites patch in the pinned core build.
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
	for _, list := range [][]*tls.CipherSuite{tls.CipherSuites(), tls.InsecureCipherSuites()} {
		for _, suite := range list {
			known[suite.Name] = true
		}
	}

	values := make([]string, 0, len(names))
	seen := make(map[string]bool)
	for _, part := range names {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if !known[name] {
			return nil, fmt.Errorf("%s: unknown TLS cipher suite %q; select a cipher supported by the patched panel core", label, name)
		}
		if !seen[name] {
			values = append(values, name)
			seen[name] = true
		}
	}
	return values, nil
}
