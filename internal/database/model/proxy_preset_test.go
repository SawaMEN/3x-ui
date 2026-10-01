package model

import "testing"

func strPtr(v string) *string { return &v }
func boolPtr(v bool) *bool    { return &v }
func intPtr(v int) *int       { return &v }

func TestApplyProxyPresetConfigOnlyTouchesExplicitFields(t *testing.T) {
	h := &Host{
		GroupId:        "group-a",
		InboundId:      7,
		Remark:         "edge",
		Address:        "edge.example.com",
		Port:           443,
		Security:       "same",
		Sni:            "old.example.com",
		AllowInsecure:  true,
		MihomoIpVersion: "dual",
	}
	cfg := ProxyPresetConfig{
		Port:          intPtr(8443),
		Security:      strPtr("tls"),
		Sni:           strPtr("preset.example.com"),
		AllowInsecure: boolPtr(false),
	}

	ApplyProxyPresetConfig(h, cfg)

	if h.Port != 8443 || h.Security != "tls" || h.Sni != "preset.example.com" || h.AllowInsecure {
		t.Fatalf("preset was not applied: %+v", h)
	}
	if h.GroupId != "group-a" || h.InboundId != 7 || h.Remark != "edge" || h.Address != "edge.example.com" {
		t.Fatalf("identity fields changed: %+v", h)
	}
	if h.MihomoIpVersion != "dual" {
		t.Fatalf("unset preset field changed: %q", h.MihomoIpVersion)
	}
}

func TestApplyProxyPresetConfigCanExplicitlyClearValues(t *testing.T) {
	empty := ""
	emptySlice := []string{}
	h := &Host{Sni: "old", Alpn: []string{"h2"}, Security: "tls"}
	cfg := ProxyPresetConfig{Sni: &empty, Alpn: &emptySlice}

	ApplyProxyPresetConfig(h, cfg)

	if h.Sni != "" {
		t.Fatalf("SNI = %q, want cleared", h.Sni)
	}
	if len(h.Alpn) != 0 {
		t.Fatalf("ALPN = %#v, want cleared", h.Alpn)
	}
	if h.Security != "tls" {
		t.Fatalf("unset security changed to %q", h.Security)
	}
}
