package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/op/go-logging"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

func TestCertificateAndSNIFailuresAreVisibleWithoutDebugLogging(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	logger.InitLogger(logging.ERROR)
	t.Cleanup(logger.CloseLogger)
	if err := RecordCertificateAudit("certmagic", "", "example.org", "renew", "failed"); err != nil {
		t.Fatal(err)
	}
	failed := newSNIJobManager(func(context.Context, *sniJob) error { return errors.New("scan connection failed token=PRIVATE_TOKEN") }, nil)
	defer failed.close()
	job, err := failed.start(SNIScanRequest{Mode: "candidates"})
	if err != nil {
		t.Fatal(err)
	}
	if view := waitSNIJob(t, failed, job.ID); view.State != "failed" {
		t.Fatal(view.State)
	}
	history := newSNIJobManager(func(context.Context, *sniJob) error { return nil }, func(SNIJobSnapshot) error { return errors.New("history disk full") })
	defer history.close()
	job, err = history.start(SNIScanRequest{Mode: "candidates"})
	if err != nil {
		t.Fatal(err)
	}
	waitSNIJob(t, history, job.ID)
	logs := strings.Join(logger.GetFileLogs(100, "error"), "\n")
	for _, want := range []string{"Certificate event", "action=renew outcome=failed", "SNI scan failed", "scan connection failed", "history persistence failed", "history disk full"} {
		if !strings.Contains(logs, want) {
			t.Errorf("missing %q in %s", want, logs)
		}
	}
	if strings.Contains(logs, "PRIVATE_TOKEN") {
		t.Fatal("SNI error leaked token")
	}
}
