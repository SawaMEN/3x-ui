package mtproto

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScrapeStatsCheckedRejectsFailedEnvelopes(t *testing.T) {
	for _, body := range []string{`{"ok":false,"data":[{"username":"a","total_octets":99}]}`, `{"ok":true}`, `{"ok":true,"data":[{"username":"a","total_octets":-1}]}`, `{"ok":true,"data":[]} trailing`, strings.Repeat(" ", (1<<20)+1)} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer srv.Close()
			if _, err := scrapeStatsChecked(serverPort(t, srv), "private-token"); err == nil || strings.Contains(err.Error(), "private-token") {
				t.Fatalf("invalid/revealing result: %v", err)
			}
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"ok":true,"data":[]}`)) }))
	defer srv.Close()
	if _, err := scrapeStatsChecked(serverPort(t, srv), ""); err != nil {
		t.Fatalf("valid empty snapshot rejected: %v", err)
	}
}
