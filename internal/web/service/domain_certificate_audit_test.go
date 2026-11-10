package service

import (
	"strings"
	"testing"
	"time"
)

func TestCertificateAuditBoundedAndSanitized(t *testing.T) {
	entries := []CertificateAuditEntry{}
	for i := 0; i < certificateAuditLimit+25; i++ {
		entries = boundedCertificateAudit(entries, CertificateAuditEntry{At: int64(i), Manager: "certmagic"})
	}
	if len(entries) != certificateAuditLimit || entries[0].At != 25 {
		t.Fatalf("unexpected audit retention: size=%d first=%d", len(entries), entries[0].At)
	}
	tests := []struct{ input, want string }{
		{"Example.ORG.", "example.org"},
		{"*.Example.org", "*.example.org"},
		{"a.b.example.org", "a.b.example.org"},
		{"user:token@example.org", ""},
		{"example.org/path", ""},
		{"evil..example.org", ""},
		{"-example.org", ""},
		{strings.Repeat("x", 64) + ".example.org", ""},
	}
	for _, tc := range tests {
		if got := certificateAuditDomain(tc.input); got != tc.want {
			t.Errorf("certificateAuditDomain(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCertificateHealthChangesOnlyAndSkipsRemote(t *testing.T) {
	now := time.Unix(1760000000, 0)
	items := []CertificateInventoryItem{
		{ID: "abc", Manager: "certmagic", Name: "soon.example.org", ExpiresAt: now.Add(10 * 24 * time.Hour).Unix()},
		{ID: "def", Manager: "manual", Name: "bad.example.org", State: "error"},
		{ID: "remote", Manager: "remote", Name: "node.example.org", NodeID: 9, ExpiresAt: now.Add(-time.Hour).Unix()},
		{ID: "good", Manager: "certbot", Name: "good.example.org", ExpiresAt: now.Add(60 * 24 * time.Hour).Unix()},
	}
	first, events := changedCertificateHealth(map[string]string{}, items, now)
	if len(first) != 2 || first["abc"] != "expiring" || first["def"] != "failed" || len(events) != 2 {
		t.Fatalf("unexpected initial state: %v events=%+v", first, events)
	}
	again, events := changedCertificateHealth(first, items, now)
	if len(events) != 0 || len(again) != 2 {
		t.Fatalf("unchanged health should not generate events: %+v", events)
	}
	items[0].ExpiresAt = now.Add(90 * 24 * time.Hour).Unix()
	items[1].State = "valid"
	items[1].ExpiresAt = now.Add(90 * 24 * time.Hour).Unix()
	cleared, events := changedCertificateHealth(first, items, now)
	if len(cleared) != 0 || len(events) != 2 {
		t.Fatalf("recoveries were not recorded: state=%+v events=%+v", cleared, events)
	}
	for _, event := range events {
		if event.Outcome != "recovered" || event.Manager == "remote" {
			t.Fatalf("invalid recovery: %+v", event)
		}
	}
}

func TestCertificateHealthTransitionsToExpired(t *testing.T) {
	now := time.Now()
	if got := certificateHealthState(CertificateInventoryItem{
		ID: "a", Manager: "acme.sh", ExpiresAt: now.Add(-time.Minute).Unix(),
	}, now); got != "expired" {
		t.Fatalf("expired certificate state: %s", got)
	}
}
