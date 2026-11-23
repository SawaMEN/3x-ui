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

func TestBuildRawSubscriptionBodyOmitsTelegramProxyLinks(t *testing.T) {
	got := buildRawSubscriptionBody([]string{
		"tg://proxy?server=telegram.example&port=443&secret=ee1234",
		"tg://webproxy?server=telegram.example&secret=ee1234\nss://vpn-profile",
		"https://t.me/proxy?server=telegram.example&port=443&secret=ee1234",
		"https://Telegram.Me/socks?server=telegram.example&port=1080",
		"https://vpn.example:443#HTTPS proxy",
	})
	if want := "ss://vpn-profile\nhttps://vpn.example:443#HTTPS proxy\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
