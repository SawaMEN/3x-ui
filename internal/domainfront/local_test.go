package domainfront

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalSubscriptionPreservesHTTPBehavior(t *testing.T) {
	for _, tc := range []struct {
		name        string
		handle      http.HandlerFunc
		status      int
		contentType string
	}{
		{"implicit body", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<html><body>subscription</body></html>"))
		}, http.StatusOK, "text/html; charset=utf-8"},
		{"empty response", func(w http.ResponseWriter, _ *http.Request) {}, http.StatusOK, ""},
		{"empty cache override", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "public")
		}, http.StatusOK, ""},
		{"explicit status", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Header().Set("Cache-Control", "public")
			_, _ = w.Write([]byte("denied"))
		}, http.StatusUnauthorized, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			localHandler(tc.handle, "", true).ServeHTTP(recorder, httptest.NewRequest("GET", "http://example.com/sub/token", nil))
			response := recorder.Result()
			defer response.Body.Close()
			if response.StatusCode != tc.status || response.Header.Get("Content-Type") != tc.contentType {
				t.Fatalf("status=%d headers=%v", response.StatusCode, response.Header)
			}
			for _, key := range []string{"Cache-Control", "CDN-Cache-Control", "Cloudflare-CDN-Cache-Control"} {
				if response.Header.Get(key) == "" || response.Header.Get(key) == "public" {
					t.Fatalf("missing cache protection: %v", response.Header)
				}
			}
		})
	}
}

type statusWriter struct {
	header   http.Header
	statuses []int
}

func (w *statusWriter) Header() http.Header         { return w.header }
func (w *statusWriter) WriteHeader(status int)      { w.statuses = append(w.statuses, status) }
func (w *statusWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestPrivateResponseDoesNotRepeatFinalStatus(t *testing.T) {
	underlying := &statusWriter{header: make(http.Header)}
	w := &privateResponse{ResponseWriter: underlying}
	w.WriteHeader(http.StatusEarlyHints)
	w.WriteHeader(http.StatusForbidden)
	w.Header().Set("Cache-Control", "public")
	_, _ = w.Write([]byte("denied"))
	w.WriteHeader(http.StatusOK)
	if len(underlying.statuses) != 2 || underlying.statuses[0] != 103 || underlying.statuses[1] != 403 {
		t.Fatalf("unexpected WriteHeader calls: %v", underlying.statuses)
	}
}
