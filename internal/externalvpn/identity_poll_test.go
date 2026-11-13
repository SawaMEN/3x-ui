package externalvpn

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

func TestOwnedPollRejectsSnapshotBeforeIdentityRebind(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	db := database.GetDB()
	mgr := &Manager{procs: map[int]*running{}}
	entered := make(chan int, 32)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var bypass atomic.Int64
	totals := make([]atomic.Int64, 10)
	originals := map[int]coremodel.ClientTraffic{}
	for id := 1; id <= 9; id++ {
		id := id
		totals[id].Store(110)
		ib := model.Inbound{Id: id, Tag: fmt.Sprintf("epoch-%d", id), Enable: true, Protocol: model.TrustTunnel}
		if err := db.Create(&ib).Error; err != nil {
			t.Fatal(err)
		}
		email := fmt.Sprintf("account-%d", id)
		row := coremodel.ClientTraffic{InboundId: id, Email: email, Enable: true}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		originals[id] = row
		owner, err := sidecartraffic.Capture("externalvpn", id, []string{email}, false)
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if bypass.Load() != int64(id) {
				entered <- id
				<-release
			}
			n := totals[id].Load()
			_, _ = fmt.Fprintf(w, `[{"username":%q,"inbound":%d,"outbound":%d}]`, email, n, n*2)
		}))
		t.Cleanup(srv.Close)
		mgr.procs[id] = &running{accounting: owner, instance: Instance{ID: id}, protocol: model.TrustTunnel, metricsAddr: strings.TrimPrefix(srv.URL, "http://"), tag: ib.Tag, counters: map[string]trafficCounters{email: {up: 10, down: 20}}}
	}
	done := make(chan error, 1)
	go func() { _, err := mgr.CollectTrafficPersisted(); done <- err }()
	observed := map[int]bool{}
	for len(observed) < 8 {
		select {
		case id := <-entered:
			observed[id] = true
		case <-time.After(5 * time.Second):
			t.Fatal("first eight polls did not start")
		}
	}
	pending := 0
	for id := 1; id <= 9; id++ {
		if !observed[id] {
			pending = id
		}
	}
	if pending == 0 {
		t.Fatal("no queued metrics snapshot")
	}
	old := originals[pending]
	if err := db.Model(&old).Update("email", "retired-"+old.Email).Error; err != nil {
		t.Fatal(err)
	}
	email := fmt.Sprintf("account-%d", pending)
	replacement := coremodel.ClientTraffic{InboundId: pending, Email: email, Enable: true}
	if err := db.Create(&replacement).Error; err != nil {
		t.Fatal(err)
	}
	next, err := sidecartraffic.Capture("externalvpn", pending, []string{email}, false)
	if err != nil {
		t.Fatal(err)
	}
	bypass.Store(int64(pending))
	mgr.mu.Lock()
	err = mgr.rebindTrafficOwnerLocked(mgr.procs[pending], next)
	mgr.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	totals[pending].Store(120)
	once.Do(func() { close(release) })
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "accounting owner changed") {
			t.Fatalf("stale owner snapshot was accepted: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("old metrics poll not joined")
	}
	if _, err := mgr.CollectTrafficPersisted(); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "externalvpn-traffic-journal", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oldUp, newUp int64
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var batch sidecartraffic.Batch
		if err := json.Unmarshal(data, &batch); err != nil {
			t.Fatal(err)
		}
		for _, delta := range batch.Clients {
			if delta.ID == old.Id {
				oldUp += delta.Up
			}
			if delta.ID == replacement.Id {
				newUp += delta.Up
			}
		}
	}
	if oldUp != 100 || newUp != 10 {
		t.Fatalf("queued old snapshot crossed identity boundary: old=%d new=%d", oldUp, newUp)
	}
}
