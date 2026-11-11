package domainfront

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"
)

func TestAuditFailedInitialCommitReleasesGatewayListeners(t *testing.T) {
	cert, key, _ := testCertificate(t, "commit.example.com", 45)
	cfg := DefaultConfig()
	cfg.Enabled, cfg.Listen = true, "127.0.0.1"
	cfg.HTTPPort, cfg.HTTPSPort = unusedPort(t), unusedPort(t)
	cfg.Domains = []DomainFrontDomain{{Name: "commit.example.com", Enabled: true, Mode: "sub_link_only", TLS: "existing"}}
	backend := Backend{
		SubscriptionTarget: "http://127.0.0.1:1",
		Certificates:       map[string]CertificateFiles{"commit.example.com": {CertFile: cert, KeyFile: key}},
	}
	manager := NewManager(t.TempDir())
	defer manager.Stop()
	commitErr := errors.New("commit rejected")
	err := manager.ApplyAndCommit(context.Background(), cfg, backend, func() error { return commitErr })
	if !errors.Is(err, commitErr) || manager.Status(cfg).Running {
		t.Fatalf("failed commit: running=%v err=%v", manager.Status(cfg).Running, err)
	}
	for _, port := range []int{cfg.HTTPPort, cfg.HTTPSPort} {
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			t.Fatalf("failed commit retained port %d: %v", port, err)
		}
		_ = listener.Close()
	}
}

func TestAuditCommitRunsOnlyAfterSuccessfulValidationAndStart(t *testing.T) {
	cert, key, _ := testCertificate(t, "commit.example.com", 45)
	cfg := DefaultConfig()
	cfg.Enabled, cfg.Listen = true, "127.0.0.1"
	cfg.HTTPPort, cfg.HTTPSPort = unusedPort(t), unusedPort(t)
	cfg.Domains = []DomainFrontDomain{{Name: "commit.example.com", Enabled: true, Mode: "sub_link_only", TLS: "existing"}}
	backend := Backend{SubscriptionTarget: "http://127.0.0.1:1", Certificates: map[string]CertificateFiles{"commit.example.com": {CertFile: cert, KeyFile: key}}}
	manager := NewManager(t.TempDir())
	defer manager.Stop()
	commits := 0
	commit := func() error { commits++; return nil }
	if err := manager.ApplyAndCommit(context.Background(), cfg, backend, commit); err != nil || commits != 1 || !manager.Status(cfg).Running {
		t.Fatalf("successful commit: calls=%d err=%v", commits, err)
	}
	invalid := cfg
	invalid.HTTPSPort = invalid.HTTPPort
	if err := manager.ApplyAndCommit(context.Background(), invalid, backend, commit); err == nil || commits != 1 || !manager.Status(cfg).Running {
		t.Fatalf("invalid draft changed previous gateway or committed: calls=%d err=%v", commits, err)
	}
}
