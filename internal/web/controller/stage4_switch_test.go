//go:build linux

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/gin-gonic/gin"
)

func TestStage4NativeCoreSwitchAPI(t *testing.T) {
	root := os.Getenv("XUI_STAGE4_CORE_DIR")
	if root == "" {
		t.Skip("requires verified native cores; mandatory in runtime-audit")
	}
	t.Setenv("XUI_DB_TYPE", "sqlite")
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "panel.db"))
	original, had := os.LookupEnv("XUI_BIN_FOLDER")
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	service.InitializeNativeCoreRuntime()
	t.Cleanup(func() {
		for _, kind := range []string{service.CoreTypeSingBox, service.CoreTypeSingBoxUpstream, service.CoreTypeXray} {
			_ = service.NativeCore(kind).Stop(context.Background())
		}
		if had {
			_ = os.Setenv("XUI_BIN_FOLDER", original)
		} else {
			_ = os.Unsetenv("XUI_BIN_FOLDER")
		}
		service.InitializeNativeCoreRuntime()
	})
	copyFile := func(src, dst string) {
		t.Helper()
		in, err := os.Open(src)
		if err != nil {
			t.Fatal(err)
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(out, in)
		closeErr := out.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
	}
	copyFile(filepath.Join(root, "owner/x-ui/bin/hiddify-core-linux-"+runtime.GOARCH), service.NativeCore(service.CoreTypeSingBox).BinaryPath())
	copyFile(filepath.Join(root, "upstream/sing-box"), service.NativeCore(service.CoreTypeSingBoxUpstream).BinaryPath())
	xray := service.NativeCore(service.CoreTypeXray)
	copyFile(filepath.Join(root, "xray/xray"), xray.BinaryPath())
	for _, name := range []string{"geoip.dat", "geosite.dat"} {
		copyFile(filepath.Join(root, "xray", name), filepath.Join(filepath.Dir(xray.BinaryPath()), name))
	}
	settings := &service.SettingService{}
	if err := settings.SetCoreType(service.CoreTypeSingBox); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetXrayConfigTemplate(`{"log":{"loglevel":"warning"},"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"direct","settings":{}}],"routing":{"rules":[]}}`); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetSingBoxConfigTemplate(`{"inbounds":[],"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"direct","rules":[]}}`); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	if err := database.GetDB().Create(&model.Inbound{Enable: true, Listen: "127.0.0.1", Port: port, Protocol: model.HTTP, Tag: "api-switch", Settings: `{"clients":[],"accounts":[]}`, StreamSettings: `{"network":"tcp","security":"none"}`}).Error; err != nil {
		t.Fatal(err)
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "stage4-switch") }))
	defer origin.Close()
	proxyURL, _ := url.Parse("http://" + address)
	check := func(kind string) {
		t.Helper()
		got, err := settings.GetCoreType()
		if err != nil || got != kind {
			t.Fatal("selection differs from running core", got, err)
		}
		for _, other := range []string{service.CoreTypeSingBox, service.CoreTypeSingBoxUpstream, service.CoreTypeXray} {
			if service.NativeCore(other).IsRunning() != (other == kind) {
				t.Fatalf("running core mismatch: %s", other)
			}
		}
		transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		response, err := client.Get(origin.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil || string(data) != "stage4-switch" {
			t.Fatal("core switch lost traffic", err)
		}
	}
	call := func(kind string, want bool) {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/panel/api/setting/cores/"+kind+"/select", nil)
		c.Params = gin.Params{{Key: "core", Value: kind}}
		(&SettingController{}).selectStandaloneCore(c)
		var body struct {
			Success bool   `json:"success"`
			Msg     string `json:"msg"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Success != want {
			t.Fatalf("select %s: %s, %v", kind, w.Body.String(), err)
		}
	}
	if err := service.NativeCore(service.CoreTypeSingBox).Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	check(service.CoreTypeSingBox)
	for _, kind := range []string{service.CoreTypeSingBoxUpstream, service.CoreTypeXray, service.CoreTypeSingBox} {
		call(kind, true)
		check(kind)
	}
	// Invalid identity must fail before the old listener is stopped.
	saved := xray.BinaryPath() + ".saved"
	if err := os.Rename(xray.BinaryPath(), saved); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(saved, xray.BinaryPath())
	if err := os.WriteFile(xray.BinaryPath(), []byte("invalid executable"), 0700); err != nil {
		t.Fatal(err)
	}
	call(service.CoreTypeXray, false)
	check(service.CoreTypeSingBox)
	// Delegate genuine checks to Xray, then inject failure only at runtime start.
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = version ] || [ \"$2\" = -test ]; then exec %q \"$@\"; fi\necho stage4-injected-startup-failure >&2\nexit 7\n", saved)
	if err := os.WriteFile(xray.BinaryPath(), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	call(service.CoreTypeXray, false)
	check(service.CoreTypeSingBox)
}
