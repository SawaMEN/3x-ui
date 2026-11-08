package domainfront

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

const MaxRootRedirectURLLength = 2048

func ValidateRootRedirectURL(raw string) (*url.URL, error) {
	if strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return nil, fmt.Errorf("redirect URL must not contain control characters")
	}
	raw = strings.TrimSpace(raw)
	if len(raw) > MaxRootRedirectURLLength {
		return nil, fmt.Errorf("redirect URL must not exceed %d bytes", MaxRootRedirectURLLength)
	}
	if strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return nil, fmt.Errorf("redirect URL must not contain control characters")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	if u.Hostname() == "" || u.Opaque != "" {
		return nil, fmt.Errorf("URL host is required")
	}
	if u.User != nil {
		return nil, fmt.Errorf("redirect URL must not contain a username or password")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("redirect URL port must be between 1 and 65535")
		}
	}
	if strings.HasSuffix(u.Host, ":") {
		return nil, fmt.Errorf("redirect URL port is empty")
	}
	return u, nil
}

func ValidateRootRedirectDomains(raw string, domains []string) error {
	u, err := ValidateRootRedirectURL(raw)
	if err != nil {
		return err
	}
	if u.Path != "" && u.Path != "/" || u.Query().Get("panel") == "1" {
		return nil
	}
	for _, name := range domains {
		if strings.EqualFold(strings.TrimSuffix(name, "."), strings.TrimSuffix(u.Hostname(), ".")) {
			return fmt.Errorf("redirect target is the root of a managed domain: choose another website, a page path, or ?panel=1 to open the panel")
		}
	}
	return nil
}
