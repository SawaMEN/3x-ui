package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

func TestExternalRenewalDoesNotAcceptUnknownOrRemoteCertificate(t *testing.T) {
	cfg := domainfront.DefaultConfig()
	status := domainfront.DomainFrontStatus{Config: cfg}
	status.Certificates = []domainfront.DomainFrontCertificate{
		{Name: "remote.example.org", Source: "remote", State: "external"},
		{Name: "manual.example.org", Source: "manual", Renewal: "manual", Mode: "manual", CertFile: "/tmp/manual.pem", KeyFile: "/tmp/manual.key"},
	}
	items := makeCertificateInventory(status, nil, nil)
	for _, item := range items {
		if _, _, _, _, err := localCertificateRenewalPlan(item.ID, status, nil, nil); err == nil {
			t.Fatalf("unexpected renewal permission for %+v", item)
		}
	}
}

func TestExternalRenewalRejectsMismatchedCertificateIdentifier(t *testing.T) {
	if _, _, _, _, err := localCertificateRenewalPlan("untrusted", domainfront.DomainFrontStatus{}, nil, nil); err == nil {
		t.Fatal("an arbitrary renewal command was accepted")
	}
}
