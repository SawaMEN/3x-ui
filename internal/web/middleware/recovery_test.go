package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"
)

func TestRecoveryRecordsPanicWithoutRequestSecrets(t *testing.T) {
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	logger.InitLogger(logging.ERROR)
	t.Cleanup(logger.CloseLogger)
	router := gin.New()
	router.Use(Recovery())
	router.GET("/sub/:token", func(c *gin.Context) { panic("recovery test marker") })
	router.GET("/healthy", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest("GET", "/sub/secret-token?key=secret-query", nil)
	request.Header.Set("Authorization", "secret-authorization")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 500 {
		t.Fatalf("panic status = %d", response.Code)
	}
	logs := strings.Join(logger.GetFileLogs(100, "error"), "\n")
	for _, want := range []string{"recovery test marker", "GET /sub/:token", "recovery_test.go"} {
		if !strings.Contains(logs, want) {
			t.Errorf("missing %q in %s", want, logs)
		}
	}
	for _, secret := range []string{"secret-token", "secret-query", "secret-authorization"} {
		if strings.Contains(logs, secret) {
			t.Errorf("request secret leaked: %s", secret)
		}
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/healthy", nil))
	if response.Code != 204 {
		t.Fatalf("next request failed: %d", response.Code)
	}
}
