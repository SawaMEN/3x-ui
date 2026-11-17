package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const yandexDiskAPIBase = "https://cloud-api.yandex.net/v1/disk"

type yandexDiskAPIHref struct {
	Href string `json:"href"`
}

func safeYandexDiskHref(href string) (*url.URL, error) {
	u, err := url.Parse(href)
	if err != nil || u == nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("Yandex Disk returned an invalid temporary URL")
	}
	host := strings.ToLower(u.Hostname())
	if !(strings.HasSuffix(host, ".disk.yandex.net") ||
		strings.HasSuffix(host, ".disk.yandex.ru")) ||
		(u.Port() != "" && u.Port() != "443") {
		return nil, fmt.Errorf("Yandex Disk temporary URL uses an unexpected host")
	}
	return u, nil
}

func yandexDiskAPIRequest(ctx context.Context, client *http.Client, token, method, target string, payload []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("cannot construct Yandex Disk API request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "OAuth "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("Yandex Disk API request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2 << 20))
	if err != nil {
		return 0, nil, fmt.Errorf("cannot read Yandex Disk API response: %w", err)
	}
	return resp.StatusCode, data, nil
}

func handleYandexDiskAPIProbe(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	token := strings.TrimSpace(c.PostForm("token"))
	folder := strings.TrimSpace(c.PostForm("folder"))
	if token == "" {
		jsonObj(c, nil, fmt.Errorf("enter a Yandex Disk OAuth token"))
		return
	}
	if !yandexXDriveFolderPattern.MatchString(folder) {
		jsonObj(c, nil, fmt.Errorf("folder must contain 1–64 Latin letters, digits, '_' or '-'"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 35 * time.Second)
	defer cancel()
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	api := func(method, endpoint string, params url.Values, payload []byte) (int, []byte, error) {
		address := yandexDiskAPIBase + endpoint
		if len(params) > 0 {
			address += "?" + params.Encode()
		}
		return yandexDiskAPIRequest(ctx, client, token, method, address, payload)
	}
	checkStatus := func(op string, status int) error {
		if status == http.StatusUnauthorized {
			return fmt.Errorf("%s: Yandex Disk OAuth token rejected (401)", op)
		}
		if status == http.StatusForbidden {
			return fmt.Errorf("%s: Yandex Disk OAuth token lacks permission (403)", op)
		}
		if status == http.StatusTooManyRequests {
			return fmt.Errorf("%s: Yandex Disk API request limit (429)", op)
		}
		return fmt.Errorf("%s: Yandex Disk API returned HTTP %d", op, status)
	}
	dir := "disk:/" + folder
	folderArgs := url.Values{"path": {dir}}
	result := yandexXDriveProbeResult{Folder: folder}

	status, _, err := api(http.MethodGet, "/resources", folderArgs, nil)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status == http.StatusNotFound {
		status, _, err = api(http.MethodPut, "/resources", folderArgs, nil)
		if err != nil {
			jsonObj(c, nil, err)
			return
		}
		if status != http.StatusCreated && status != http.StatusConflict {
			jsonObj(c, nil, checkStatus("Create folder", status))
			return
		}
		result.FolderCreated = status == http.StatusCreated
	} else if status != http.StatusOK {
		jsonObj(c, nil, checkStatus("Check folder", status))
		return
	}

	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		jsonObj(c, nil, fmt.Errorf("cannot create probe filename"))
		return
	}
	name := ".xdrive-probe-" + hex.EncodeToString(id)
	path := dir + "/" + name
	fileArgs := url.Values{"path": {path}}
	payload := []byte("xdrive-yandex-api-probe-" + hex.EncodeToString(id))
	presigned := func(endpoint string, args url.Values) (string, error) {
		status, raw, err := api(http.MethodGet, endpoint, args, nil)
		if err != nil {
			return "", err
		}
		if status != http.StatusOK {
			return "", checkStatus("Get temporary file URL", status)
		}
		var value yandexDiskAPIHref
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", fmt.Errorf("Yandex Disk API returned an invalid temporary URL response")
		}
		u, err := safeYandexDiskHref(value.Href)
		if err != nil {
			return "", err
		}
		return u.String(), nil
	}
	uploadArgs := url.Values{"path": {path}, "overwrite": {"true"}}
	uploadURL, err := presigned("/resources/upload", uploadArgs)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	status, _, err = yandexDiskAPIRequest(ctx, client, "", http.MethodPut, uploadURL, payload)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status < 200 || status >= 300 {
		jsonObj(c, nil, checkStatus("Upload test file", status))
		return
	}
	result.Written = true
	// After an upload, deletion is best-effort even if a later validation fails.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5 * time.Second)
		defer cleanupCancel()
		target := yandexDiskAPIBase + "/resources?" + url.Values{
			"path": {path}, "permanently": {"true"},
		}.Encode()
		_, _, _ = yandexDiskAPIRequest(cleanupCtx, client, token, http.MethodDelete, target, nil)
	}()

	downloadURL, err := presigned("/resources/download", fileArgs)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	status, got, err := yandexDiskAPIRequest(ctx, client, "", http.MethodGet, downloadURL, nil)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusOK {
		jsonObj(c, nil, checkStatus("Download test file", status))
		return
	}
	if !bytes.Equal(got, payload) {
		jsonObj(c, nil, fmt.Errorf("Yandex Disk returned data different from the test file"))
		return
	}
	result.Read = true

	status, raw, err := api(http.MethodGet, "/resources", url.Values{
		"path": {dir}, "limit": {"1000"}, "fields": {"_embedded.items.name"},
	}, nil)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusOK {
		jsonObj(c, nil, checkStatus("List test files", status))
		return
	}
	var listing struct {
		Embedded struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		} `json:"_embedded"`
	}
	if err := json.Unmarshal(raw, &listing); err != nil {
		jsonObj(c, nil, fmt.Errorf("Yandex Disk folder list was not valid JSON"))
		return
	}
	for _, file := range listing.Embedded.Items {
		if file.Name == name {
			result.Listed = true
			break
		}
	}
	if !result.Listed {
		jsonObj(c, nil, fmt.Errorf("Yandex Disk folder listing omitted the test file"))
		return
	}
	status, _, err = api(http.MethodDelete, "/resources", url.Values{
		"path": {path}, "permanently": {"true"},
	}, nil)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusNoContent && status != http.StatusAccepted && status != http.StatusOK {
		jsonObj(c, nil, checkStatus("Delete test file", status))
		return
	}
	result.Deleted = true
	jsonObj(c, result, nil)
}
