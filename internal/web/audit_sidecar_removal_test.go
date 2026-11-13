//go:build !windows

package web

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
	"github.com/SawaMEN/3x-ui/v3/internal/web/job"
	webruntime "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

type removalFixture struct {
	ib           model.Inbound
	start        func() error
	stop, runJob func()
	running      func() bool
}

func newRemovalFixture(t *testing.T, protocol model.Protocol) *removalFixture {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	f := &removalFixture{ib: model.Inbound{Id: 1001, Enable: true, Tag: "removal", Protocol: protocol, Listen: "127.0.0.1", Port: 34588, Settings: `{"hostname":"fixture.example.test","clients":[{"email":"first","enable":true,"password":"synthetic","secret":"0123456789abcdef0123456789abcdef"}]}`}}
	if err := database.GetDB().Create(&f.ib).Error; err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(dir, "pid")
	t.Setenv("XUI_AUDIT_REMOVAL_PID", pidFile)
	script := []byte("#!/bin/sh\ncase \"$1\" in\nget|reload) exit 0;;\n*) echo $$ > \"$XUI_AUDIT_REMOVAL_PID\"; exec sleep 60;;\nesac\n")
	binary := ""
	switch protocol {
	case model.Mieru:
		mgr := mieru.GetManager()
		f.stop = mgr.StopAll
		f.running = mgr.HasRunning
		binary = "mita"
		f.runJob = job.NewMieruJob().Run
		f.start = func() error {
			inst, ok := mieru.InstanceFromInbound(&f.ib)
			if !ok {
				t.Fatal("invalid fixture")
			}
			return mgr.Ensure(inst)
		}
	case model.MTProto:
		mgr := mtproto.GetManager()
		f.stop = mgr.StopAll
		f.running = mgr.HasRunning
		binary = mtproto.GetBinaryName()
		f.runJob = job.NewMtprotoJob().Run
		f.start = func() error {
			inst, ok := mtproto.InstanceFromInbound(&f.ib)
			if !ok {
				t.Fatal("invalid fixture")
			}
			return mgr.Ensure(inst)
		}
	default:
		mgr := externalvpn.GetManager()
		f.stop = mgr.StopAll
		binary = "trusttunnel_endpoint"
		f.runJob = job.NewExternalVPNJob().Run
		f.start = func() error {
			inst, err := externalvpn.FromInbound(&f.ib)
			if err != nil {
				return err
			}
			return mgr.Ensure(inst)
		}
		f.running = func() bool {
			data, err := os.ReadFile(pidFile)
			if err != nil {
				return false
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			return err == nil && syscall.Kill(pid, 0) == nil
		}
	}
	f.stop()
	t.Cleanup(f.stop)
	if err := os.WriteFile(filepath.Join(dir, binary), script, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := f.start(); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestDelayedSidecarRemovalUsesCommittedState(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Mieru, model.MTProto, model.TrustTunnel} {
		for _, action := range []string{"stale-delete", "stale-empty-clients", "disabled", "deleted", "depleted", "read-failure"} {
			t.Run(string(protocol)+"/"+action, func(t *testing.T) {
				f := newRemovalFixture(t, protocol)
				db := database.GetDB()
				local := webruntime.NewLocal(webruntime.LocalDeps{})
				snapshot := f.ib
				keep := action == "stale-delete" || action == "stale-empty-clients" || action == "read-failure"
				switch action {
				case "stale-delete":
					snapshot.Enable = false
				case "stale-empty-clients":
					snapshot.Settings = `{"hostname":"fixture.example.test","clients":[]}`
				case "disabled":
					if err := db.Model(&f.ib).Update("enable", false).Error; err != nil {
						t.Fatal(err)
					}
				case "deleted":
					if err := db.Delete(&f.ib).Error; err != nil {
						t.Fatal(err)
					}
				case "depleted":
					if err := db.Create(&coremodel.ClientTraffic{InboundId: f.ib.Id, Email: "first", Enable: false}).Error; err != nil {
						t.Fatal(err)
					}
					snapshot.RuntimeSourceRevision = sidecartraffic.RevisionOf(&f.ib)
					snapshot.RuntimeClientRevision = sidecartraffic.DisabledRevision(map[string]bool{"first": true})
					snapshot.Settings = `{"hostname":"fixture.example.test","clients":[]}`
				case "read-failure":
					if err := db.Callback().Query().Before("gorm:query").Register("audit:removal-read-failure", func(tx *gorm.DB) { tx.AddError(errors.New("fixture failure")) }); err != nil {
						t.Fatal(err)
					}
					defer db.Callback().Query().Remove("audit:removal-read-failure")
				}
				var err error
				if action == "stale-empty-clients" || action == "depleted" {
					err = local.UpdateInbound(context.Background(), &f.ib, &snapshot)
				} else {
					err = local.DelInbound(context.Background(), &snapshot)
				}
				if action == "read-failure" {
					if err == nil {
						t.Error("removal ignored failed authoritative DB read")
					}
				} else if err != nil && (action != "stale-empty-clients" || !errors.Is(err, sidecartraffic.ErrStaleDesired)) {
					t.Fatal(err)
				}
				if f.running() != keep {
					t.Fatalf("running=%t, want %t after %s", f.running(), keep, action)
				}
			})
		}
	}
}

func TestEmptySidecarJobSnapshotCannotStopNewStartup(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Mieru, model.MTProto, model.TrustTunnel} {
		t.Run(string(protocol), func(t *testing.T) {
			f := newRemovalFixture(t, protocol)
			db := database.GetDB()
			f.stop()
			if err := db.Model(&f.ib).Update("enable", false).Error; err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			if err := db.Callback().Query().After("gorm:query").Register("audit:empty-desired-barrier", func(tx *gorm.DB) {
				sql := tx.Statement.SQL.String()
				if tx.Statement.Table == "inbounds" && strings.Contains(sql, "enable =") && strings.Contains(sql, "node_id IS NULL") {
					once.Do(func() { close(entered); <-release })
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer db.Callback().Query().Remove("audit:empty-desired-barrier")
			jobDone := make(chan struct{})
			go func() { defer close(jobDone); f.runJob() }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				close(release)
				t.Fatal("job never reached desired snapshot")
			}
			if err := db.Model(&f.ib).Update("enable", true).Error; err != nil {
				close(release)
				t.Fatal(err)
			}
			started := make(chan error, 1)
			go func() { started <- f.start() }()
			// The corrected job holds the manager lock while taking its snapshot; a
			// newer startup joins it. The old job lets startup finish before replaying
			// its empty snapshot and then incorrectly stops that child.
			var startErr error
			joined := false
			select {
			case startErr = <-started:
				joined = true
			case <-time.After(300 * time.Millisecond):
			}
			close(release)
			if !joined {
				select {
				case startErr = <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("startup not joined")
				}
			}
			if startErr != nil {
				t.Fatal(startErr)
			}
			select {
			case <-jobDone:
			case <-time.After(5 * time.Second):
				t.Fatal("job not joined")
			}
			if !f.running() {
				t.Fatal("stale empty desired stopped newly started runtime")
			}
		})
	}
}

func TestSidecarResolverFailurePreservesRunningProcess(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Mieru, model.MTProto, model.TrustTunnel} {
		t.Run(string(protocol), func(t *testing.T) {
			f := newRemovalFixture(t, protocol)
			failure := errors.New("desired read failed")
			calls := 0
			check := func(ctx context.Context) {
				calls++
				if _, ok := ctx.Deadline(); !ok {
					t.Error("resolver has no DB deadline")
				}
			}
			resolve := func(ctx context.Context) error {
				switch protocol {
				case model.Mieru:
					_, err := mieru.GetManager().ReconcileCurrent(ctx, func(ctx context.Context) ([]mieru.Instance, error) { check(ctx); return nil, failure })
					return err
				case model.MTProto:
					_, err := mtproto.GetManager().ReconcileCurrent(ctx, func(ctx context.Context) ([]mtproto.Instance, error) { check(ctx); return nil, failure })
					return err
				default:
					_, err := externalvpn.GetManager().ReconcileCurrent(ctx, func(ctx context.Context) ([]externalvpn.Instance, error) { check(ctx); return nil, failure })
					return err
				}
			}
			if err := resolve(context.Background()); !errors.Is(err, failure) {
				t.Fatalf("resolver error lost: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := resolve(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled resolver: %v", err)
			}
			if calls != 1 {
				t.Fatalf("cancelled operation invoked resolver; calls=%d", calls)
			}
			if !f.running() {
				t.Fatal("resolver error/cancellation stopped live process")
			}
		})
	}
}
