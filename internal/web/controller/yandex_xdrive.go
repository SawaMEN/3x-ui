package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const yandexXDriveWebDAV = "https://webdav.yandex.ru"

// Only a single, unescaped folder component is permitted: the probe must never
// accept an arbitrary URL or a path that could access the user's other files.
var yandexXDriveFolderPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type yandexXDriveProbeResult struct {
	Folder        string `json:"folder"`
	FolderCreated bool   `json:"folderCreated"`
	Written       bool   `json:"written"`
	Read          bool   `json:"read"`
	Listed        bool   `json:"listed"`
	Deleted       bool   `json:"deleted"`
}

func yandexXDriveRequest(ctx context.Context, client *http.Client, login, password, method, target string, payload []byte, depth string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("cannot prepare WebDAV request: %w", err)
	}
	req.SetBasicAuth(login, password)
	if depth != "" {
		req.Header.Set("Depth", depth)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("cannot contact Yandex WebDAV: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("cannot read WebDAV response: %w", err)
	}
	return resp.StatusCode, data, nil
}

// Probe Yandex Disk without touching user files. This is a server-side check,
// not evidence that the same API is available from a mobile operator's network.
func handleYandexXDriveProbe(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	login := strings.TrimSpace(c.PostForm("login"))
	password := c.PostForm("appPassword")
	folder := strings.TrimSpace(c.PostForm("folder"))
	if login == "" || password == "" {
		jsonObj(c, nil, fmt.Errorf("enter your Yandex login and WebDAV application password"))
		return
	}
	if !yandexXDriveFolderPattern.MatchString(folder) {
		jsonObj(c, nil, fmt.Errorf("folder must be one name of 1–64 Latin letters, digits, '_' or '-'"))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	client := &http.Client{
		Timeout: 10*time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	base := yandexXDriveWebDAV + "/" + folder
	call := func(method, url string, payload []byte, depth string) (int, []byte, error) {
		return yandexXDriveRequest(ctx, client, login, password, method, url, payload, depth)
	}
	failStatus := func(op string, status int) error {
		switch status {
		case http.StatusUnauthorized:
			return fmt.Errorf("%s: WebDAV login or application password was rejected (401)", op)
		case http.StatusForbidden:
			return fmt.Errorf("%s: WebDAV access denied (403); check Yandex 360 and application permissions", op)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%s: Yandex Disk request limit reached (429); retry later", op)
		default:
			return fmt.Errorf("%s: Yandex WebDAV returned HTTP %d", op, status)
		}
	}
	result := yandexXDriveProbeResult{Folder: folder}
	status, _, err := call("PROPFIND", base+"/", nil, "0")
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status == http.StatusNotFound {
		status, _, err = call("MKCOL", base, nil, "")
		if err != nil {
			jsonObj(c, nil, err)
			return
		}
		if status != http.StatusCreated && status != http.StatusMethodNotAllowed {
			jsonObj(c, nil, failStatus("Create folder", status))
			return
		}
		result.FolderCreated = status == http.StatusCreated
	} else if status != http.StatusMultiStatus && status != http.StatusOK {
		jsonObj(c, nil, failStatus("Check folder", status))
		return
	}

	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		jsonObj(c, nil, fmt.Errorf("cannot generate probe filename"))
		return
	}
	name := ".xdrive-probe-" + hex.EncodeToString(id)
	target := base + "/" + name
	payload := []byte("xdrive-yandex-webdav-probe-" + hex.EncodeToString(id))

	status, _, err = call(http.MethodPut, target, payload, "")
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusCreated && status != http.StatusNoContent && status != http.StatusOK {
		jsonObj(c, nil, failStatus("Write test file", status))
		return
	}
	result.Written = true
	// Always attempt cleanup, even when a later read/list operation fails.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _, _ = yandexXDriveRequest(cleanupCtx, client, login, password, http.MethodDelete, target, nil, "")
	}()

	status, got, err := call(http.MethodGet, target, nil, "")
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusOK {
		jsonObj(c, nil, failStatus("Read test file", status))
		return
	}
	if !bytes.Equal(got, payload) {
		jsonObj(c, nil, fmt.Errorf("WebDAV returned different content than was uploaded"))
		return
	}
	result.Read = true

	status, listing, err := call("PROPFIND", base+"/", nil, "1")
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusMultiStatus || !bytes.Contains(listing, []byte(name)) {
		if status != http.StatusMultiStatus {
			jsonObj(c, nil, failStatus("List test files", status))
		} else {
			jsonObj(c, nil, fmt.Errorf("WebDAV listing did not contain the test file"))
		}
		return
	}
	result.Listed = true

	status, _, err = call(http.MethodDelete, target, nil, "")
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	if status != http.StatusOK && status != http.StatusNoContent && status != http.StatusAccepted {
		jsonObj(c, nil, failStatus("Delete test file", status))
		return
	}
	result.Deleted = true
	jsonObj(c, result, nil)
}
