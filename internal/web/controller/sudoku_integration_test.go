package controller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sudoku"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type sudokuHTTPTransport func(*http.Request) (*http.Response, error)

func (f sudokuHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercise the production panel endpoints, downloader, generated credentials,
// reconciliation and rollback against the unmodified official executable.
func TestSudokuPanelInstallUpdateRollbackAndRemove(t *testing.T) {
	binary := os.Getenv("SUDOKU_TEST_BINARY")
	if binary == "" {
		t.Skip("set SUDOKU_TEST_BINARY to an official release executable")
	}
	if runtime.GOOS != "linux" {
		t.Skip("Linux component")
	}
	executable, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", binDir)
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	if err := database.InitDB(filepath.Join(t.TempDir(), "panel.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	sudoku.GetManager().StopAll()
	t.Cleanup(sudoku.GetManager().StopAll)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	inbound := &model.Inbound{Protocol: model.Sudoku, Enable: true, Tag: "official-panel-test", Port: port, Settings: `{"clients":[{"email":"alice@example.com","enable":true}]}`}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	archive := func(payload []byte) []byte {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(&tar.Header{Name: "sudoku", Mode: 0o755, Size: int64(len(payload))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	payload := archive(executable)
	tag := "v0.5.2.0"
	failRelease := false
	const assetURL = "https://github.com/SUDOKU-ASCII/sudoku/releases/download/test/sudoku.tar.gz"
	oldClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldClient })
	http.DefaultClient = &http.Client{Transport: sudokuHTTPTransport(func(req *http.Request) (*http.Response, error) {
		var body []byte
		switch req.URL.String() {
		case "https://api.github.com/repos/SUDOKU-ASCII/sudoku/releases/latest":
			if failRelease {
				return &http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Unavailable", Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
			}
			body, _ = json.Marshal(map[string]any{"tag_name": tag, "assets": []map[string]string{{"name": "sudoku-linux-" + runtime.GOARCH + ".tar.gz", "browser_download_url": assetURL}}})
		case assetURL:
			body = payload
		default:
			t.Fatalf("unexpected HTTP request: %s", req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewSettingController(router.Group("/panel/api"))
	// Register the production removal handler without the other server routes.
	router.POST("/panel/api/server/components/remove/:protocol", (&ServerController{}).removeProtocolComponent)
	request := func(method, path string, want bool) json.RawMessage {
		t.Helper()
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(method, path, nil))
		var data struct {
			Success bool            `json:"success"`
			Obj     json.RawMessage `json:"obj"`
			Msg     string          `json:"msg"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if rr.Code != 200 || data.Success != want {
			t.Fatalf("%s: HTTP %d success %v: %s", path, rr.Code, data.Success, data.Msg)
		}
		return data.Obj
	}
	status := func() sudoku.Status {
		t.Helper()
		var s sudoku.Status
		if err := json.Unmarshal(request("GET", "/panel/api/setting/sudoku/status", true), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	ready := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 100*time.Millisecond)
			if c != nil {
				c.Close()
			}
			if (err == nil) == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("listener running=%v, want %v", err == nil, want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if s := status(); s.Installed || s.Version != "" {
		t.Fatalf("fresh status: %+v", s)
	}
	request("POST", "/panel/api/setting/sudoku/update", true)
	ready(true)
	if s := status(); !s.Installed || s.Version != tag || s.UpdateAvailable {
		t.Fatalf("installed status: %+v", s)
	}
	var saved model.Inbound
	if err := database.GetDB().First(&saved, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	var keys struct {
		Key     string         `json:"key"`
		Clients []model.Client `json:"clients"`
	}
	if err := json.Unmarshal([]byte(saved.Settings), &keys); err != nil {
		t.Fatal(err)
	}
	if !sudoku.ValidPublicKey(keys.Key) || len(keys.Clients) != 1 || !sudoku.ValidPrivateKey(keys.Clients[0].SudokuPrivateKey) {
		t.Fatal("installation did not generate usable client credentials")
	}
	// A failed release lookup must leave an existing connection alive.
	connection, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	failRelease = true
	request("POST", "/panel/api/setting/sudoku/update", false)
	_ = connection.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	var probe [1]byte
	var netErr net.Error
	if _, err := connection.Read(probe[:]); err == nil {
		t.Fatal("unexpected server response")
	} else if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("download failure interrupted the existing connection: %v", err)
	}
	failRelease = false
	// A build with a valid CLI but rejected server config must never become a
	// successful panel update or leave the previous listeners offline.
	payload = archive([]byte("#!/bin/sh\nif [ \"$1\" = -h ]; then echo '-c value -test -keygen'; exit 0; fi\necho 'rejected server configuration'\nexit 1\n"))
	tag = "v0.5.3"
	request("POST", "/panel/api/setting/sudoku/update", false)
	ready(true)
	if s := status(); s.Version != "v0.5.2.0" || !s.UpdateAvailable {
		t.Fatalf("rollback status: %+v", s)
	}
	var after model.Inbound
	if err := database.GetDB().First(&after, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.Settings != saved.Settings {
		t.Fatal("update changed saved keys/settings")
	}
	request("POST", "/panel/api/server/components/remove/sudoku", false)
	ready(true)
	if err := database.GetDB().Model(inbound).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcileSudoku(context.Background()); err != nil {
		t.Fatal(err)
	}
	ready(false)
	request("POST", "/panel/api/server/components/remove/sudoku", true)
	if _, err := os.Stat(filepath.Join(binDir, "sudoku.version")); !os.IsNotExist(err) {
		t.Fatal("removal retained version marker")
	}
	if s := status(); s.Installed || s.Version != "" {
		t.Fatalf("removed status: %+v", s)
	}
	if _, err := os.Stat(filepath.Join(binDir, "sudoku", "sudoku-"+strconv.Itoa(inbound.Id)+".key")); err != nil {
		t.Fatal("component removal lost master key")
	}
	payload = archive(executable)
	tag = "v0.5.2.0"
	request("POST", "/panel/api/setting/sudoku/update", true)
	ready(false)
	if err := database.GetDB().Model(inbound).Update("enable", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcileSudoku(context.Background()); err != nil {
		t.Fatal(err)
	}
	ready(true)
	if err := database.GetDB().First(&after, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after.Settings, keys.Clients[0].SudokuPrivateKey) {
		t.Fatal("reinstallation changed client key")
	}
}
