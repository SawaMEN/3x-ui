package middleware

import (
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCoreManagementDeadlineBeforeGzip(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{"POST", "/secret/panel/api/setting/cores/xray/install", true},
		{"POST", "/secret/panel/api/setting/cores/sing-box-upstream/select", true},
		{"GET", "/secret/panel/api/setting/cores/xray/install", false},
		{"POST", "/secret/panel/api/setting/update", false},
		{"POST", "/panel/api/setting/cores/xray/install", false},
		{"POST", "/secret/panel/api/setting/cores/unknown/install", false},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := gin.New()
			r.Use(CoreManagementDeadline("/secret/panel/api/setting/cores/"))
			r.Use(gzip.Gzip(gzip.DefaultCompression))
			r.Any(tc.path, func(c *gin.Context) { c.String(http.StatusOK, "done") })
			w := &scanDeadlineWriter{ResponseRecorder: httptest.NewRecorder()}
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Accept-Encoding", "gzip")
			before := time.Now()
			r.ServeHTTP(w, req)
			if tc.want {
				if w.deadline.Before(before.Add(6 * time.Minute)) {
					t.Fatal("core response deadline not extended through gzip")
				}
			} else if !w.deadline.IsZero() {
				t.Fatal("unrelated request deadline changed")
			}
		})
	}
}
