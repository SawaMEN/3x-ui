package sub

import (
	"net/url"
	"strings"
)

// These share links require clients outside the stock Xray and sing-box
// protocols. This is a UI/URI classification; it does not imply a common
// client app can import every scheme.
var standaloneSubscriptionSchemes = map[string]struct{}{
	"pingtunnel": {}, "fptn": {}, "openflux": {}, "vetroff": {},
	"tt": {}, "sudoku": {},
	"naive": {}, "naive+https": {}, "naive+quic": {},
}

func isStandaloneSubscriptionLink(link string) bool {
	link = strings.TrimSpace(link)
	colon := strings.IndexByte(link, ':')
	if colon <= 0 {
		return false
	}
	_, ok := standaloneSubscriptionSchemes[strings.ToLower(link[:colon])]
	return ok
}

// Preserve original order while flattening multi-endpoint entries.
func standaloneSubscriptionLinks(links []string) []string {
	out := make([]string, 0)
	for _, entry := range links {
		for _, link := range splitLinkLines(entry) {
			if isStandaloneSubscriptionLink(link) {
				out = append(out, link)
			}
		}
	}
	return out
}

// coreSubscriptionLinks keeps URI schemes supported by at least one of the
// two cores, plus verified Mieru URIs for Hiddify. Unknown external schemes
// require opting in.
func coreSubscriptionLinks(links []string, hiddify bool) []string {
	var result []string
	for _, entry := range links {
		for _, link := range splitLinkLines(entry) {
			if isCoreSubscriptionLink(link) || hiddify && strings.HasPrefix(strings.ToLower(link), "mierus://") {
				result = append(result, link)
			}
		}
	}
	return result
}

func isCoreSubscriptionLink(link string) bool {
	link = strings.TrimSpace(link)
	scheme, _, ok := strings.Cut(link, "://")
	if !ok {
		return false
	}
	switch strings.ToLower(scheme) {
	case "http", "https":
		proxy, err := url.Parse(link)
		if err != nil || proxy.Hostname() == "" {
			return false
		}
		host := strings.ToLower(proxy.Hostname())
		return host != "t.me" && host != "telegram.me" && host != "telegram.dog"
	case "vless", "vmess", "trojan", "ss", "socks", "socks5",
		"hysteria2", "hy2", "wireguard", "wg", "tuic", "anytls", "shadowtls", "snell",
		"naive", "naive+https", "naive+quic":
		return true
	}
	return false
}
