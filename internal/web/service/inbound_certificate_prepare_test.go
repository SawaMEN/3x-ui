package service

import (
	"context"
	"testing"
)

func TestInboundCertificateEnrollmentPreservesOtherNodes(t *testing.T) {
	setupBulkDB(t)
	name := "web.site.ru"
	for _, id := range []int{2, 3, 2} {
		if err := enrollMainCertificateTarget(name, id, false); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := CoverCertificateFleets()
	if err != nil || len(rows) != 1 || len(rows[0].Targets) != 2 || !rows[0].MainCertificate {
		t.Fatalf("additive enrollment: %+v %v", rows, err)
	}
	if err := enrollMainCertificateTarget(name, 2, true); err != nil {
		t.Fatal(err)
	}
	rows, _ = CoverCertificateFleets()
	if !rows[0].Targets[0].RSAOnly {
		t.Fatal("RSA requirement was not persisted")
	}
	if err := enrollMainCertificateTarget(name, 2, false); err != nil {
		t.Fatal(err)
	}
	rows, _ = CoverCertificateFleets()
	if !rows[0].Targets[0].RSAOnly {
		t.Fatal("RSA requirement was downgraded")
	}
}

func TestInboundCertificateEnrollmentRejectsConflictingSource(t *testing.T) {
	setupBulkDB(t)
	rows := []CoverCertificateFleet{{SourceNodeID: 7, Domain: "web.site.ru", Targets: []CoverCertificateTarget{{NodeID: 3, State: "ready"}}}}
	if err := saveFirewallJSON(coverCertificateFleetKey, rows); err != nil {
		t.Fatal(err)
	}
	if err := enrollMainCertificateTarget("web.site.ru", 3, false); err == nil {
		t.Fatal("overrode another source")
	}
	stored, _ := CoverCertificateFleets()
	if len(stored) != 1 || stored[0].SourceNodeID != 7 {
		t.Fatalf("existing source changed: %+v", stored)
	}
}

func TestInboundCertificateStatusDoesNotIssueOrEnroll(t *testing.T) {
	setupBulkDB(t)
	before, err := (&SettingService{}).getString("domainFrontConfig")
	if err != nil {
		t.Fatal(err)
	}
	status, err := InboundCertificateStatus(context.Background(), InboundCertificateRequest{Domain: "web.site.ru"})
	if err != nil || status.State != "not_requested" || status.CertFile != "" {
		t.Fatalf("unexpected read-only status: %+v %v", status, err)
	}
	after, err := (&SettingService{}).getString("domainFrontConfig")
	if err != nil || before != after {
		t.Fatal("status changed domain configuration")
	}
	rows, err := CoverCertificateFleets()
	if err != nil || len(rows) != 0 {
		t.Fatal("status enrolled a recipient")
	}
}

func TestInboundCertificatePrepareValidatesBeforeSideEffects(t *testing.T) {
	setupBulkDB(t)
	for _, req := range []InboundCertificateRequest{
		{Domain: "https://web.site.ru"}, {Domain: "web.site.ru", NodeID: -1}, {Domain: "web.site.ru", RSAOnly: true},
	} {
		if _, err := (&ManagerService{}).PrepareInboundCertificate(context.Background(), 1, req); err == nil {
			t.Fatalf("invalid preparation accepted: %+v", req)
		}
	}
	if _, err := (&ManagerService{}).PrepareInboundCertificate(context.Background(), 0, InboundCertificateRequest{Domain: "web.site.ru"}); err == nil {
		t.Fatal("accepted missing administrator")
	}
	rows, _ := CoverCertificateFleets()
	if len(rows) != 0 {
		t.Fatal("invalid preparation enrolled nodes")
	}
}
