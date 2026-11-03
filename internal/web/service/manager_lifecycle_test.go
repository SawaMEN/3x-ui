package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

func TestManagerGraceSurvivesReloadAndExpiresThroughCleanup(t *testing.T) {
	s, _, sub := managerFixture(t)
	for range 2 {
		if op := managerRun(t, s, sub); op.State != "success" {
			t.Fatal(op.Message)
		}
	}
	if len(sub.Retired) != 1 {
		t.Fatalf("retired generations: %+v", sub.Retired)
	}
	old := sub.Retired[0].Entries[0].ActiveInboundID
	current := sub.Published[0].ActiveInboundID
	op, err := s.Enqueue(1, sub.ID, "cleanup")
	if err != nil {
		t.Fatal(err)
	}
	s.execute(context.Background(), op)
	if op.State != "success" {
		t.Fatal(op.Message)
	}
	if err := database.GetDB().First(&model.Inbound{}, old).Error; err != nil {
		t.Fatalf("previous listener lost within grace: %v", err)
	}
	sub.Retired[0].ExpiresAt = time.Now().Add(-time.Second).UnixMilli()
	if err := database.GetDB().Save(sub).Error; err != nil {
		t.Fatal(err)
	}
	op, err = s.Enqueue(1, sub.ID, "cleanup")
	if err != nil {
		t.Fatal(err)
	}
	s.execute(context.Background(), op)
	if op.State != "success" {
		t.Fatal(op.Message)
	}
	var n int64
	if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", old).Count(&n).Error; err != nil || n != 0 {
		t.Fatalf("expired generation: count=%d %v", n, err)
	}
	if err := database.GetDB().First(&model.Inbound{}, current).Error; err != nil {
		t.Fatalf("current generation deleted: %v", err)
	}
}

func TestManagerCompactSnapshotDoesNotExposeRecipeAndSearchIsOwnerScoped(t *testing.T) {
	s, _, sub := managerFixture(t)
	sub.Entries[0].Mode = "generate"
	sub.Entries[0].Config = &model.ManagerInboundConfig{Protocol: model.VLESS, Settings: `{"clients":[]}`, StreamSettings: `{"network":"tcp","security":"none"}`, Sniffing: `{}`}
	var err error
	sub, err = s.Save(1, *sub)
	if err != nil {
		t.Fatal(err)
	}
	foreign := *sub
	foreign.ID = 0
	foreign.UserID = 2
	foreign.Token = "foreign"
	foreign.Name = "Other owner"
	if err := database.GetDB().Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Snapshot(1, 1, "", "", ManagerListOptions{Page: 1, Search: "Office", Compact: true})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SubscriptionTotal != 1 || len(snapshot.Subscriptions) != 1 || snapshot.Subscriptions[0].Entries[0].Config != nil {
		t.Fatalf("compact snapshot: %+v", snapshot)
	}
	snapshot, err = s.Snapshot(1, 1, "", "", ManagerListOptions{Page: 1, Search: "Other owner", Compact: true})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SubscriptionTotal != 0 || len(snapshot.Subscriptions) != 0 {
		t.Fatal("search exposed another owner's subscription")
	}
}

func TestManagerReadinessRejectsWholeGenerationBeforeWrites(t *testing.T) {
	s, template, sub := managerFixture(t)
	node := &model.Node{Name: "not-ready", Address: "node.example.com", Port: 2053, Enable: true, Status: "online", ProtocolAvailability: map[string]model.ProtocolAvailability{"vetroff": {Supported: true, Ready: false}}}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	runtime.GetManager().SetRuntimeOverride(node.Id, &fakeNodeRuntime{})
	sub.Entries = append(sub.Entries, model.ManagerEntry{Mode: "generate", Port: "random", NodeID: &node.Id, Config: &model.ManagerInboundConfig{Protocol: model.VetroffProxy, Settings: `{"clients":[]}`, StreamSettings: `{}`, Sniffing: `{}`}})
	var err error
	sub, err = s.Save(1, *sub)
	if err != nil {
		t.Fatal(err)
	}
	op := managerRun(t, s, sub)
	if op.State != "failed" || !strings.Contains(op.Message, "not ready") {
		t.Fatalf("preflight: %+v", op)
	}
	var count int64
	if err := database.GetDB().Model(&model.Inbound{}).Where("id <> ?", template.Id).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial generation written: %d %v", count, err)
	}
}

type managerBlockingRuntime struct {
	fakeNodeRuntime
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *managerBlockingRuntime) UpdateInbound(context.Context, *model.Inbound, *model.Inbound) error {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return nil
}

func TestManagerSlowNodeDoesNotHoldQueueLock(t *testing.T) {
	s, template, sub := managerFixture(t)
	node := &model.Node{Name: "slow", Address: "node.example.com", Port: 2053, Enable: true, Status: "online"}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	blocked := &managerBlockingRuntime{started: make(chan struct{}), release: make(chan struct{})}
	runtime.GetManager().SetRuntimeOverride(node.Id, blocked)
	sub.Entries[0].NodeID = &node.Id
	var err error
	sub, err = s.Save(1, *sub)
	if err != nil {
		t.Fatal(err)
	}
	op, err := s.Enqueue(1, sub.ID, "rotate")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { s.execute(context.Background(), op); close(done) }()
	t.Cleanup(func() { close(blocked.release); <-done })
	select {
	case <-blocked.started:
	case <-time.After(3 * time.Second):
		t.Fatal("node request did not start")
	}
	saved := make(chan error, 1)
	go func() {
		_, err := s.Save(1, model.ManagerSubscription{Name: "Independent", Enabled: true, IntervalMinutes: 60, Entries: []model.ManagerEntry{{Mode: "rotate", TemplateID: template.Id, Port: "random"}}})
		saved <- err
	}()
	select {
	case err := <-saved:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("slow node blocked saving an independent subscription")
	}
}
