package sub

import "testing"

func TestSplitClashHostPort(t *testing.T) {
	for _, test := range []struct {
		endpoint, host string
		port           int
	}{
		{"host.example:51820", "host.example", 51820},
		{"127.0.0.1:51820", "127.0.0.1", 51820},
		{"[2001:db8::1]:51820", "2001:db8::1", 51820},
		{"2001:db8::1", "2001:db8::1", 0},
		{"host.example:-1", "host.example", 0},
		{"host.example:65536", "host.example", 0},
		{"host.example:bad", "host.example", 0},
	} {
		t.Run(test.endpoint, func(t *testing.T) {
			host, port := splitClashHostPort(test.endpoint)
			if host != test.host || port != test.port {
				t.Fatalf("got %s:%d, want %s:%d", host, port, test.host, test.port)
			}
		})
	}
}
