// Package routingcatalog normalizes portable domain and IP routing lists.
package routingcatalog

import (
	"bufio"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"sort"
	"strings"
)

const MaxEntries = 50000

type Entries struct {
	Domains []string `json:"domains"`
	IPs     []string `json:"ips"`
}

var domainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$`)

func normalize(value string) (string, bool, error) {
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return prefix.Masked().String(), true, nil
	}
	if ip, err := netip.ParseAddr(value); err == nil {
		if ip.Zone() != "" {
			return "", false, fmt.Errorf("scoped IP addresses are unsupported")
		}
		return ip.Unmap().String(), true, nil
	}
	mode, domain := "domain:", value
	for _, prefix := range []string{"domain:", "full:", "keyword:", "regexp:"} {
		if strings.HasPrefix(value, prefix) {
			mode, domain = prefix, strings.TrimPrefix(value, prefix)
			break
		}
	}
	if mode == "regexp:" {
		if domain == "" {
			return "", false, fmt.Errorf("empty regular expression")
		}
		if _, err := regexp.Compile(domain); err != nil {
			return "", false, err
		}
		return mode + domain, false, nil
	}
	if mode == "keyword:" {
		if domain == "" || strings.ContainsAny(domain, " \t/:<>") {
			return "", false, fmt.Errorf("invalid domain keyword")
		}
		return mode + strings.ToLower(domain), false, nil
	}
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	if !domainPattern.MatchString(domain) || len(domain) > 253 {
		return "", false, fmt.Errorf("invalid domain or IP %q", value)
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) > 63 {
			return "", false, fmt.Errorf("domain label is too long")
		}
	}
	return mode + domain, false, nil
}

// Parse accepts TXT domains, domain:/full:/keyword:/regexp: matchers, IP/CIDR
// and hosts rows. Unknown formats fail atomically instead of silently producing
// a partial routing policy (e.g. HTML error pages or unresolved include: lines).
func Parse(r io.Reader) (Entries, error) {
	out := Entries{Domains: []string{}, IPs: []string{}}
	domains, ips := map[string]bool{}, map[string]bool{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 16<<10)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if text == "" || strings.HasPrefix(text, "#") || strings.HasPrefix(text, "!") {
			continue
		}
		if !strings.HasPrefix(text, "regexp:") {
			text = strings.TrimSpace(strings.SplitN(text, "#", 2)[0])
		}
		values := []string{text}
		fields := strings.Fields(text)
		if len(fields) > 1 && !strings.HasPrefix(text, "regexp:") {
			if _, err := netip.ParseAddr(fields[0]); err != nil {
				return out, fmt.Errorf("line %d: expected domain/IP or hosts row", line)
			}
			values = fields[1:]
		}
		for _, value := range values {
			if value == "localhost" || value == "localhost.localdomain" || strings.HasPrefix(value, "ip6-") {
				continue
			}
			normalized, isIP, err := normalize(value)
			if err != nil {
				return out, fmt.Errorf("line %d: %w", line, err)
			}
			if isIP {
				ips[normalized] = true
			} else {
				domains[normalized] = true
			}
			if len(domains)+len(ips) > MaxEntries {
				return out, fmt.Errorf("list exceeds %d entries", MaxEntries)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}
	for value := range domains {
		out.Domains = append(out.Domains, value)
	}
	for value := range ips {
		out.IPs = append(out.IPs, value)
	}
	sort.Strings(out.Domains)
	sort.Strings(out.IPs)
	if len(out.Domains)+len(out.IPs) == 0 {
		return out, fmt.Errorf("list contains no routing entries")
	}
	return out, nil
}
