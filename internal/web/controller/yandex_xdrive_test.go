package controller

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestYandexXDriveRequestUsesBasicAuthAndDepth(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "test-user" || password != "app-password" {
			t.Errorf("BasicAuth was not transmitted correctly")
		}
		if r.Method != "PROPFIND" || r.Header.Get("Depth") != "1" {
			t.Errorf("unexpected WebDAV request: %s Depth=%s", r.Method, r.Header.Get("Depth"))
		}
		data, _ := io.ReadAll(r.Body)
		if !bytes.Equal(data, []byte("test")) {
			t.Errorf("request body changed")
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte("<multistatus/>"))
	}))
	defer server.Close()

	status, body, err := yandexXDriveRequest(context.Background(), server.Client(), "test-user", "app-password",
		"PROPFIND", server.URL+"/xdrive/", []byte("test"), "1")
	if err != nil || status != http.StatusMultiStatus || string(body) != "<multistatus/>" {
		t.Fatalf("WebDAV request failed: status=%d body=%q err=%v", status, body, err)
	}
}

func TestYandexXDriveProbeRejectsUnsafeInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name     string
		login    string
		password string
		folder   string
	}{
		{"missing-login", "", "private-webdav-secret-123", "xdrive"},
		{"missing-password", "user", "", "xdrive"},
		{"path-traversal", "user", "private-webdav-secret-123", "../private"},
		{"nested-folder", "user", "private-webdav-secret-123", "xdrive/private"},
		{"invalid-encoding", "user", "private-webdav-secret-123", "%2Fprivate"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/probe", handleYandexXDriveProbe)
			form := url.Values{
				"login":       {test.login},
				"appPassword": {test.password},
				"folder":      {test.folder},
			}
			request := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if !strings.Contains(response.Body.String(), `"success":false`) {
				t.Fatalf("unsafe input unexpectedly accepted: %s", response.Body.String())
			}
			if strings.Contains(response.Body.String(), test.password) && test.password != "" {
				t.Fatal("WebDAV credentials leaked into response")
			}
		})
	}
}
