package service

import "testing"

func TestAppendFrozenInboundRules(t *testing.T) {
	desired := []FirewallRule{{Port: 443, Protocol: "tcp", Source: "panel"}}
	managed := []FirewallRule{
		{Port: 443, Protocol: "tcp", Source: "panel"},
		{Port: 8443, Protocol: "tcp", Source: "inbound", Label: "VLESS"},
		{Port: 53, Protocol: "udp", Source: "manual"},
	}
	got := appendFrozenInboundRules(desired, managed)
	if len(got) != 2 {
		t.Fatalf("len = %d, want panel + frozen inbound", len(got))
	}
	if got[0].Port != 443 || got[1].Port != 8443 || got[1].Source != "inbound" {
		t.Fatalf("unexpected frozen rules: %#v", got)
	}
}

func TestNormalizeFirewallLabelUnicode(t *testing.T) {
	label := "  Прокси для мониторинга  "
	if got := normalizeFirewallLabelUnicode(label); got != "Прокси для мониторинга" {
		t.Fatalf("label = %q", got)
	}

	long := ""
	for range 130 {
		long += "я"
	}
	got := normalizeFirewallLabelUnicode(long)
	if len([]rune(got)) != 120 {
		t.Fatalf("rune count = %d, want 120", len([]rune(got)))
	}
	if ![]rune(got)[119:120][0].IsLetter() {
		// unreachable for valid rune truncation; retained as an explicit UTF-8
		// integrity guard without relying on byte length.
		t.Fatal("truncated label ended with invalid rune")
	}
}
