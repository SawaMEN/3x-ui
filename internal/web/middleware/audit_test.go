package middleware

import "testing"

func TestAuditSafeMethods(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "OPTIONS", "TRACE"} {
		if !isAuditSafeMethod(method) {
			t.Fatalf("%s should be audit-safe", method)
		}
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if isAuditSafeMethod(method) {
			t.Fatalf("%s should be audited", method)
		}
	}
}

func TestAuditNoiseRoutes(t *testing.T) {
	if !isAuditNoiseRoute("/panel/api/inbounds/pushClientTraffics") {
		t.Fatal("traffic sync route should be excluded")
	}
	if !isAuditNoiseRoute("/panel/api/server/clientIps") {
		t.Fatal("client IP sync route should be excluded")
	}
	if isAuditNoiseRoute("/panel/api/setting/update") {
		t.Fatal("settings mutation should be audited")
	}
}
