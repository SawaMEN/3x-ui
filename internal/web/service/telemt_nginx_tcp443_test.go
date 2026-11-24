package service

import "testing"

func TestTelemtNginxForeignTCP443(t *testing.T) {
	for _, tc := range []struct {
		name string
		dump string
		want bool
	}{
		{"QUIC", "server { listen 443 quic reuseport; }", false},
		{"UDP stream", "server { listen [::]:443 udp; }", false},
		{"TCP", "server { listen 443 ssl; }", true},
		{"TCP and QUIC", "server { listen 443 quic; listen 443 ssl; }", true},
		{"multiline QUIC", "server { listen\n [::]:443 # comment\n quic; }", false},
		{"multiline TCP", "server { listen\n [::]:443\n ssl; }", true},
		{"quoted address", "server { listen \"127.0.0.1:443\" ssl; }", true},
		{"comment", "# listen 443 ssl;\nserver { listen 80; }", false},
		{"quoted text", "set $text 'listen 443 ssl;';", false},
		{"quoted #", "set $text 'a#b'; listen 443;", true},
		{"other port", "listen 1443 ssl;", false},
		{"owned file", "# configuration file /owned.conf:\nlisten 443 ssl;", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dump := "# configuration file /foreign.conf:\n" + tc.dump
			if got := nginxHasForeignTCP443(dump, "/owned.conf"); got != tc.want {
				t.Fatalf("foreign TCP 443 = %v, want %v", got, tc.want)
			}
		})
	}
}
