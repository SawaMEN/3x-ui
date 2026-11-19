package sub

import "strings"

// These share links require clients outside the stock Xray and sing-box
// protocols. This is a UI/URI classification; it does not imply a common
// client app can import every scheme.
var standaloneSubscriptionSchemes = map[string]struct{}{
	"pingtunnel": {}, "fptn": {}, "openflux": {}, "vetroff": {},
	"tt": {}, "sudoku": {}, "tg": {},
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
