package model

import (
	"strings"
	"testing"
)

func TestClientConnectionNameValidationAndRoundTrip(t *testing.T) {
	name := "  Домашний VPN / #1  "
	c := Client{ConnectionName: &name}
	if err := c.ValidateConnectionName(); err != nil {
		t.Fatal(err)
	}
	if got := c.ToRecord().ToClient().ConnectionDisplayName(); got != "Домашний VPN / #1" {
		t.Fatal(got)
	}
	for _, invalid := range []string{strings.Repeat("я", 257), "a\nb", "a\x00b", "a\u0085b"} {
		c.ConnectionName = &invalid
		if err := c.ValidateConnectionName(); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
}

func TestMergeClientConnectionNamePreservesOmissionAndHonorsNewerClear(t *testing.T) {
	name := "VPN"
	existing := &ClientRecord{ConnectionName: &name, UpdatedAt: 2}
	MergeClientRecord(existing, &ClientRecord{UpdatedAt: 3})
	if existing.ConnectionName == nil || *existing.ConnectionName != name {
		t.Fatal("omission erased name")
	}
	empty := ""
	MergeClientRecord(existing, &ClientRecord{ConnectionName: &empty, UpdatedAt: 1})
	if *existing.ConnectionName != name {
		t.Fatal("old node snapshot erased name")
	}
	MergeClientRecord(existing, &ClientRecord{ConnectionName: &empty, UpdatedAt: 4})
	if existing.ConnectionName == nil || *existing.ConnectionName != "" {
		t.Fatal("explicit clear lost")
	}
}
