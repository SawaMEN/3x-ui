package service

import "testing"

func TestXrayTemplateUsesFakeDNS(t *testing.T) {
	for _, tc := range []struct {
		name       string
		template   map[string]any
		configured bool
	}{
		{name: "default without FakeDNS", template: map[string]any{"dns": map[string]any{"servers": []any{"1.1.1.1"}}}},
		{name: "empty FakeDNS pool", template: map[string]any{"fakedns": []any{}}},
		{name: "configured FakeDNS pool", template: map[string]any{"fakedns": []any{map[string]any{"ipPool": "198.18.0.0/15"}}}, configured: true},
		{name: "FakeDNS server name", template: map[string]any{"dns": map[string]any{"servers": []any{"fakedns"}}}, configured: true},
		{name: "FakeDNS server object", template: map[string]any{"dns": map[string]any{"servers": []any{map[string]any{"address": "fakedns"}}}}, configured: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := xrayTemplateUsesFakeDNS(tc.template); got != tc.configured {
				t.Fatalf("configured=%v, want %v", got, tc.configured)
			}
		})
	}
}
