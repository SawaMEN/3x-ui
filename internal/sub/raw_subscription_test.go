package sub

import (
	"strings"
	"testing"
)

func TestFormatRawSubscriptionLinksGroupsByProtocol(t *testing.T) {
	links := []string{
		"vless://one@example.com",
		"trojan://two@example.com",
		"vless://three@example.com",
		"mierus://four@example.com",
		"trojan://five@example.com",
	}

	got := formatRawSubscriptionLinks(links)

	wantSections := []string{
		"# VLESS",
		"vless://one@example.com",
		"vless://three@example.com",
		"# Trojan",
		"trojan://two@example.com",
		"trojan://five@example.com",
		"# Mieru",
		"mierus://four@example.com",
	}
	pos := -1
	for _, want := range wantSections {
		next := strings.Index(got, want)
		if next == -1 {
			t.Fatalf("subscription is missing %q:\n%s", want, got)
		}
		if next < pos {
			t.Fatalf("subscription section/link order is wrong around %q:\n%s", want, got)
		}
		pos = next
	}

	if strings.Contains(got, "vless://one@example.com\ntrojan://two@example.com") {
		t.Fatalf("VLESS and Trojan links must not be interleaved:\n%s", got)
	}
	if strings.Contains(got, "trojan://two@example.com\nvless://three@example.com") {
		t.Fatalf("protocol groups must stay separate:\n%s", got)
	}
}

func TestFormatRawSubscriptionLinksEmpty(t *testing.T) {
	if got := formatRawSubscriptionLinks(nil); got != "" {
		t.Fatalf("empty subscription = %q, want empty", got)
	}
}
