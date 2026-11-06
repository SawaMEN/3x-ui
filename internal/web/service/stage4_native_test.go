//go:build linux

package service

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func stage4Copy(t *testing.T, src, dst string, mode os.FileMode) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("copy: %v %v", err, closeErr)
	}
}

func stage4ProxyEcho(t *testing.T, address, target string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.WriteString(conn, "CONNECT "+target+" HTTP/1.1\r\nHost: "+target+"\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("HTTP CONNECT rejected: %s", response.Status)
	}
	payload := []byte("stage4-native-roundtrip")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(reader, got); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("native echo: %q %v", got, err)
	}
}

func TestStage4NativeEngineLifecycle(t *testing.T) {
	root := os.Getenv("XUI_STAGE4_CORE_DIR")
	if root == "" {
		t.Skip("requires checksum-verified native binaries; mandatory in runtime-audit workflow")
	}
	for _, engine := range []string{CoreTypeSingBox, CoreTypeSingBoxUpstream, CoreTypeXray} {
		t.Run(engine, func(t *testing.T) {
			setupBulkDB(t)
			oldRuntime, oldOwner, oldUpstream, oldXray := hiddifyRuntime, hiddifyProcess, upstreamProcess, standaloneXrayProcess
			t.Setenv("XUI_BIN_FOLDER", t.TempDir())
			InitializeNativeCoreRuntime()
			svc := NativeCore(engine)
			t.Cleanup(func() {
				_ = svc.process().Stop()
				hiddifyRuntime, hiddifyProcess, upstreamProcess, standaloneXrayProcess = oldRuntime, oldOwner, oldUpstream, oldXray
			})
			source := filepath.Join(root, "owner", "x-ui", "bin", "hiddify-core-linux-"+runtime.GOARCH)
			if engine == CoreTypeXray {
				source = filepath.Join(root, "xray", "xray")
			}
			if engine == CoreTypeSingBoxUpstream {
				source = filepath.Join(root, "upstream", "sing-box")
			}
			stage4Copy(t, source, svc.BinaryPath(), 0700)
			if engine == CoreTypeXray {
				for _, name := range []string{"geoip.dat", "geosite.dat"} {
					stage4Copy(t, filepath.Join(filepath.Dir(source), name), filepath.Join(filepath.Dir(svc.BinaryPath()), name), 0600)
				}
			}
			settings := &SettingService{}
			if err := settings.SetCoreType(engine); err != nil {
				t.Fatal(err)
			}
			if got, err := settings.GetCoreType(); err != nil || got != engine {
				t.Fatal("core selection not persisted", got, err)
			}
			var databases []struct{ File string }
			if err := database.GetDB().Raw("PRAGMA database_list").Scan(&databases).Error; err != nil || len(databases) == 0 {
				t.Fatal("find fixture database", err)
			}
			if err := database.CloseDB(); err != nil {
				t.Fatal(err)
			}
			if err := database.InitDB(databases[0].File); err != nil {
				t.Fatal(err)
			}
			if got, err := settings.GetCoreType(); err != nil || got != engine || SelectedNativeCore().CoreType() != engine {
				t.Fatal("core selection lost after database reopen", got, err)
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
			inbound := &model.Inbound{Enable: true, Listen: "127.0.0.1", Port: port, Protocol: model.HTTP, Tag: "stage4-native", Settings: `{"auth":"noauth","udp":false,"accounts":[],"clients":[]}`, StreamSettings: `{"network":"tcp","security":"none"}`}
			if err := database.GetDB().Create(inbound).Error; err != nil {
				t.Fatal(err)
			}
			echo, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer echo.Close()
			go func() {
				for {
					c, err := echo.Accept()
					if err != nil {
						return
					}
					go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
				}
			}()
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			if err := svc.Start(ctx); err != nil {
				t.Fatal(err)
			}
			stage4ProxyEcho(t, address, echo.Addr().String())
			identity, err := svc.process().RuntimeID()
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.Start(ctx); err != nil {
				t.Fatal(err)
			}
			again, err := svc.process().RuntimeID()
			if err != nil || again != identity {
				t.Fatal("duplicate core instance on Start", again, identity, err)
			}
			applied, err := os.ReadFile(svc.ProcessConfigPath())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(svc.ProcessConfigPath(), []byte("{invalid"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := svc.process().Restart(ctx); err == nil {
				t.Fatal("invalid native configuration accepted")
			}
			if !svc.IsRunning() {
				t.Fatal("invalid check stopped the working core")
			}
			if err := writeNativeConfigBytes(svc.ProcessConfigPath(), applied); err != nil {
				t.Fatal(err)
			}
			busy, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer busy.Close()
			if err := database.GetDB().Model(inbound).Update("port", busy.Addr().(*net.TCPAddr).Port).Error; err != nil {
				t.Fatal(err)
			}
			if err := svc.Restart(ctx); err == nil {
				t.Fatal("occupied port reported success")
			}
			if !svc.IsRunning() {
				t.Fatal("startup failure did not restore old core")
			}
			restored, err := os.ReadFile(svc.ProcessConfigPath())
			if err != nil || !bytes.Equal(restored, applied) {
				t.Fatal("startup failure left candidate on disk", err)
			}
			stage4ProxyEcho(t, address, echo.Addr().String())
			if err := database.GetDB().Model(inbound).Update("port", port).Error; err != nil {
				t.Fatal(err)
			}
			if err := svc.Restart(ctx); err != nil {
				t.Fatal(err)
			}
			stage4ProxyEcho(t, address, echo.Addr().String())
			id, err := svc.process().RuntimeID()
			if err != nil {
				t.Fatal(err)
			}
			pid, err := strconv.Atoi(strings.Split(id, ":")[0])
			if err != nil {
				t.Fatal(err)
			}
			child, err := os.FindProcess(pid)
			if err != nil {
				t.Fatal(err)
			}
			if err := child.Kill(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for svc.IsRunning() && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if svc.IsRunning() || svc.LastError() == nil {
				t.Fatal("child crash not observed")
			}
			if err := svc.Start(ctx); err != nil {
				t.Fatal(err)
			}
			stage4ProxyEcho(t, address, echo.Addr().String())
			var adopted *singbox.Process
			if engine == CoreTypeSingBox {
				adopted = hiddify.NewProcess(svc.ProcessConfigPath(), false)
			} else {
				adopted = standaloneProcess(engine, svc.ProcessConfigPath(), false)
			}
			if !adopted.IsRunning() {
				t.Fatal("boot recovery did not adopt owned native core")
			}
			oldID, err := svc.process().RuntimeID()
			if err != nil {
				t.Fatal(err)
			}
			adoptedID, err := adopted.RuntimeID()
			if err != nil || adoptedID != oldID {
				t.Fatal("adoption changed runtime identity")
			}
			if err := adopted.Start(ctx); err != nil {
				t.Fatal(err)
			}
			if err := adopted.Stop(); err != nil {
				t.Fatal(err)
			}
			free, err := net.Listen("tcp", address)
			if err != nil {
				t.Fatal("stop left listener open", err)
			}
			_ = free.Close()
			if svc.IsRunning() {
				t.Fatal("stop left duplicate core")
			}
			t.Logf("%s: DB generation, native checks, HTTP CONNECT echo, idempotent start, invalid config, occupied-port rollback, restart, crash recovery, adoption and listener cleanup passed", engine)
		})
	}
}
