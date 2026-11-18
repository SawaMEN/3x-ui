package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Fixed endpoint prevents the panel from becoming an arbitrary S3/SSRF proxy.
// This check runs on the panel host, never on the visitor's mobile network.
const (
	yandexS3Endpoint = "https://storage.yandexcloud.net"
	yandexS3Region   = "ru-central1"
)

var yandexS3BucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func validYandexS3Bucket(name string) bool {
	if !yandexS3BucketPattern.MatchString(name) || strings.Contains(name, "..") {
		return false
	}
	return net.ParseIP(name) == nil
}

func yandexS3HMAC(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

// Sign exactly the path-style SigV4 request that the native XDRIVE S3
// backend sends. Keeping the probe's signing small avoids requiring AWS SDKs.
func signYandexS3Probe(req *http.Request, accessID, secret string, body []byte, now time.Time) {
	utc := now.UTC()
	date := utc.Format("20060102")
	stamp := utc.Format("20060102T150405Z")
	hash := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(hash[:])
	req.Header.Set("X-Amz-Date", stamp)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	signed := "host;x-amz-content-sha256;x-amz-date"
	headers := "host:" + req.URL.Host + "\nx-amz-content-sha256:" + payloadHash + "\nx-amz-date:" + stamp + "\n"
	canonical := req.Method + "\n" + req.URL.EscapedPath() + "\n" + req.URL.RawQuery +
		"\n" + headers + "\n" + signed + "\n" + payloadHash
	scope := date + "/" + yandexS3Region + "/s3/aws4_request"
	canonicalHash := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" +
		hex.EncodeToString(canonicalHash[:])
	key := yandexS3HMAC([]byte("AWS4"+secret), date)
	key = yandexS3HMAC(key, yandexS3Region)
	key = yandexS3HMAC(key, "s3")
	key = yandexS3HMAC(key, "aws4_request")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessID+"/"+scope+
		", SignedHeaders="+signed+", Signature="+hex.EncodeToString(yandexS3HMAC(key, toSign)))
}

func yandexS3ProbeRequest(ctx context.Context, client *http.Client, accessID, secret, method, path string, params url.Values, payload []byte) (int, []byte, error) {
	target := yandexS3Endpoint + path
	if len(params) != 0 {
		target += "?" + strings.ReplaceAll(params.Encode(), "+", "%20")
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("cannot construct Object Storage request: %w", err)
	}
	signYandexS3Probe(req, accessID, secret, payload, time.Now())
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("cannot reach Yandex Object Storage: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1 << 20))
	if err != nil {
		return 0, nil, fmt.Errorf("cannot read Object Storage response: %w", err)
	}
	return resp.StatusCode, data, nil
}

func handleYandexS3Probe(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	bucket := strings.TrimSpace(c.PostForm("bucket"))
	folder := strings.TrimSpace(c.PostForm("folder"))
	accessID := strings.TrimSpace(c.PostForm("accessKey"))
	secret := c.PostForm("secretKey")
	if !validYandexS3Bucket(bucket) || !yandexXDriveFolderPattern.MatchString(folder) {
		jsonObj(c, nil, fmt.Errorf("enter a valid bucket name and a folder containing 1-64 Latin letters, digits, _ or -"))
		return
	}
	if accessID == "" || secret == "" || strings.ContainsAny(accessID, "\r\n") || strings.ContainsAny(secret, "\r\n") {
		jsonObj(c, nil, fmt.Errorf("enter the service account static access key ID and secret key"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 35 * time.Second)
	defer cancel()
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	result := yandexXDriveProbeResult{Folder: folder}
	// S3 prefixes are virtual: there is no folder-creation request.
	base := "/" + bucket + "/"
	request := func(method, path string, params url.Values, data []byte) (int, []byte, error) {
		return yandexS3ProbeRequest(ctx, client, accessID, secret, method, path, params, data)
	}
	checkStatus := func(op string, status int) error {
		switch status {
		case http.StatusForbidden:
			return fmt.Errorf("%s: S3 access denied (403); check bucket policy and service-account permissions", op)
		case http.StatusNotFound:
			return fmt.Errorf("%s: S3 bucket or object not found (404)", op)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%s: S3 request limit reached (429)", op)
		default:
			return fmt.Errorf("%s: Object Storage returned HTTP %d", op, status)
		}
	}

	// ListObjectsV2 is required by XDRIVE. Fail early if credentials cannot list.
	status, _, err := request(http.MethodGet, base, url.Values{
		"list-type": {"2"}, "prefix": {folder + "/"}, "max-keys": {"1"},
	}, nil)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusOK {
		jsonObj(c, nil, checkStatus("List bucket", status))
		return
	}

	id := make([]byte, 12)
	if _, err = rand.Read(id); err != nil {
		jsonObj(c, nil, fmt.Errorf("cannot generate test object name"))
		return
	}
	key := folder + "/.xdrive-probe-" + hex.EncodeToString(id)
	target := base + key
	payload := []byte("xdrive-yandex-s3-probe-" + hex.EncodeToString(id))
	status, _, err = request(http.MethodPut, target, nil, payload)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusOK && status != http.StatusCreated {
		jsonObj(c, nil, checkStatus("Write test object", status))
		return
	}
	result.Written = true
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5 * time.Second)
		defer cleanupCancel()
		_, _, _ = yandexS3ProbeRequest(cleanupCtx, client, accessID, secret, http.MethodDelete, target, nil, nil)
	}()
	status, data, err := request(http.MethodGet, target, nil, nil)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusOK {
		jsonObj(c, nil, checkStatus("Read test object", status))
		return
	}
	if !bytes.Equal(data, payload) {
		jsonObj(c, nil, fmt.Errorf("S3 returned data different from the test object"))
		return
	}
	result.Read = true

	status, data, err = request(http.MethodGet, base, url.Values{
		"list-type": {"2"}, "prefix": {key}, "max-keys": {"1"},
	}, nil)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusOK {
		jsonObj(c, nil, checkStatus("List test object", status))
		return
	}
	var list struct {
		Contents []struct{ Key string `xml:"Key"` } `xml:"Contents"`
	}
	if err := xml.Unmarshal(data, &list); err != nil {
		jsonObj(c, nil, fmt.Errorf("S3 object list was not valid XML"))
		return
	}
	for _, item := range list.Contents {
		if item.Key == key {
			result.Listed = true
			break
		}
	}
	if !result.Listed {
		jsonObj(c, nil, fmt.Errorf("S3 listing did not include the test object"))
		return
	}
	status, _, err = request(http.MethodDelete, target, nil, nil)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		jsonObj(c, nil, checkStatus("Delete test object", status))
		return
	}
	result.Deleted = true
	jsonObj(c, result, nil)
}
