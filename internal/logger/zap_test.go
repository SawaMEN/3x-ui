package logger

import (
	"errors"
	"strings"
	"testing"

	"github.com/op/go-logging"
	"go.uber.org/zap"
)

func TestCertificateLibraryDiagnosticsReachPersistentLogAndBuffer(t *testing.T) {
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	InitLogger(logging.WARNING)
	t.Cleanup(CloseLogger)
	certLogger := zap.New(NewZapCore()).Named("tls.obtain").With(zap.String("domain", "example.org"), zap.Any("request", map[string]string{"Authorization": "PRIVATE_HEADER"}))
	certLogger.Error("certificate issuance failed", zap.Error(errors.New("DNS update failed token=PRIVATE_TOKEN")), zap.String("response", "PRIVATE_BODY"))
	certLogger.Warn("renewal will retry")
	certLogger.Info("PRIVATE_VERBOSE")
	for _, logs := range []string{strings.Join(GetFileLogs(100, "debug"), "\n"), strings.Join(GetLogs(100, "debug"), "\n")} {
		for _, want := range []string{"tls.obtain", "example.org", "certificate issuance failed", "DNS update failed", "renewal will retry"} {
			if !strings.Contains(logs, want) {
				t.Errorf("missing %q in %s", want, logs)
			}
		}
		for _, secret := range []string{"PRIVATE_HEADER", "PRIVATE_TOKEN", "PRIVATE_BODY", "PRIVATE_VERBOSE"} {
			if strings.Contains(logs, secret) {
				t.Errorf("secret leaked: %s", secret)
			}
		}
	}
}
