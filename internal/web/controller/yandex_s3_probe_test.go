package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestYandexS3BucketNames(t *testing.T) {
	for _, name := range []string{"xdrive-bucket", "my.bucket.2026", "abc"} {
		if !validYandexS3Bucket(name) {
			t.Errorf("valid bucket name %q rejected", name)
		}
	}
	for _, name := range []string{"", "ab", "-abc", "abc-", "a..b", "UPPERCASE", "my/bucket", "192.168.1.1"} {
		if validYandexS3Bucket(name) {
			t.Errorf("unsafe bucket name %q accepted", name)
		}
	}
}

func TestYandexS3SignatureMatchesFixedRegionAndPayload(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 34, 56, 0, time.UTC)
	body := []byte("signed-yandex-object")
	request, err := http.NewRequest(http.MethodPut,
		"https://storage.yandexcloud.net/bucket/xdrive/test", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	signYandexS3Probe(request, "example-key-id", "example-secret", body, now)
	auth := request.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=example-key-id/20261118/ru-central1/s3/aws4_request, ") {
		t.Fatalf("S3 signing scope is wrong: %s", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=host;x-amz-content-sha256;x-amz-date") {
		t.Fatalf("S3 signed headers are wrong: %s", auth)
	}
	if request.Header.Get("X-Amz-Date") != "20261118T123456Z" {
		t.Fatal("S3 signing date was not set")
	}
	if len(request.Header.Get("X-Amz-Content-Sha256")) != 64 {
		t.Fatal("S3 payload hash missing")
	}

	other, _ := http.NewRequest(http.MethodPut,
		"https://storage.yandexcloud.net/bucket/xdrive/test", bytes.NewReader([]byte("different")))
	signYandexS3Probe(other, "example-key-id", "example-secret", []byte("different"), now)
	if other.Header.Get("Authorization") == auth {
		t.Fatal("payload change did not change signature")
	}
}

func TestYandexS3ProbeRejectsUnsafeInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/probe", handleYandexS3Probe)
	for _, tc := range []struct {
		name, bucket, folder, accessID, secret string
	}{
		{"missing bucket", "", "xdrive", "key", "secret"},
		{"unsafe bucket", "../private", "xdrive", "key", "secret"},
		{"unsafe prefix", "valid-bucket", "../private", "key", "secret"},
		{"missing key", "valid-bucket", "xdrive", "", "secret"},
		{"missing secret", "valid-bucket", "xdrive", "key", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form := url.Values{
				"bucket": {tc.bucket}, "folder": {tc.folder},
				"accessKey": {tc.accessID}, "secretKey": {tc.secret},
			}
			req := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if !strings.Contains(resp.Body.String(), `"success":false`) {
				t.Fatalf("invalid settings accepted: %s", resp.Body.String())
			}
			if strings.Contains(resp.Body.String(), "example-sensitive-secret") {
				t.Fatal("secret disclosed in HTTP response")
			}
		})
	}
}
