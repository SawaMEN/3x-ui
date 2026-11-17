package externalvpn

import "testing"

func TestOpenFluxProbeHostAllowlist(t *testing.T) {
	for _, typ := range []string{"yandex", "vyandex", "boards", "mailru", "cupsonline", "avito", "telemost", "oneme"} {
		host, ok := openFluxProbeHost(typ)
		if !ok || host == "" {
			t.Fatalf("missing public provider target for %s", typ)
		}
	}
	for _, typ := range []string{"", "direct", "http://127.0.0.1", "localhost", "oneme;127.0.0.1", "docs.yandex.ru"} {
		if host, ok := openFluxProbeHost(typ); ok || host != "" {
			t.Fatalf("unexpected target for %q: %s", typ, host)
		}
	}
}
