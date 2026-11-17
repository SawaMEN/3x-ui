package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

type scanDeadlineWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *scanDeadlineWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestSniScanDeadlineBeforeGzip(t *testing.T) {
	const scanPath = "/secret/panel/api/server/scanRealityTargets"
	for _, tc := range []struct {
		method, path string
		extended     bool
	}{
		{http.MethodPost, scanPath, true},
		{http.MethodGet, scanPath, false},
		{http.MethodPost, "/secret/panel/api/setting/update", false},
		{http.MethodPost, "/panel/api/server/scanRealityTargets", false},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			router := gin.New()
			router.Use(SniScanDeadline(scanPath))
			router.Use(gzip.Gzip(gzip.DefaultCompression))
			router.Any(tc.path, func(c *gin.Context) { c.String(http.StatusOK, "done") })
			writer := &scanDeadlineWriter{ResponseRecorder: httptest.NewRecorder()}
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Accept-Encoding", "gzip")
			before := time.Now()
			router.ServeHTTP(writer, req)
			if writer.Code != http.StatusOK {
				t.Fatalf("status: %d", writer.Code)
			}
			if tc.extended {
				if writer.deadline.Before(before.Add(6*time.Minute)) || writer.deadline.After(time.Now().Add(6*time.Minute)) {
					t.Fatalf("unexpected deadline: %v", writer.deadline)
				}
			} else if !writer.deadline.IsZero() {
				t.Fatalf("unrelated request deadline changed: %v", writer.deadline)
			}
		})
	}
}
