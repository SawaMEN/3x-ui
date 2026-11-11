package service

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

func auditDomainPorts(t *testing.T) (int, int) {
	t.Helper()
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	return first.Addr().(*net.TCPAddr).Port, second.Addr().(*net.TCPAddr).Port
}

func auditDomainTLSReachable(t *testing.T, port int) {
	t.Helper()
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), &tls.Config{ServerName: "golden-fixture.test", InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("existing certificate listener unavailable: %v", err)
	}
	defer conn.Close()
	if len(conn.ConnectionState().PeerCertificates) == 0 {
		t.Fatal("listener did not present its certificate")
	}
}

func TestAuditDomainCommitFailureRestoresExistingTLSGateway(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancelRequest), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XUI_DB_FOLDER", dir)
			if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { StopDomainFront(); _ = database.CloseDB() })
			certPath, keyPath := writeTestCertificate(t)
			settings := &SettingService{}
			for key, value := range map[string]string{"webCertFile": certPath, "webKeyFile": keyPath} {
				if err := settings.setString(key, value); err != nil {
					t.Fatal(err)
				}
			}
			cfg := domainfront.DefaultConfig()
			cfg.Enabled, cfg.Listen = true, "127.0.0.1"
			cfg.HTTPPort, cfg.HTTPSPort = auditDomainPorts(t)
			cfg.PublicPort = cfg.HTTPSPort
			cfg.Domains = []domainfront.DomainFrontDomain{{Name: "golden-fixture.test", Mode: "sni_only", TLS: "existing", Enabled: true}}
			if status, err := ApplyDomainFront(context.Background(), cfg); err != nil || !status.Running {
				t.Fatalf("initial TLS gateway: running=%v err=%v", status.Running, err)
			}
			auditDomainTLSReachable(t, cfg.HTTPSPort)
			before, err := settings.getString("domainFrontConfig")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			callback := database.GetDB().Callback().Update()
			if err := callback.Before("gorm:update").Register("audit:domain-commit-failure", func(tx *gorm.DB) {
				if row, ok := tx.Statement.Dest.(*model.Setting); ok && row.Key == "domainFrontConfig" {
					if cancelRequest {
						cancel()
					}
					tx.AddError(errors.New("injected domain config commit failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = callback.Remove("audit:domain-commit-failure") })
			next := cfg
			next.Enabled = false
			status, err := ApplyDomainFront(ctx, next)
			if err == nil {
				t.Fatal("expected domain configuration commit failure")
			}
			after, readErr := settings.getString("domainFrontConfig")
			if readErr != nil || before != after {
				t.Fatalf("failed commit changed stored config: %v", readErr)
			}
			if !status.Running {
				t.Fatalf("failed commit stopped previously running gateway: %v", err)
			}
			auditDomainTLSReachable(t, cfg.HTTPSPort)
		})
	}
}
