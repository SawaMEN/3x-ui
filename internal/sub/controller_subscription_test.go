package sub

import "testing"

func TestBuildRawSubscriptionBodyIsUsedForPlainLinks(t *testing.T) {
	got := buildRawSubscriptionBody([]string{
		"vless://user@vless.example:443#vless-1",
		"vmess://eyJ2IjoiMiJ9#vmess-1",
		"trojan://secret@trojan.example:443#trojan-1",
		"ss://method:password@ss.example:8388#ss-1",
	})

	want := "vless://user@vless.example:443#vless-1\n" +
		"vmess://eyJ2IjoiMiJ9#vmess-1\n" +
		"trojan://secret@trojan.example:443#trojan-1\n" +
		"ss://method:password@ss.example:8388#ss-1\n"
	if got != want {
		t.Fatalf("plain subscription body = %q, want %q", got, want)
	}
}
