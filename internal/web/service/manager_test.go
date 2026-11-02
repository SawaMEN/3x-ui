package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func managerFixture(t *testing.T) (*ManagerService, *model.Inbound, *model.ManagerSubscription) {
	t.Helper()
	setupBulkDB(t)
	useTestRuntimeManager(t).SetLocalRuntimeOverride(&fakeNodeRuntime{})
	ib := mkInbound(t, 32101, model.VLESS, `{"clients":[],"decryption":"none"}`)
	ib.UserId = 1
	ib.StreamSettings = `{"network":"tcp","security":"none"}`
	if err := database.GetDB().Save(ib).Error; err != nil {
		t.Fatal(err)
	}
	s := &ManagerService{}
	sub, err := s.Save(1, model.ManagerSubscription{Name: "Office", Enabled: true, IntervalMinutes: 60, Entries: []model.ManagerEntry{{TemplateID: ib.Id, Mode: "rotate", Port: "random", Label: "VLESS"}}})
	if err != nil {
		t.Fatal(err)
	}
	return s, ib, sub
}

func managerRun(t *testing.T, s *ManagerService, sub *model.ManagerSubscription) *model.ManagerOperation {
	t.Helper()
	op, err := s.Enqueue(1, sub.ID, "rotate")
	if err != nil {
		t.Fatal(err)
	}
	s.execute(context.Background(), op)
	if err := database.GetDB().First(sub, sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	return op
}

func TestManagerRotationKeepsIdentityAndDeletesOnlyOwnedGeneration(t *testing.T) {
	s, template, sub := managerFixture(t)
	if op := managerRun(t, s, sub); op.State != "success" {
		t.Fatal(op.Message)
	}
	token, client, old := sub.Token, sub.ClientID, sub.Published[0].ActiveInboundID
	if client == 0 || !sub.OwnClient {
		t.Fatalf("client not created: %+v", sub)
	}
	if op := managerRun(t, s, sub); op.State != "success" {
		t.Fatal(op.Message)
	}
	if sub.Token != token || sub.ClientID != client || sub.Published[0].ActiveInboundID == old {
		t.Fatalf("unstable generation: %+v", sub)
	}
	var n int64
	database.GetDB().Model(&model.Inbound{}).Where("id = ?", old).Count(&n)
	if n != 0 {
		t.Fatal("retired inbound remains")
	}
	if err := database.GetDB().First(&model.Inbound{}, template.Id).Error; err != nil {
		t.Fatalf("template deleted: %v", err)
	}
	snap, err := s.Snapshot(1, 1, "", "")
	if err != nil || len(snap.Clients) != 1 || snap.OperationTotal != 2 {
		t.Fatalf("snapshot: %+v %v", snap, err)
	}
}

func TestManagerFailedGenerationPreservesPublication(t *testing.T) {
	s, template, sub := managerFixture(t)
	if op := managerRun(t, s, sub); op.State != "success" {
		t.Fatal(op.Message)
	}
	old := sub.Published[0].ActiveInboundID
	if err := database.GetDB().Delete(template).Error; err != nil {
		t.Fatal(err)
	}
	op := managerRun(t, s, sub)
	if op.State != "failed" || sub.Published[0].ActiveInboundID != old {
		t.Fatalf("failed generation published: %+v %+v", op, sub)
	}
}

func TestManagerQueueDeduplicatesAndIsolatesOwners(t *testing.T) {
	s, _, sub := managerFixture(t)
	a, err := s.Enqueue(1, sub.ID, "rotate")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Enqueue(1, sub.ID, "rotate")
	if err != nil || a.ID != b.ID {
		t.Fatal("duplicate operation")
	}
	if _, err := s.Enqueue(2, sub.ID, "delete"); err == nil {
		t.Fatal("cross-owner deletion")
	}
	if _, err := s.Save(1, *sub); err == nil {
		t.Fatal("editing while queued")
	}
	snap, err := s.Snapshot(2, 1, "", "")
	if err != nil || len(snap.Subscriptions) != 0 || len(snap.Operations) != 0 {
		t.Fatalf("cross-owner snapshot: %+v %v", snap, err)
	}
}

func TestManagerEditsPreserveServerOwnedFields(t *testing.T) {
	s, _, sub := managerFixture(t)
	if op := managerRun(t, s, sub); op.State != "success" {
		t.Fatal(op.Message)
	}
	token, old := sub.Token, sub.Published[0].ActiveInboundID
	sub.Token = "attacker"
	sub.OwnClient = false
	sub.Published = []model.ManagerEntry{{ActiveInboundID: 999}}
	sub.Entries[0].ActiveInboundID = 999
	v, err := s.Save(1, *sub)
	if err != nil {
		t.Fatal(err)
	}
	if v.Token != token || !v.OwnClient || v.Published[0].ActiveInboundID != old || v.Entries[0].ActiveInboundID != 0 {
		t.Fatalf("trusted server fields: %+v", v)
	}
}

func TestManagerRecoveryInterruptsRunningWithoutReplaying(t *testing.T) {
	s, _, sub := managerFixture(t)
	op, err := s.Enqueue(1, sub.ID, "rotate")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(op).Update("state", "running").Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var v model.ManagerOperation
		if err := database.GetDB().First(&v, op.ID).Error; err != nil {
			t.Fatal(err)
		}
		if v.State == "interrupted" {
			if v.ActiveKey != nil {
				t.Fatal("active lease retained")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("operation was not interrupted")
}

func TestManagerDomainsAndRouting(t *testing.T) {
	s, _, _ := managerFixture(t)
	if err := s.ImportDomains(1, "EXAMPLE.COM.\nhttps://www.example.net/path\n1.2.3.4\nexample.com", false); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Snapshot(1, 1, "", "")
	if len(snap.Domains) != 2 {
		t.Fatalf("domains: %+v", snap.Domains)
	}
	if err := s.ImportDomains(1, "bad;$(touch /tmp/not-created)", true); err == nil {
		t.Fatal("invalid replacement accepted")
	}
	snap, _ = s.Snapshot(1, 1, "", "")
	if len(snap.Domains) != 2 {
		t.Fatal("invalid replacement destroyed list")
	}
	for _, scheme := range []string{"happ", "incy"} {
		v, err := managerRouting(scheme, `{"name":"Русский"}`)
		if err != nil || !strings.HasPrefix(v, scheme+"://routing/add/") {
			t.Fatalf("profile: %s %v", v, err)
		}
		if _, err := managerRouting(scheme, "[]"); err == nil {
			t.Fatal("array profile accepted")
		}
	}
}

type managerProvider struct{}

func (managerProvider) SubLinksForSubId(string, string) ([]string, error) { return nil, nil }
func (managerProvider) LinksForClient(_ string, ib *model.Inbound, email string) []string {
	return []string{"vless://" + email + "@" + ib.ShareAddr + ":443#" + ib.Remark}
}
func (managerProvider) LinksForInbounds(string, []*model.Inbound) []string { return nil }
func TestManagerPublicSubscriptionChecksExpiryAndQuota(t *testing.T) {
	s, ib, sub := managerFixture(t)
	_ = s
	RegisterSubLinkProvider(managerProvider{})
	t.Cleanup(func() { RegisterSubLinkProvider(nil) })
	db := database.GetDB()
	ib.ShareAddr = "vpn.example.com"
	if err := db.Save(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "subscriber", Enable: true, SubID: "private", TotalGB: 1000}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	sub.ClientID = client.Id
	sub.Published = []model.ManagerEntry{{ActiveInboundID: ib.Id, Label: "Office", Mode: "existing"}}
	if err := db.Save(sub).Error; err != nil {
		t.Fatal(err)
	}
	get := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "https://panel.example/bus/"+sub.Token, nil)
		ServeManagerSubscription(w, r, sub.Token)
		return w
	}
	w := get()
	body, err := base64.StdEncoding.DecodeString(w.Body.String())
	if w.Code != 200 || err != nil || !strings.Contains(string(body), "subscriber@vpn.example.com") {
		t.Fatalf("response: %d %s %v", w.Code, body, err)
	}
	client.ExpiryTime = time.Now().Add(-time.Hour).UnixMilli()
	if err := db.Save(client).Error; err != nil {
		t.Fatal(err)
	}
	if w := get(); w.Code != 404 {
		t.Fatalf("expired: %d", w.Code)
	}
	client.ExpiryTime = 0
	if err := db.Save(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&coremodel.ClientTraffic{Email: client.Email, Up: 1000}).Error; err != nil {
		t.Fatal(err)
	}
	if w := get(); w.Code != 404 {
		t.Fatalf("exhausted: %d", w.Code)
	}
}

func TestManagerRelayScriptOwnsOnlyExactPortsAndHidesSecrets(t *testing.T) {
	script, err := managerRelayScript(7, netip.MustParseAddr("203.0.113.5"), []int{12345, 23456}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"-t nat -A XMGR_7D -p tcp", "-t nat -A XMGR_7S -p udp", "--dport 12345", "--dport 23456", "--dst-type LOCAL", "systemctl enable 3x-ui-manager-relay-7.service"} {
		if !strings.Contains(script, v) {
			t.Fatalf("missing %s", v)
		}
	}
	for _, bad := range []string{"ufw --force", "iptables -F\n", "--dports 10000:60000", "curl "} {
		if strings.Contains(script, bad) {
			t.Fatalf("unsafe command %s", bad)
		}
	}
	if _, err := managerRelayScript(7, netip.MustParseAddr("203.0.113.5"), []int{-1}, true); err == nil {
		t.Fatal("invalid port")
	}
	raw, _ := json.Marshal(model.ManagerRelay{Credential: "secret"})
	if strings.Contains(string(raw), "secret") {
		t.Fatal("credential leak")
	}
}

type managerApplyFailure struct{ fakeNodeRuntime }

func (*managerApplyFailure) ApplyConfigNow(context.Context) error {
	return fmt.Errorf("core rejected generation")
}

func TestManagerRuntimeFailureKeepsOldGeneration(t *testing.T) {
	s, _, sub := managerFixture(t)
	if op := managerRun(t, s, sub); op.State != "success" {
		t.Fatal(op.Message)
	}
	old := sub.Published[0].ActiveInboundID
	mgr := useTestRuntimeManager(t)
	mgr.SetLocalRuntimeOverride(&managerApplyFailure{})
	op := managerRun(t, s, sub)
	if op.State != "failed" || !strings.Contains(op.Message, "core rejected") {
		t.Fatalf("runtime failure: %+v", op)
	}
	if sub.Published[0].ActiveInboundID != old {
		t.Fatal("publication changed")
	}
	if err := database.GetDB().First(&model.Inbound{}, old).Error; err != nil {
		t.Fatal(err)
	}
}
