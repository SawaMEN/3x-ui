package service

import (
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestManagerHealthySNIRequiresStableDNSCertificate(t *testing.T) {
	ok := true
	good := &RealityScanResult{
		Host: "site.example.org", Port: 443, Feasible: true, CertValid: true,
		TLS13: true, H2: true, X25519: true, Attempts: 5, Successes: 4, DNSMatchesIP: &ok,
	}
	if !managerHealthySNI(good, "site.example.org") {
		t.Fatal("stable SNI with matching DNS rejected")
	}
	bad := *good
	bad.DNSMatchesIP = nil
	if managerHealthySNI(&bad, good.Host) {
		t.Fatal("a neighboring certificate name with unverified DNS was accepted")
	}
	bad = *good
	bad.Successes = 3
	if managerHealthySNI(&bad, good.Host) {
		t.Fatal("unstable TLS target accepted")
	}
	bad = *good
	bad.Port = 8443
	if managerHealthySNI(&bad, good.Host) {
		t.Fatal("an unsupported live target port accepted")
	}
	bad = *good
	bad.PrivateTarget = true
	if managerHealthySNI(&bad, good.Host) {
		t.Fatal("private target accepted")
	}
}

func TestManagerSNIHealthHysteresisAndCooldown(t *testing.T) {
	now := time.Now().UnixMilli()
	entry := model.ManagerEntry{}
	for failures := 1; failures <= 2; failures++ {
		entry.SNIHealthFailures, _ = managerSNIHealthDecision(entry, false, now)
		if entry.SNIHealthFailures != failures {
			t.Fatalf("expected %d independent failures, got %d", failures, entry.SNIHealthFailures)
		}
	}
	failures, switchNow := managerSNIHealthDecision(entry, false, now)
	if failures != 3 || !switchNow {
		t.Fatalf("three consecutive failures must permit switching: %d %t", failures, switchNow)
	}
	entry.SNIHealthFailures = 3
	entry.SNIHealthLastSwitchAt = now
	_, switchNow = managerSNIHealthDecision(entry, false, now+int64(time.Minute/time.Millisecond))
	if switchNow {
		t.Fatal("cooldown did not prevent rapid oscillation")
	}
	failures, switchNow = managerSNIHealthDecision(entry, true, now)
	if failures != 0 || switchNow {
		t.Fatal("healthy check did not clear failures")
	}
}

func TestManagerSNIHealthPreservesPreviousHostAndCredentials(t *testing.T) {
	entry := model.ManagerEntry{
		SNI:        "old.example.org",
		SNIBackups: []string{"next.example.org", "third.example.org"},
	}
	backup := managerSNIHealthBackups(entry, "next.example.org")
	if !slices.Contains(backup, "old.example.org") ||
		slices.Contains(backup, "next.example.org") {
		t.Fatalf("previous SNI was lost or active one duplicated: %v", backup)
	}
	old := `{"security":"reality","realitySettings":{"privateKey":"important","shortIds":["deadbeef"],"serverNames":["old.example.org"],"dest":"old.example.org:443"}}`
	next := `{"security":"reality","realitySettings":{"privateKey":"important","shortIds":["deadbeef"],"serverNames":["next.example.org"],"dest":"next.example.org:443"}}`
	if !managerSNIHealthRealityKeysEqual(old, next) {
		t.Fatal("changing SNI incorrectly classified as changing REALITY keys")
	}
	changedKey := `{"security":"reality","realitySettings":{"privateKey":"different","shortIds":["deadbeef"]}}`
	if managerSNIHealthRealityKeysEqual(old, changedKey) {
		t.Fatal("REALITY private key rotation was overlooked")
	}
}

func TestManagerSNIHealthQueuedOperationDoesNotMutateDisabledSubscriptions(t *testing.T) {
	svc, _, sub := managerFixture(t)
	sub.MonitorSNI = false
	op := &model.ManagerOperation{Total: 1}
	if err := svc.monitorSNI(t.Context(), op, sub); err != nil {
		t.Fatal(err)
	}
	if op.Message == "" || op.Completed != 0 {
		t.Fatalf("disabled monitor unexpectedly changed generation: %+v", op)
	}
}

func TestManagerCheckCertificatesNativeTLSPaths(t *testing.T) {
	missing := t.TempDir() + "/nonexistent.pem"
	for _, ib := range []*model.Inbound{
		{Protocol: model.TUIC, Settings: `{"server":{"certificate":"` + missing + `","private_key":"` + missing + `","sni":"site.example.org"}}`, StreamSettings: "{}"},
		{Protocol: model.TrustTunnel, Settings: `{"hostname":"site.example.org","certificate":"` + missing + `","privateKey":"` + missing + `"}`, StreamSettings: "{}"},
		{Protocol: model.NaiveProxy, Settings: `{"tls":{"enabled":true,"serverName":"site.example.org","certificatePath":"` + missing + `","keyPath":"` + missing + `"}}`, StreamSettings: "{}"},
	} {
		if err := managerCheckCertificates(ib); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s missing TLS file not detected: %v", ib.Protocol, err)
		}
	}
}
