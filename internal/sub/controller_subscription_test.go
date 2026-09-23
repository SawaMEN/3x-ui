package sub

import "testing"

func TestFormatRawSubscriptionLinksGroupsProtocols(t *testing.T) {
	got := formatRawSubscriptionLinks([]string{
		"vless://user@vless.example:443#vless-1",
		"vmess://eyJ2IjoiMiJ9#vmess-1",
		"vless://user@vless2.example:8443#vless-2",
		"trojan://secret@trojan.example:443#trojan-1",
		"ss://method:password@ss.example:8388#ss-1",
	})

	want := "# VLESS\n" +
		"vless://user@vless.example:443#vless-1\n" +
		"vless://user@vless2.example:8443#vless-2\n\n" +
		"# VMess\n" +
		"vmess://eyJ2IjoiMiJ9#vmess-1\n\n" +
		"# Trojan\n" +
		"trojan://secret@trojan.example:443#trojan-1\n\n" +
		"# Shadowsocks\n" +
		"ss://method:password@ss.example:8388#ss-1\n"
	if got != want {
		t.Fatalf("grouped subscription = %q, want %q", got, want)
	}
}

func TestFormatRawSubscriptionLinksKeepsMultipleLinksFromOneEntry(t *testing.T) {
	got := formatRawSubscriptionLinks([]string{
		"vless://one.example:443#one\n vless://two.example:443#two",
		"naive+https://user:secret@naive.example:443?sni=naive.example&padding=true#naive",
		"mierus://user:secret@mieru.example:2101?protocol=TCP&port=2101#m",
	})

	want := "# VLESS\n" +
		"vless://one.example:443#one\n" +
		"vless://two.example:443#two\n\n" +
		"# NaiveProxy TCP\n" +
		"naive+https://user:secret@naive.example:443?sni=naive.example&padding=true#naive\n\n" +
		"# Mieru\n" +
		"mierus://user:secret@mieru.example:2101?protocol=TCP&port=2101#m\n"
	if got != want {
		t.Fatalf("grouped multi-link subscription = %q, want %q", got, want)
	}
}

func TestRawSubscriptionSchemeLabels(t *testing.T) {
	cases := map[string]string{
		"vless": "VLESS",
		"vmess": "VMess",
		"trojan": "Trojan",
		"ss": "Shadowsocks",
		"hysteria2": "Hysteria2",
		"tuic": "TUIC",
		"wireguard": "WireGuard",
		"amneziawg": "AmneziaWG",
		"mtproto": "MTProto",
		"naive+https": "NaiveProxy TCP",
		"naive+quic": "NaiveProxy QUIC",
		"mierus": "Mieru",
	}
	for scheme, want := range cases {
		if got := rawSubscriptionProtocolLabel(scheme); got != want {
			t.Fatalf("label(%q) = %q, want %q", scheme, got, want)
		}
	}
}
