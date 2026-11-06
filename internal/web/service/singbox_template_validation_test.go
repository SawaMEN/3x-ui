package service

import "testing"

func TestNormalizeSingBoxTemplateRejectsNonObject(t *testing.T) {
	for _, raw := range []string{"null", "[]", "42", `"value"`} {
		if _, err := normalizeSingBoxTemplate(raw); err == nil {
			t.Errorf("accepted invalid settings %s", raw)
		}
	}
	if _, err := normalizeSingBoxTemplate(`{"dns":{"servers":[]}}`); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeSingBoxTemplateRejectsOxiDNSRuntimeOverlay(t *testing.T) {
	for _, raw := range []string{
		`{"dns":{"servers":[{"type":"udp","tag":"oxidns-local","server":"127.0.0.1","server_port":15353}],"final":"oxidns-local"}}`,
		`{"dns":{"servers":[],"final":"oxidns-local"}}`,
	} {
		if _, err := normalizeSingBoxTemplate(raw); err == nil {
			t.Fatalf("allowed runtime-only OxiDNS DNS to be persisted: %s", raw)
		}
	}
	if _, err := normalizeSingBoxTemplate(`{"dns":{"servers":[{"tag":"google","type":"https","server":"dns.google"}],"final":"google"}}`); err != nil {
		t.Fatalf("rejected user DNS config: %v", err)
	}
}
