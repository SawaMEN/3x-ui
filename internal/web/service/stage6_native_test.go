package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	webRuntime "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
	"golang.org/x/net/proxy"
)

func TestStage6NativeAccountingAndRevocation(t *testing.T) {
	root := os.Getenv("XUI_STAGE4_CORE_DIR")
	if root == "" {
		t.Skip("requires pinned native core")
	}
	setupBulkDB(t)
	oldRuntime, oldOwner, oldUpstream, oldXray := hiddifyRuntime, hiddifyProcess, upstreamProcess, standaloneXrayProcess
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	InitializeNativeCoreRuntime()
	svc := NativeCore(CoreTypeSingBox)
	t.Cleanup(func() {
		_ = svc.process().Stop()
		hiddifyRuntime, hiddifyProcess, upstreamProcess, standaloneXrayProcess = oldRuntime, oldOwner, oldUpstream, oldXray
	})
	stage4Copy(t, filepath.Join(root, "owner", "x-ui", "bin", "hiddify-core-linux-"+runtime.GOARCH), svc.BinaryPath(), 0700)
	settings := &SettingService{}
	// The production default blocks private destinations. This fixture uses a
	// loopback echo server and must explicitly allow it in the shared policy.
	if err := settings.SetXrayConfigTemplate(`{"log":{"loglevel":"warning"},"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"direct","settings":{}}],"routing":{"rules":[]}}`); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetCoreType(CoreTypeSingBox); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetSingBoxConfigTemplate(`{"inbounds":[],"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"direct","rules":[]}}`); err != nil {
		t.Fatal(err)
	}
	mgr := useTestRuntimeManager(t)
	mgr.SetLocalRuntimeOverride(webRuntime.NewLocal(webRuntime.LocalDeps{CoreType: func() string { return CoreTypeSingBox }, RequestConfigApply: svc.Restart}))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	clients := []model.Client{{ID: "11111111-1111-4111-8111-111111111111", Email: "stage6-alice", Password: "pass", Enable: true, ExpiryTime: -86400000}, {ID: "22222222-2222-4222-8222-222222222222", Email: "stage6-bob", Password: "pass", Enable: true}}
	ib := &model.Inbound{Enable: true, Listen: "127.0.0.1", Port: port, Protocol: model.Mixed, Tag: "stage6-traffic", Settings: clientsSettings(t, clients)}
	db := database.GetDB()
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	inboundSvc := &InboundService{}
	if err := inboundSvc.clientService.SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatal(err)
	}
	for _, c := range clients {
		if err := inboundSvc.AddClientStat(db, ib.Id, &c); err != nil {
			t.Fatal(err)
		}
	}
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, e := echo.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			for _, path := range []string{svc.logPath(), svc.ProcessConfigPath()} {
				data, _ := os.ReadFile(path)
				t.Logf("%s: %s", filepath.Base(path), data)
			}
		}
	})
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	dial := func(user string) (net.Conn, error) {
		d, e := proxy.SOCKS5("tcp", address, &proxy.Auth{User: user, Password: "pass"}, &net.Dialer{Timeout: time.Second})
		if e != nil {
			return nil, e
		}
		return d.(proxy.ContextDialer).DialContext(ctx, "tcp", echo.Addr().String())
	}
	transfer := func(t *testing.T, user string, payload []byte) {
		t.Helper()
		c, e := dial(user)
		if e != nil {
			t.Fatal(e)
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		sent := make(chan error, 1)
		go func() { _, e := c.Write(payload); sent <- e }()
		got := make([]byte, len(payload))
		_, e = io.ReadFull(c, got)
		if e != nil || !bytes.Equal(got, payload) {
			t.Fatal("payload", e)
		}
		if e := <-sent; e != nil {
			t.Fatal(e)
		}
	}
	poll := func(want int64) coremodel.ClientTraffic {
		t.Helper()
		var row coremodel.ClientTraffic
		var last error
		for until := time.Now().Add(10 * time.Second); time.Now().Before(until); {
			last = svc.pollHiddifyTraffic(ctx)
			if last == nil {
				last = database.GetDB().Where("email = ?", "stage6-alice").First(&row).Error
				if last == nil && row.Up >= want && row.Down >= want {
					return row
				}
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatalf("statistics: want >=%d got %+v error=%v", want, row, last)
		return row
	}
	if row := poll(0); row.ExpiryTime >= 0 {
		t.Fatal("zero traffic activated delayed expiry", row)
	}
	payload := bytes.Repeat([]byte("s"), 32*1024)
	for range 3 {
		transfer(t, "stage6-alice", payload)
	}
	expected := int64(3 * len(payload))
	row := poll(expected)
	// No TLS/transport padding is used. Allow at most 512 bytes of implementation
	// framing, and require exact persistence of the observed counters thereafter.
	if row.Up > expected+512 || row.Down > expected+512 || row.ExpiryTime <= time.Now().UnixMilli() {
		t.Fatalf("byte metering/activation: %+v expected %d (+512 max)", row, expected)
	}
	beforeUp, beforeDown := row.Up, row.Down
	for range 3 {
		if err := svc.pollHiddifyTraffic(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if row = poll(expected); row.Up != beforeUp || row.Down != beforeDown {
		t.Fatal("replayed cumulative snapshot", row)
	}
	dbPath := filepath.Join(os.Getenv("XUI_DB_FOLDER"), "x-ui.db")
	if err := database.CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(dbPath); err != nil {
		t.Fatal(err)
	}
	if row = poll(expected); row.Up != beforeUp || row.Down != beforeDown {
		t.Fatal("database reopen replayed counters", row)
	}
	if err := svc.Restart(ctx); err != nil {
		t.Fatal(err)
	}
	transfer(t, "stage6-alice", payload)
	row = poll(beforeUp + int64(len(payload)))
	if row.Up != beforeUp+int64(len(payload)) || row.Down != beforeDown+int64(len(payload)) {
		t.Fatal("core epoch lost or duplicated bytes", row)
	}
	t.Logf("metered TCP up=%d down=%d; duplicate polls, DB reopen and core restart preserved counters", row.Up, row.Down)
	for _, state := range []string{"disabled", "expired", "quota"} {
		t.Run(state, func(t *testing.T) {
			rec, e := inboundSvc.clientService.GetRecordByEmail(nil, "stage6-alice")
			if e != nil {
				t.Fatal(e)
			}
			enabled := *rec.ToClient()
			enabled.Enable = true
			enabled.ExpiryTime = time.Now().Add(time.Hour).UnixMilli()
			enabled.TotalGB = 0
			if _, e := inboundSvc.clientService.Update(inboundSvc, rec.Id, enabled, 0); e != nil {
				t.Fatal(e)
			}
			active, e := dial("stage6-alice")
			if e != nil {
				t.Fatal(e)
			}
			defer active.Close()
			_ = active.SetDeadline(time.Now().Add(5 * time.Second))
			_, _ = active.Write([]byte("live"))
			if _, e = io.ReadFull(active, make([]byte, 4)); e != nil {
				t.Fatal(e)
			}
			changed := enabled
			switch state {
			case "disabled":
				changed.Enable = false
			case "expired":
				changed.ExpiryTime = 1
			case "quota":
				changed.TotalGB = 1
			}
			if _, e := inboundSvc.clientService.Update(inboundSvc, rec.Id, changed, 0); e != nil {
				t.Fatal(e)
			}
			if _, _, e := inboundSvc.AddTraffic(nil, nil); e != nil {
				t.Fatal(e)
			}
			if e := svc.enforceHiddifySessions(ctx); e != nil {
				t.Fatal(e)
			}
			_ = active.SetDeadline(time.Now().Add(time.Second))
			_, _ = active.Write([]byte("blocked"))
			var b [1]byte
			n, e := active.Read(b[:])
			var timeout net.Error
			if n > 0 || e == nil || (errors.As(e, &timeout) && timeout.Timeout()) {
				t.Fatalf("active session not revoked: bytes=%d error=%v", n, e)
			}
			c, e := dial("stage6-alice")
			if c != nil {
				_ = c.Close()
			}
			if e == nil {
				t.Fatal("revoked client opened a new connection")
			}
			transfer(t, "stage6-bob", []byte("unaffected-client"))
		})
	}
}
