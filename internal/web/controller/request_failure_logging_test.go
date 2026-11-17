package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/middleware"
)

func TestApplicationFailuresReachErrorLogsWithSafeContext(t *testing.T) {
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	logger.InitLogger(logging.ERROR)
	t.Cleanup(logger.CloseLogger)
	router := gin.New()
	router.Use(middleware.RequestFailures())
	router.POST("/PRIVATE_BASE/panel/api/nodes/:token", func(c *gin.Context) { jsonObj(c, nil, errors.New("apply failed password=PRIVATE_PASSWORD")) })
	router.POST("/PRIVATE_BASE/panel/api/denied/:token", func(c *gin.Context) { pureJsonMsg(c, 403, false, "denied token=PRIVATE_TOKEN") })
	for _, path := range []string{"nodes", "denied"} {
		req := httptest.NewRequest(http.MethodPost, "/PRIVATE_BASE/panel/api/"+path+"/PRIVATE_ID?key=PRIVATE_QUERY", nil)
		router.ServeHTTP(httptest.NewRecorder(), req)
	}
	logs := strings.Join(logger.GetFileLogs(100, "error"), "\n")
	for _, want := range []string{"/panel/api/nodes/:token", "handler=", "request_failure_logging_test.go", "apply failed", "denied"} {
		if !strings.Contains(logs, want) {
			t.Errorf("missing %q in %s", want, logs)
		}
	}
	for _, secret := range []string{"PRIVATE_BASE", "PRIVATE_ID", "PRIVATE_QUERY", "PRIVATE_PASSWORD", "PRIVATE_TOKEN"} {
		if strings.Contains(logs, secret) {
			t.Errorf("secret leaked: %s", secret)
		}
	}
	if strings.Count(logs, "operation failed") != 2 || strings.Contains(logs, "HTTP request failed") {
		t.Fatalf("duplicate failures: %s", logs)
	}
}
