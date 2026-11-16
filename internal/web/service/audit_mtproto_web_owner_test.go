package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

type webOwnerFixture struct {
	tracker  mtprotoWebTrafficTracker
	ib       *model.Inbound
	old      coremodel.ClientTraffic
	state    TelemtWebProxyState
	rendered string
	current  int64
	fetchErr error
	omitted  bool
}

func newWebOwnerFixture(t *testing.T) *webOwnerFixture {
	t.Helper()
	setupConflictDB(t)
	settings := `{"webProxy":{"enabled":true,"domain":"web.example.com"},"clients":[{"email":"alice","enable":true,"secret":"00112233445566778899aabbccddeeff"}]}`
	seedInboundConflict(t, "web-owned", "", 443, model.MTProto, "", settings)
	f := &webOwnerFixture{ib: loadInboundByTag(t, "web-owned"), current: 100}
	seedClientTraffic(t, f.ib.Id, "alice", true)
	if err := database.GetDB().Where("email = ?", "alice").Take(&f.old).Error; err != nil {
		t.Fatal(err)
	}
	f.state = TelemtWebProxyState{Enabled: true, ManagedInboundID: f.ib.Id}
	f.rendered = fmt.Sprintf("[access.users]\n%q = %q\n", mtprotoWebUsername(f.ib.Id, "alice"), "00112233445566778899aabbccddeeff")
	return f
}

func (f *webOwnerFixture) binding(t *testing.T) *mtprotoWebTrafficBinding {
	t.Helper()
	b, err := captureMtprotoWebTrafficBinding(f.state, f.rendered)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (f *webOwnerFixture) fetch() ([]telemtUserStats, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	if f.omitted {
		return nil, nil
	}
	return []telemtUserStats{{Username: mtprotoWebUsername(f.ib.Id, "alice"), TotalOctets: f.current, CurrentConnections: 1}}, nil
}

func (f *webOwnerFixture) poll(t *testing.T) error {
	t.Helper()
	_, _, err := f.tracker.poll(context.Background(), func() (*mtprotoWebTrafficBinding, error) { return f.binding(t), nil }, func() bool { return true }, f.fetch)
	return err
}

func (f *webOwnerFixture) replace(t *testing.T) {
	t.Helper()
	if err := database.GetDB().Model(&coremodel.ClientTraffic{}).Where("id = ?", f.old.Id).Update("email", "retired-alice").Error; err != nil {
		t.Fatal(err)
	}
	seedClientTraffic(t, f.ib.Id, "alice", true)
}

func replayWebOwnerTraffic(t *testing.T) {
	t.Helper()
	dir := filepath.Join(os.Getenv("XUI_DB_FOLDER"), "mtproto-traffic-journal")
	if err := sidecartraffic.FlushPending(dir); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var batch SidecarTrafficBatch
		if err := json.Unmarshal(data, &batch); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := (&InboundService{}).PersistSidecarTrafficBatch(batch); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *webOwnerFixture) totals(t *testing.T, oldWant, newWant int64) {
	t.Helper()
	replayWebOwnerTraffic(t)
	var old, replacement coremodel.ClientTraffic
	if err := database.GetDB().First(&old, f.old.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Where("email = ?", "alice").Take(&replacement).Error; err != nil {
		t.Fatal(err)
	}
	if old.Down != oldWant || replacement.Down != newWant {
		t.Fatalf("old=%d replacement=%d; want %d/%d", old.Down, replacement.Down, oldWant, newWant)
	}
	var inbound model.Inbound
	if err := database.GetDB().First(&inbound, f.ib.Id).Error; err != nil {
		t.Fatal(err)
	}
	if inbound.Down != oldWant+newWant {
		t.Fatalf("inbound=%d, want %d", inbound.Down, oldWant+newWant)
	}
}

func TestMtprotoWebPollKeepsOriginalOwner(t *testing.T) {
	f := newWebOwnerFixture(t)
	if err := f.poll(t); err != nil {
		t.Fatal(err)
	}
	f.replace(t)
	f.current = 120
	if err := f.poll(t); err != nil {
		t.Fatal(err)
	}
	f.totals(t, 20, 0)
}

func TestMtprotoWebOwnerBoundaryAndRecovery(t *testing.T) {
	for _, mode := range []string{"normal", "disk failure", "missing sample", "metrics failure", "activation failure", "rollback restart", "fresh restart"} {
		t.Run(mode, func(t *testing.T) {
			f := newWebOwnerFixture(t)
			if err := f.poll(t); err != nil {
				t.Fatal(err)
			}
			f.replace(t)
			next := f.binding(t)
			f.current = 120
			dir := filepath.Join(os.Getenv("XUI_DB_FOLDER"), "mtproto-traffic-journal")
			if mode == "disk failure" {
				if err := os.WriteFile(dir, []byte("blocked"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "missing sample" {
				f.omitted = true
			}
			if mode == "metrics failure" {
				f.fetchErr = errors.New("metrics unavailable")
			}
			activated := false
			apply := func() (bool, error) {
				activated = true
				if mode == "activation failure" {
					return false, errors.New("activation failed")
				}
				if mode == "rollback restart" {
					return true, errors.New("rollback restart")
				}
				return mode == "fresh restart", nil
			}
			mutation := mode != "missing sample" && mode != "metrics failure"
			err := f.tracker.transition(context.Background(), next, mutation, true, f.fetch, apply)
			switch mode {
			case "missing sample", "metrics failure":
				if err == nil || activated {
					t.Fatal("incomplete pure identity handoff changed configuration")
				}
				f.omitted = false
				f.fetchErr = nil
				if err := f.tracker.transition(context.Background(), next, false, true, f.fetch, func() (bool, error) { return false, nil }); err != nil {
					t.Fatal(err)
				}
			case "activation failure", "rollback restart":
				if err == nil {
					t.Fatal("failed activation accepted")
				}
				if mode == "rollback restart" {
					f.current = 7
				} else {
					f.current = 130
				}
				if err := f.poll(t); err != nil {
					t.Fatal(err)
				}
				want := int64(30)
				if mode == "rollback restart" {
					want = 27
				}
				f.totals(t, want, 0)
				return
			default:
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "disk failure" {
				if err := os.Remove(dir); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "fresh restart" {
				f.current = 7
			} else {
				f.current = 130
			}
			if err := f.poll(t); err != nil {
				t.Fatal(err)
			}
			want := int64(10)
			if mode == "fresh restart" {
				want = 7
			}
			f.totals(t, 20, want)
		})
	}
}

func TestMtprotoWebRevocationSurvivesMetricsFailure(t *testing.T) {
	f := newWebOwnerFixture(t)
	if err := f.poll(t); err != nil {
		t.Fatal(err)
	}
	f.fetchErr = errors.New("metrics unavailable")
	activated := false
	if err := f.tracker.transition(context.Background(), nil, true, true, f.fetch, func() (bool, error) { activated = true; return false, nil }); err != nil {
		t.Fatal(err)
	}
	if !activated || f.tracker.binding != nil {
		t.Fatal("revocation blocked")
	}
}

func TestMtprotoWebUnmanagedConfigDoesNotPoll(t *testing.T) {
	var tracker mtprotoWebTrafficTracker
	if err := tracker.transition(context.Background(), nil, true, true, func() ([]telemtUserStats, error) { t.Fatal("unmanaged config polled metrics"); return nil, nil }, func() (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
}

func TestMtprotoWebQueuedPollJoinsOwnerBoundary(t *testing.T) {
	f := newWebOwnerFixture(t)
	if err := f.poll(t); err != nil {
		t.Fatal(err)
	}
	entered, release, activated := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	pollDone, transitionDone := make(chan error, 1), make(chan error, 1)
	go func() {
		defer close(pollDone)
		_, _, err := f.tracker.poll(context.Background(), func() (*mtprotoWebTrafficBinding, error) { return nil, errors.New("unexpected bootstrap") }, func() bool { return true }, func() ([]telemtUserStats, error) {
			close(entered)
			<-release
			return []telemtUserStats{{Username: mtprotoWebUsername(f.ib.Id, "alice"), TotalOctets: 120}}, nil
		})
		pollDone <- err
	}()
	<-entered
	f.replace(t)
	next := f.binding(t)
	go func() {
		defer close(transitionDone)
		transitionDone <- f.tracker.transition(context.Background(), next, false, true, func() ([]telemtUserStats, error) {
			return []telemtUserStats{{Username: mtprotoWebUsername(f.ib.Id, "alice"), TotalOctets: 130}}, nil
		}, func() (bool, error) { close(activated); return false, nil })
	}()
	t.Cleanup(func() { once.Do(func() { close(release) }); <-pollDone; <-transitionDone })
	select {
	case <-activated:
		t.Fatal("owner changed during old poll")
	case <-time.After(30 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	if err := <-pollDone; err != nil {
		t.Fatal(err)
	}
	if err := <-transitionDone; err != nil {
		t.Fatal(err)
	}
	f.current = 140
	if err := f.poll(t); err != nil {
		t.Fatal(err)
	}
	f.totals(t, 30, 10)
}

func TestMtprotoWebIdleActivityUsesPinnedOwner(t *testing.T) {
	f := newWebOwnerFixture(t)
	if err := f.poll(t); err != nil {
		t.Fatal(err)
	}
	f.replace(t)
	if err := f.poll(t); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(os.Getenv("XUI_DB_FOLDER"), "mtproto-traffic-journal", "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("WAL: %v %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var batch SidecarTrafficBatch
	if err := json.Unmarshal(data, &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Clients) != 1 || batch.Clients[0].ID != f.old.Id || batch.Clients[0].Down != 0 {
		t.Fatalf("activity identity lost: %+v", batch)
	}
	f.totals(t, 0, 0)
}

func TestMtprotoWebCanceledPollDoesNotAdvanceCursor(t *testing.T) {
	f := newWebOwnerFixture(t)
	if err := f.poll(t); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _, err := f.tracker.poll(ctx, func() (*mtprotoWebTrafficBinding, error) { return nil, nil }, func() bool { return true }, func() ([]telemtUserStats, error) {
		cancel()
		return []telemtUserStats{{Username: mtprotoWebUsername(f.ib.Id, "alice"), TotalOctets: 120}}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	f.replace(t)
	f.current = 130
	if err := f.poll(t); err != nil {
		t.Fatal(err)
	}
	f.totals(t, 30, 0)
}

func TestMtprotoWebColdBindingDoesNotChargeUnobservedHistory(t *testing.T) {
	f := newWebOwnerFixture(t)
	next := f.binding(t)
	if err := f.tracker.transition(context.Background(), next, false, true, func() ([]telemtUserStats, error) { return nil, nil }, func() (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	if err := f.poll(t); err != nil {
		t.Fatal(err)
	}
	f.replace(t)
	f.current = 120
	if err := f.poll(t); err != nil {
		t.Fatal(err)
	}
	f.totals(t, 20, 0)
}

func TestMtprotoWebConfigMutationDoesNotWaitForOwnerMetrics(t *testing.T) {
	for _, cold := range []bool{false, true} {
		t.Run(fmt.Sprint("cold-", cold), func(t *testing.T) {
			f := newWebOwnerFixture(t)
			if !cold {
				if err := f.poll(t); err != nil {
					t.Fatal(err)
				}
			}
			if !cold {
				f.replace(t)
			}
			next := f.binding(t)
			f.fetchErr = errors.New("metrics unavailable")
			activated := false
			if err := f.tracker.transition(context.Background(), next, true, true, f.fetch, func() (bool, error) { activated = true; return false, nil }); err != nil {
				t.Fatal(err)
			}
			if !activated {
				t.Fatal("mutation blocked")
			}
			f.fetchErr = nil
			if cold {
				f.replace(t)
			}
			f.current = 120
			if err := f.poll(t); err != nil {
				t.Fatal(err)
			}
			f.current = 130
			if err := f.poll(t); err != nil {
				t.Fatal(err)
			}
			if cold {
				f.totals(t, 10, 0)
			} else {
				f.totals(t, 0, 10)
			}
		})
	}
}
