package sub

import (
	"strings"
	"testing"
)

func TestBuildRawSubscriptionBodyPreservesConnectionOrder(t *testing.T) {
	got := buildRawSubscriptionBody([]string{
		"vless://one@example.com",
		"trojan://two@example.com",
		"vless://three@example.com",
		"mierus://four@example.com",
	})

	want := strings.Join([]string{
		"vless://one@example.com",
		"trojan://two@example.com",
		"vless://three@example.com",
		"mierus://four@example.com",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("subscription body = %q, want %q", got, want)
	}

	if strings.Contains(got, "# VLESS") || strings.Contains(got, "# Trojan") || strings.Contains(got, "# Mieru") {
		t.Fatalf("subscription body must contain only importable connection lines: %q", got)
	}
}

func TestBuildRawSubscriptionBodyKeepsMultiLinkEntrySeparate(t *testing.T) {
	got := buildRawSubscriptionBody([]string{
		"vless://one.example:443#one\n vless://two.example:443#two ",
		"trojan://secret@trojan.example:443#trojan",
	})

	want := "vless://one.example:443#one\n" +
		"vless://two.example:443#two\n" +
		"trojan://secret@trojan.example:443#trojan\n"
	if got != want {
		t.Fatalf("subscription body = %q, want %q", got, want)
	}
}

func TestBuildRawSubscriptionBodyEmpty(t *testing.T) {
	if got := buildRawSubscriptionBody(nil); got != "" {
		t.Fatalf("empty subscription body = %q, want empty", got)
	}
}
