package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
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
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	statsproto "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func phase6TrafficRows(t *testing.T) {
	t.Helper()
	db := database.GetDB()
	for _, row := range []any{&model.Inbound{Tag: "vpn", Port: 46408, Protocol: model.VLESS, Enable: true, Up: 7, Settings: `{"decryption":"none","clients":[]}`, StreamSettings: `{"network":"tcp","security":"none"}`}, &coremodel.ClientTraffic{Email: "alice", Enable: true, Up: 11}} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
}
func phase6Snapshot(value int64) []singbox.V2RayStat {
	return []singbox.V2RayStat{{Name: "user>>>alice>>>traffic>>>uplink", Value: value}, {Name: "inbound>>>vpn>>>traffic>>>uplink", Value: value}}
}
func phase6AssertUsage(t *testing.T, user, inbound int64) {
	t.Helper()
	var client coremodel.ClientTraffic
	var ib model.Inbound
	if err := database.GetDB().Where("email = ?", "alice").First(&client).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Where("tag = ?", "vpn").First(&ib).Error; err != nil {
		t.Fatal(err)
	}
	if client.Up != user || ib.Up != inbound {
		t.Fatalf("usage %d/%d, want %d/%d", client.Up, ib.Up, user, inbound)
	}
}

func TestHiddifyCumulativeCheckpointSurvivesPanelAndCoreRestarts(t *testing.T) {
	setupBulkDB(t)
	phase6TrafficRows(t)
	svc := NativeCore(CoreTypeHiddify)
	for _, value := range []int64{100, 100, 125} {
		if err := svc.commitHiddifySnapshot("core-1", phase6Snapshot(value)); err != nil {
			t.Fatal(err)
		}
	}
	phase6AssertUsage(t, 136, 132)
	path := filepath.Join(os.Getenv("XUI_DB_FOLDER"), "x-ui.db")
	if err := database.CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(path); err != nil {
		t.Fatal(err)
	}
	// Fresh service, same live core: persisted baseline must prevent replay.
	svc = NativeCore(CoreTypeHiddify)
	if err := svc.commitHiddifySnapshot("core-1", phase6Snapshot(125)); err != nil {
		t.Fatal(err)
	}
	phase6AssertUsage(t, 136, 132)
	if err := svc.commitHiddifySnapshot("core-2", phase6Snapshot(8)); err != nil {
		t.Fatal(err)
	}
	phase6AssertUsage(t, 144, 140)
	// Operator reset clears totals, not the core's cumulative baseline.
	if err := database.GetDB().Model(&coremodel.ClientTraffic{}).Where("email = ?", "alice").Update("up", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.commitHiddifySnapshot("core-2", phase6Snapshot(8)); err != nil {
		t.Fatal(err)
	}
	phase6AssertUsage(t, 0, 140)
	if err := svc.commitHiddifySnapshot("core-2", phase6Snapshot(10)); err != nil {
		t.Fatal(err)
	}
	phase6AssertUsage(t, 2, 142)
}

func TestHiddifyCheckpointRollsBackWithTrafficFailure(t *testing.T) {
	setupBulkDB(t)
	phase6TrafficRows(t)
	svc := NativeCore(CoreTypeHiddify)
	db := database.GetDB()
	if err := db.Exec(`CREATE TRIGGER phase6_fail_usage BEFORE UPDATE ON client_traffics BEGIN SELECT RAISE(ABORT, 'injected traffic write failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DROP TRIGGER IF EXISTS phase6_fail_usage`) })
	if err := svc.commitHiddifySnapshot("core-1", phase6Snapshot(100)); err == nil {
		t.Fatal("expected database failure")
	}
	phase6AssertUsage(t, 11, 7)
	var count int64
	if err := db.Model(&model.CoreTrafficCheckpoint{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("baseline committed without usage: %d %v", count, err)
	}
	if err := db.Exec(`DROP TRIGGER phase6_fail_usage`).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.commitHiddifySnapshot("core-1", phase6Snapshot(100)); err != nil {
		t.Fatal(err)
	}
	phase6AssertUsage(t, 111, 107)
	for _, values := range [][]singbox.V2RayStat{append(phase6Snapshot(100), phase6Snapshot(100)[0]), phase6Snapshot(-1)} {
		if err := svc.commitHiddifySnapshot("core-1", values); err == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
	phase6AssertUsage(t, 111, 107)
}

func TestHiddifyPollDoesNotResetAfterLostReply(t *testing.T) {
	svc, _, _ := stage5ProcessFixture(t)
	phase6TrafficRows(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	var calls atomic.Int32
	server.RegisterService(&grpc.ServiceDesc{ServiceName: "v2ray.core.app.stats.command.StatsService", HandlerType: (*interface{})(nil), Methods: []grpc.MethodDesc{{MethodName: "QueryStats", Handler: func(_ any, ctx context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		request := &statsproto.QueryStatsRequest{}
		if err := decode(request); err != nil {
			return nil, err
		}
		if request.Reset_ {
			return nil, status.Error(codes.InvalidArgument, "destructive query")
		}
		if calls.Add(1) == 1 {
			return nil, status.Error(codes.Unavailable, "lost reply")
		}
		return &statsproto.QueryStatsResponse{Stat: []*statsproto.Stat{{Name: "user>>>alice>>>traffic>>>uplink", Value: 100}, {Name: "inbound>>>vpn>>>traffic>>>uplink", Value: 100}}}, nil
	}}}}, struct{}{})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	old := hiddifyStats
	hiddifyStats = singbox.NewV2RayStatsClient(listener.Addr().String(), hiddify.StatsQueryMethod)
	t.Cleanup(func() { hiddifyStats.Close(); hiddifyStats = old })
	if err := svc.pollHiddifyTraffic(context.Background()); err == nil {
		t.Fatal("expected lost reply")
	}
	for i := 0; i < 2; i++ {
		if err := svc.pollHiddifyTraffic(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	phase6AssertUsage(t, 111, 107)
	before, err := svc.process().RuntimeID()
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, err := svc.process().RuntimeID()
	if err != nil || before == after {
		t.Fatalf("core restart reused statistics epoch: %s %s %v", before, after, err)
	}
	if err := svc.pollHiddifyTraffic(context.Background()); err != nil {
		t.Fatal(err)
	}
	phase6AssertUsage(t, 211, 207)

}

func TestHiddifyCheckpointQuotaDisablesCommittedUser(t *testing.T) {
	setupConflictDB(t)
	calls, configs := hiddifyApplyRecorder(t)
	seedInboundConflict(t, "vpn", "", 46406, model.VLESS, `{"network":"tcp"}`, `{"clients":[{"email":"alice","id":"11111111-1111-4111-8111-111111111111","enable":true}]}`)
	ib := loadInboundByTag(t, "vpn")
	clients, err := (&InboundService{}).GetClients(ib)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&ClientService{}).SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatal(err)
	}
	seedClientTraffic(t, ib.Id, "alice", true)
	if err := database.GetDB().Model(&coremodel.ClientTraffic{}).Where("email = ?", "alice").Update("total", 100).Error; err != nil {
		t.Fatal(err)
	}
	svc := NativeCore(CoreTypeHiddify)
	// Maintenance may fail after usage was saved. Its retry must not replay bytes.
	db := database.GetDB()
	if err := db.Exec(`CREATE TRIGGER phase6_fail_disable BEFORE UPDATE OF enable ON client_traffics WHEN NEW.enable = 0 BEGIN SELECT RAISE(ABORT, 'injected disable failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.commitHiddifySnapshot("core-1", phase6Snapshot(100)); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("rolled-back disable applied to runtime")
	}
	if err := db.Exec(`DROP TRIGGER phase6_fail_disable`).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.commitHiddifySnapshot("core-1", phase6Snapshot(100)); err != nil {
		t.Fatal(err)
	}
	var traffic coremodel.ClientTraffic
	if err := database.GetDB().Where("email = ?", "alice").First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Enable || traffic.Up != 100 {
		t.Fatalf("quota not applied: %+v", traffic)
	}
	var record model.ClientRecord
	if err := database.GetDB().Where("email = ?", "alice").First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.Enable || calls.Load() != 1 || strings.Contains((*configs)[0], `"alice"`) {
		t.Fatal("disabled user not removed from desired runtime state")
	}
}

func TestHiddifyCheckpointExpiryAndRenewalWithoutTraffic(t *testing.T) {
	for _, renew := range []bool{false, true} {
		t.Run(fmt.Sprint("renew=", renew), func(t *testing.T) {
			setupBulkDB(t)
			calls, _ := hiddifyApplyRecorder(t)
			expiry := time.Now().Add(-time.Hour).UnixMilli()
			reset := 0
			if renew {
				reset = 1
			}
			client := model.Client{Email: "alice", ID: "11111111-1111-4111-8111-111111111111", Enable: !renew, ExpiryTime: expiry, Reset: reset}
			ib := mkInbound(t, 46407, model.VLESS, clientsSettings(t, []model.Client{client}))
			if err := (&ClientService{}).SyncInbound(nil, ib.Id, []model.Client{client}); err != nil {
				t.Fatal(err)
			}
			if err := database.GetDB().Create(&coremodel.ClientTraffic{Email: "alice", InboundId: ib.Id, Enable: !renew, ExpiryTime: expiry, Reset: reset, Up: 50}).Error; err != nil {
				t.Fatal(err)
			}
			svc := NativeCore(CoreTypeHiddify)
			if err := svc.commitHiddifySnapshot("core-1", nil); err != nil {
				t.Fatal(err)
			}
			var row coremodel.ClientTraffic
			if err := database.GetDB().Where("email = ?", "alice").First(&row).Error; err != nil {
				t.Fatal(err)
			}
			if renew {
				if !row.Enable || row.Up != 0 || row.ExpiryTime <= time.Now().UnixMilli() {
					t.Fatalf("renewal not applied: %+v", row)
				}
			} else if row.Enable || row.Up != 50 {
				t.Fatalf("expiry not applied without bytes: %+v", row)
			}
			if calls.Load() != 1 {
				t.Fatalf("maintenance applied %d times", calls.Load())
			}
		})
	}
}

func TestHiddifyConcurrentSnapshotsUseSerialWriter(t *testing.T) {
	setupBulkDB(t)
	phase6TrafficRows(t)
	startSerializedWriter(t)
	svc := NativeCore(CoreTypeHiddify)
	var group sync.WaitGroup
	failures := make(chan error, 16)
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() { defer group.Done(); failures <- svc.commitHiddifySnapshot("core-1", phase6Snapshot(100)) }()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	phase6AssertUsage(t, 111, 107)
	// An explicit reset is visible even when counters temporarily report zero.
	if err := svc.commitHiddifySnapshot("core-1", phase6Snapshot(0)); err != nil {
		t.Fatal(err)
	}
	if err := svc.commitHiddifySnapshot("core-1", phase6Snapshot(5)); err != nil {
		t.Fatal(err)
	}
	phase6AssertUsage(t, 116, 112)
}

func TestHiddifyDisabledSessionEnforcementRetries(t *testing.T) {
	setupBulkDB(t)
	if err := database.GetDB().Create(&model.ClientRecord{Email: "alice", Enable: false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.Inbound{Tag: "disabled", Enable: false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	attempts := map[string]int{}
	removed := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodDelete {
			id := strings.TrimPrefix(r.URL.Path, "/connections/")
			attempts[id]++
			if id == "alice" && attempts[id] == 1 {
				w.WriteHeader(500)
				return
			}
			removed[id] = true
			w.WriteHeader(204)
			return
		}
		rows := []map[string]any{}
		for _, id := range []string{"alice", "bob", "anonymous"} {
			if removed[id] {
				continue
			}
			user, tag := id, "enabled"
			if id == "anonymous" {
				user = ""
				tag = "disabled"
			}
			rows = append(rows, map[string]any{"id": id, "metadata": map[string]any{"user": user, "type": "vless/" + tag}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"connections": rows})
	}))
	defer server.Close()
	cfg, _ := json.Marshal(map[string]any{"experimental": map[string]any{"clash_api": map[string]any{"external_controller": strings.TrimPrefix(server.URL, "http://")}}})
	singbox.SetRuntimeConfigReader(func() []byte { return cfg })
	t.Cleanup(func() {
		singbox.SetRuntimeConfigReader(func() []byte { return SelectedNativeCore().process().AppliedConfig() })
	})
	svc := NativeCore(CoreTypeHiddify)
	if err := svc.enforceHiddifySessions(context.Background()); err == nil {
		t.Fatal("partial API failure hidden")
	}
	for i := 0; i < 2; i++ {
		if err := svc.enforceHiddifySessions(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts["alice"] != 2 || attempts["anonymous"] != 1 || attempts["bob"] != 0 {
		t.Fatalf("incorrect revocation/retry: %v", attempts)
	}
}

func TestHiddifyCheckpointFollowsRenamedTrafficOwner(t *testing.T) {
	setupBulkDB(t)
	phase6TrafficRows(t)
	svc := NativeCore(CoreTypeHiddify)
	if err := svc.commitHiddifySnapshot("core-1", phase6Snapshot(100)); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&coremodel.ClientTraffic{}).Where("email = ?", "alice").Update("email", "renamed").Error; err != nil {
		t.Fatal(err)
	}
	// The old running config still reports alice until its pending apply.
	if err := svc.commitHiddifySnapshot("core-1", phase6Snapshot(125)); err != nil {
		t.Fatal(err)
	}
	var row coremodel.ClientTraffic
	if err := database.GetDB().Where("email = ?", "renamed").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Up != 136 {
		t.Fatalf("lost bytes after email edit: %+v", row)
	}
	// Do not charge a replacement user reusing the old source name.
	if err := database.GetDB().Create(&coremodel.ClientTraffic{Email: "alice", Enable: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.commitHiddifySnapshot("core-1", phase6Snapshot(130)); err != nil {
		t.Fatal(err)
	}
	var replacement coremodel.ClientTraffic
	if err := database.GetDB().Where("email = ?", "alice").First(&replacement).Error; err != nil {
		t.Fatal(err)
	}
	if replacement.Up != 0 {
		t.Fatal("old core usage charged to replacement owner")
	}
}
