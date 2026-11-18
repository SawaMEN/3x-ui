package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSafeYandexDiskHref(t *testing.T) {
	valid := []string{
		"https://uploader22.disk.yandex.net/upload-target/abc",
		"https://downloader.disk.yandex.ru:443/download",
	}
	invalid := []string{
		"http://uploader.disk.yandex.net/segment",
		"https://disk.yandex.net/segment",
		"https://uploader.disk.yandex.net.evil.example/upload",
		"https://127.0.0.1/admin",
		"https://user:secret@uploader.disk.yandex.net/segment",
		"https://uploader.disk.yandex.net:9999/segment",
		"https://uploader.disk.yandex.net/segment#fragment",
		"not a URL",
	}
	for _, value := range valid {
		if _, err := safeYandexDiskHref(value); err != nil {
			t.Errorf("valid presigned URL rejected: %q: %v", value, err)
		}
	}
	for _, value := range invalid {
		if _, err := safeYandexDiskHref(value); err == nil {
			t.Errorf("unsafe presigned URL accepted: %q", value)
		}
	}
}

func TestYandexDiskAPIRequestScopesAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/authorized" && r.Header.Get("Authorization") != "OAuth mytoken" {
			t.Errorf("API OAuth token missing")
		}
		if r.URL.Path == "/presigned" && r.Header.Get("Authorization") != "" {
			t.Errorf("OAuth token forwarded to a temporary file URL")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	for _, test := range []struct{ path, token string }{
		{"/authorized", "mytoken"}, {"/presigned", ""},
	} {
		status, body, err := yandexDiskAPIRequest(context.Background(), server.Client(),
			test.token, http.MethodGet, server.URL+test.path, nil)
		if status != http.StatusOK || err != nil || string(body) != "ok" {
			t.Fatalf("request failed: %d %q %v", status, body, err)
		}
	}
}

func TestYandexDiskAPIProbeRejectsUntrustedFolders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/probe", handleYandexDiskAPIProbe)
	for _, folder := range []string{"../private", "foo/bar", "%2F", ""} {
		form := url.Values{"token": {"sensitive-token"}, "folder": {folder}}
		req := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if !strings.Contains(resp.Body.String(), `"success":false`) {
			t.Errorf("unsafe folder %q accepted: %s", folder, resp.Body.String())
		}
		if strings.Contains(resp.Body.String(), "sensitive-token") {
			t.Errorf("OAuth token disclosed in response")
		}
	}
}
