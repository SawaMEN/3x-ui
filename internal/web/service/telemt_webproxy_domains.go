package service

import (
	"errors"
	"fmt"
	"sort"
)

// WEB domains share one HTTPS certificate and frontend. A second domain is an
// alias of the same Telemt endpoint, not a second process or TCP listener.
func normalizeTelemtWebAliases(primary string, domains []string) ([]string, error) {
	if len(domains) > 7 {
		return nil, errors.New("WEB Proxy supports up to 7 additional domains")
	}
	seen := map[string]bool{}
	if primary != "" {
		seen[normalizeTelemtWebDomain(primary)] = true
	}
	result := make([]string, 0, len(domains))
	for _, raw := range domains {
		domain := normalizeTelemtWebDomain(raw)
		if !telemtWebDomainPattern.MatchString(domain) || len(domain) > 253 {
			return nil, fmt.Errorf("invalid additional WEB domain %q", raw)
		}
		if seen[domain] {
			return nil, fmt.Errorf("duplicate WEB domain %q", domain)
		}
		seen[domain] = true
		result = append(result, domain)
	}
	sort.Strings(result)
	return result, nil
}

func telemtWebHosts(state TelemtWebProxyState) []string {
	hosts := make([]string, 0, len(state.Aliases)+1)
	if state.Domain != "" {
		hosts = append(hosts, state.Domain)
	}
	return append(hosts, state.Aliases...)
}

func telemtWebCertificateCoversHosts(certFile, keyFile string, hosts []string) bool {
	if len(hosts) == 0 {
		return false
	}
	for _, host := range hosts {
		if !telemtWebCertificatePairReady(certFile, keyFile, host) {
			return false
		}
	}
	return true
}
