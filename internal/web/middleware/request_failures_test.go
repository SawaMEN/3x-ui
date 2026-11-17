package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

func TestRequestFailuresRecordsProductionDenialsWithoutRequestSecrets(t *testing.T) {
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	logger.InitLogger(logging.ERROR)
	t.Cleanup(logger.CloseLogger)
	router := gin.New()
	router.Use(RequestFailures(), Recovery())
	for _, code := range []int{401, 403, 429, 500} {
		router.GET("/PRIVATE_BASE/panel/api/failure/"+http.StatusText(code)+"/:token", func(c *gin.Context) { c.AbortWithStatus(code) })
	}
	// Use a path with a real dynamic token, a private base path, and headers/body.
	router.POST("/PRIVATE_BASE/panel/api/PRIVATE_BASE2/panel/api/nodes/:token", func(c *gin.Context) { c.AbortWithStatus(403) })
	router.GET("/healthy", func(c *gin.Context) { c.Status(204) })
	router.GET("/gin-error", func(c *gin.Context) { _ = c.Error(errors.New("disk failed password=secret-pass")) })
	req := httptest.NewRequest(http.MethodPost, "/PRIVATE_BASE/panel/api/PRIVATE_BASE2/panel/api/nodes/PRIVATE_ID?token=PRIVATE_QUERY", strings.NewReader("PRIVATE_BODY"))
	req.Header.Set("Authorization", "Bearer PRIVATE_HEADER")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != 403 {
		t.Fatalf("status=%d", response.Code)
	}
	for _, code := range []int{401, 403, 429, 500} {
		path := "/PRIVATE_BASE/panel/api/failure/" + strings.ReplaceAll(http.StatusText(code), " ", "%20") + "/PRIVATE_ID"
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/PRIVATE_UNKNOWN", nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthy", nil))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/gin-error", nil))
	logs := strings.Join(logger.GetFileLogs(100, "error"), "\n")
	for _, want := range []string{"route=/panel/api/nodes/:token", "status=401", "status=403", "status=429", "status=500", "status=404", "disk failed"} {
		if !strings.Contains(logs, want) {
			t.Errorf("missing %q in %s", want, logs)
		}
	}
	for _, secret := range []string{"PRIVATE_BASE", "PRIVATE_ID", "PRIVATE_QUERY", "PRIVATE_BODY", "PRIVATE_HEADER", "PRIVATE_UNKNOWN", "secret-pass", "/healthy"} {
		if strings.Contains(logs, secret) {
			t.Errorf("secret or success logged: %s", secret)
		}
	}
}
