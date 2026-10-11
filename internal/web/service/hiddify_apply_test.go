package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	webRuntime "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
	stats "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
)

func hiddifyApplyRecorder(t *testing.T) (*atomic.Int32, *[]string) {
	t.Helper()
	if err := singBoxSettingService.SetCoreType(CoreTypeHiddify); err != nil {
		t.Fatal(err)
	}
	old := webRuntime.GetManager()
	var calls atomic.Int32
	configs := []string{}
	webRuntime.SetManager(webRuntime.NewManager(webRuntime.LocalDeps{
		CoreType: func() string { return CoreTypeHiddify },
		APIPort:  func() int { t.Error("Hiddify mutation attempted Xray API"); return -1 },
		RequestConfigApply: func(context.Context) error {
			cfg, err := NativeCore(CoreTypeHiddify).GetConfig()
			if err != nil {
				return err
			}
			configs = append(configs, string(checkGeneratedHiddifyConfig(t, cfg)))
			calls.Add(1)
			return nil
		},
	}))
	t.Cleanup(func() { webRuntime.SetManager(old) })
	return &calls, &configs
}

func TestHiddifyInboundChangesApplyCommittedStateOnce(t *testing.T) {
	setupBulkDB(t)
	calls, configs := hiddifyApplyRecorder(t)
	svc := &InboundService{}
	ib := &model.Inbound{Enable: true, Protocol: model.VLESS, Port: 21043, Tag: "stage5-vless", Settings: clientsSettings(t, []model.Client{{Email: "alice", ID: "11111111-1111-4111-8111-111111111111", Enable: true}}), StreamSettings: `{"network":"tcp","security":"none"}`}
	if _, _, err := svc.AddInbound(ib); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || !strings.Contains((*configs)[0], "alice") {
		t.Fatal("add did not apply committed users")
	}
	ib.Port = 21044
	if _, _, err := svc.UpdateInbound(ib); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || !strings.Contains((*configs)[1], "21044") {
		t.Fatal("update used remove+add or stale DB state")
	}
	for _, enabled := range []bool{false, true} {
		before := calls.Load()
		if _, err := svc.SetInboundEnable(ib.Id, enabled); err != nil {
			t.Fatal(err)
		}
		if calls.Load() != before+1 {
			t.Fatal("enable toggle applied more than once")
		}
	}
	if _, err := svc.DelInbound(ib.Id); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 5 || strings.Contains((*configs)[4], "stage5-vless") {
		t.Fatal("delete left listener in applied state")
	}
}

func TestHiddifyBulkClientsApplyOneFinalConfiguration(t *testing.T) {
	setupBulkDB(t)
	calls, configs := hiddifyApplyRecorder(t)
	svc, inbounds := &ClientService{}, &InboundService{}
	first := mkInbound(t, 22043, model.VLESS, `{"decryption":"none","clients":[]}`)
	second := mkInbound(t, 22044, model.VLESS, `{"decryption":"none","clients":[]}`)
	payloads := []ClientCreatePayload{}
	emails := []string{}
	for i := 0; i < 30; i++ {
		email := fmt.Sprintf("stage5-%02d", i)
		emails = append(emails, email)
		payloads = append(payloads, ClientCreatePayload{Client: model.Client{Email: email, ID: fmt.Sprintf("%08d-1111-4111-8111-111111111111", i+1), Enable: true}, InboundIds: []int{first.Id, second.Id}})
	}
	created, _, err := svc.BulkCreate(inbounds, payloads)
	if err != nil || created.Created != 30 {
		t.Fatalf("bulk create: %+v %v", created, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("30 users on two listeners caused %d applies", calls.Load())
	}
	for _, email := range emails {
		if !strings.Contains((*configs)[0], email) {
			t.Fatalf("partial bulk state lacks %s", email)
		}
	}
	disabled, _, err := svc.BulkSetEnable(inbounds, emails, false)
	if err != nil || disabled.Changed != 30 {
		t.Fatalf("bulk disable: %+v %v", disabled, err)
	}
	if calls.Load() != 2 || strings.Contains((*configs)[1], "stage5-00") {
		t.Fatal("disabled users survived or applied repeatedly")
	}
	enabled, _, err := svc.BulkSetEnable(inbounds, emails, true)
	if err != nil || enabled.Changed != 30 {
		t.Fatalf("bulk enable: %+v %v", enabled, err)
	}
	if calls.Load() != 3 || !strings.Contains((*configs)[2], "stage5-29") {
		t.Fatal("bulk enable lost users")
	}
	deleted, _, err := svc.BulkDelete(inbounds, emails, false)
	if err != nil || deleted.Deleted != 30 {
		t.Fatalf("bulk delete: %+v %v", deleted, err)
	}
	if calls.Load() != 4 || strings.Contains((*configs)[3], "stage5-00") {
		t.Fatal("bulk delete did not apply final state once")
	}
}

// A process fixture exercises failure/rollback and restart counts. Schema checks
// use the real pinned core in the DB tests above, not this fixture.
func stage5ProcessFixture(t *testing.T) (*SingBoxService, string, *atomic.Int32) {
	t.Helper()
	setupBulkDB(t)
	if err := singBoxSettingService.SetCoreType(CoreTypeHiddify); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	starts := filepath.Join(dir, "starts")
	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
version) printf 'hiddify-core version %s\nTags: with_xui_panel,with_v2ray_api,with_awg,with_quic,with_wireguard,with_clash_api,with_utls,with_masque,with_gvisor,with_naive_outbound,with_purego,tfogo_checklinkname0\nRevision: %s\n' ;;
check) test -f "$3" || exit 2
if grep -q stage-audit-reject "$3"; then echo 'candidate rejected' >&2; exit 3; fi
if test -f "$(dirname "$0")/hold-check"; then
 touch "$(dirname "$0")/checking"
 while test -f "$(dirname "$0")/hold-check"; do sleep 0.02; done
fi ;;
srun) echo started >> %q; if grep -q stage5-fail "$3"; then exit 7; fi; exec sleep 60 ;;
*) exit 9 ;;
esac
`, hiddify.TargetVersion, hiddify.TargetRevision, starts)
	if err := os.WriteFile(hiddify.GetBinaryPath(), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	oldRuntime, oldProcess, oldStats := hiddifyRuntime, hiddifyProcess, hiddifyStats
	oldManager := webRuntime.GetManager()
	hiddifyRuntime = hiddify.NewRuntime(hiddify.GetConfigPath(), true)
	hiddifyProcess = hiddifyRuntime.Process
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	var polls atomic.Int32
	server.RegisterService(&grpc.ServiceDesc{ServiceName: "v2ray.core.app.stats.command.StatsService", HandlerType: (*interface{})(nil), Methods: []grpc.MethodDesc{{MethodName: "QueryStats", Handler: func(_ any, _ context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		if err := decode(&stats.QueryStatsRequest{}); err != nil {
			return nil, err
		}
		polls.Add(1)
		return &stats.QueryStatsResponse{}, nil
	}}}}, struct{}{})
	go server.Serve(listener)
	hiddifyStats = singbox.NewV2RayStatsClient(listener.Addr().String(), hiddify.StatsQueryMethod)
	svc := NativeCore(CoreTypeHiddify)
	svc.restartFlag().Store(false)
	svc.stoppedFlag().Store(false)
	webRuntime.SetManager(webRuntime.NewManager(webRuntime.LocalDeps{CoreType: func() string { return CoreTypeHiddify }, RequestConfigApply: func(context.Context) error { svc.SetToNeedRestart(); return nil }}))
	t.Cleanup(func() {
		_ = hiddifyRuntime.Stop(context.Background())
		hiddifyStats.Close()
		server.Stop()
		hiddifyRuntime, hiddifyProcess, hiddifyStats = oldRuntime, oldProcess, oldStats
		webRuntime.SetManager(oldManager)
		svc.restartFlag().Store(false)
		svc.stoppedFlag().Store(false)
	})
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return svc, starts, &polls
}

func stage5Starts(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "started\n")
}

func TestHiddifyPendingApplyBatchNoopAndRollback(t *testing.T) {
	svc, starts, polls := stage5ProcessFixture(t)
	previous := svc.process().AppliedConfig()
	svc.SetToNeedRestart()
	finish := beginLocalConfigBatch()
	svc.ApplyPendingRestart(context.Background())
	if !svc.restartFlag().Load() || stage5Starts(t, starts) != 1 {
		t.Fatal("active batch consumed pending apply")
	}
	finish()
	svc.ApplyPendingRestart(context.Background())
	if svc.restartFlag().Load() || stage5Starts(t, starts) != 1 || polls.Load() != 0 {
		t.Fatal("unchanged config restarted or reset stats")
	}
	if err := os.WriteFile(svc.ProcessConfigPath(), []byte(`{"preview":"unapplied"}`), 0600); err != nil {
		t.Fatal(err)
	}
	svc.SetToNeedRestart()
	svc.ApplyPendingRestart(context.Background())
	if got, _ := os.ReadFile(svc.ProcessConfigPath()); string(got) != string(previous) || stage5Starts(t, starts) != 1 {
		t.Fatal("preview disk file confused applied state")
	}
	ib := seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none"}`, `{}`)
	svc.SetToNeedRestart()
	svc.ApplyPendingRestart(context.Background())
	if svc.restartFlag().Load() || stage5Starts(t, starts) != 2 || !strings.Contains(string(svc.process().AppliedConfig()), ib.Tag) {
		t.Fatal("committed listener not applied")
	}
	stable := svc.process().AppliedConfig()
	if err := database.GetDB().Model(ib).Update("tag", "stage5-fail").Error; err != nil {
		t.Fatal(err)
	}
	svc.SetToNeedRestart()
	svc.ApplyPendingRestart(context.Background())
	if !svc.IsRunning() || !svc.restartFlag().Load() || stage5Starts(t, starts) != 4 {
		t.Fatal("failed startup did not restore process and queue retry")
	}
	if got, _ := os.ReadFile(svc.ProcessConfigPath()); string(got) != string(stable) || string(svc.process().AppliedConfig()) != string(stable) {
		t.Fatal("failed candidate displaced last applied config")
	}
	if err := database.GetDB().Model(ib).Update("tag", "stage5-fixed").Error; err != nil {
		t.Fatal(err)
	}
	svc.ApplyPendingRestart(context.Background())
	if svc.restartFlag().Load() || stage5Starts(t, starts) != 5 || !strings.Contains(string(svc.process().AppliedConfig()), "stage5-fixed") {
		t.Fatal("corrected DB state did not retry")
	}
	if err := svc.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc.SetToNeedRestart()
	svc.ApplyPendingRestart(context.Background())
	if svc.IsRunning() || stage5Starts(t, starts) != 5 {
		t.Fatal("pending apply undid manual stop")
	}
}

func TestHiddifyBulkAttachDetachAndSnellDelete(t *testing.T) {
	setupBulkDB(t)
	calls, configs := hiddifyApplyRecorder(t)
	clients, inbounds := &ClientService{}, &InboundService{}
	first := mkInbound(t, 23043, model.VLESS, `{"decryption":"none","clients":[]}`)
	second := mkInbound(t, 23044, model.VLESS, `{"decryption":"none","clients":[]}`)
	record := &model.ClientRecord{Email: "attached", SubID: "attached-sub", UUID: "11111111-1111-4111-8111-111111111111", Enable: true}
	if err := database.GetDB().Create(record).Error; err != nil {
		t.Fatal(err)
	}
	result, _, err := clients.BulkAttach(inbounds, []string{record.Email}, []int{first.Id, second.Id})
	if err != nil || len(result.Errors) > 0 || calls.Load() != 1 {
		t.Fatalf("attach: %+v %v applies=%d", result, err, calls.Load())
	}
	if !strings.Contains((*configs)[0], record.Email) {
		t.Fatal("attached identity missing")
	}
	detached, _, err := clients.BulkDetach(inbounds, []string{record.Email}, []int{first.Id, second.Id})
	if err != nil || len(detached.Errors) > 0 || calls.Load() != 2 {
		t.Fatalf("detach: %+v %v applies=%d", detached, err, calls.Load())
	}
	if strings.Contains((*configs)[1], record.Email) {
		t.Fatal("detached identity survived")
	}
	snell := mkInbound(t, 23045, model.Snell, `{"version":6,"psk":"test-psk-12345678","clients":[]}`)
	if _, err := inbounds.DelInbound(snell.Id); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("Snell removal did not queue core configuration")
	}
}

func TestHiddifyInvalidDesiredConfigPreservesRunningProcess(t *testing.T) {
	svc, starts, polls := stage5ProcessFixture(t)
	previous := svc.process().AppliedConfig()
	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"route":{"final":"missing"}}`); err != nil {
		t.Fatal(err)
	}
	svc.SetToNeedRestart()
	svc.ApplyPendingRestart(context.Background())
	if !svc.IsRunning() || !svc.restartFlag().Load() || stage5Starts(t, starts) != 1 || polls.Load() != 0 {
		t.Fatal("invalid desired config stopped core or consumed stats")
	}
	if data, _ := os.ReadFile(svc.ProcessConfigPath()); string(data) != string(previous) {
		t.Fatal("invalid desired config was published")
	}
}

func TestHiddifyBinaryRejectDoesNotConsumeStats(t *testing.T) {
	svc, starts, polls := stage5ProcessFixture(t)
	previous := svc.process().AppliedConfig()
	ib := seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none"}`, `{}`)
	if err := database.GetDB().Model(ib).Update("tag", "stage-audit-reject").Error; err != nil {
		t.Fatal(err)
	}
	svc.SetToNeedRestart()
	svc.ApplyPendingRestart(context.Background())
	if !svc.IsRunning() || !svc.restartFlag().Load() || stage5Starts(t, starts) != 1 || polls.Load() != 0 {
		t.Fatal("binary rejection consumed stats or restarted core")
	}
	if data, _ := os.ReadFile(svc.ProcessConfigPath()); string(data) != string(previous) {
		t.Fatal("rejected config published")
	}
	if err := svc.Restart(context.Background()); err == nil || polls.Load() != 0 {
		t.Fatal("manual restart consumed stats before validation")
	}
}

func TestHiddifyBatchChangesDuringCandidateValidation(t *testing.T) {
	svc, starts, polls := stage5ProcessFixture(t)
	previous := svc.process().AppliedConfig()
	ib := seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none"}`, `{}`)
	dir := filepath.Dir(svc.BinaryPath())
	hold, checking := filepath.Join(dir, "hold-check"), filepath.Join(dir, "checking")
	if err := os.WriteFile(hold, nil, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	svc.SetToNeedRestart()
	go func() { defer close(done); svc.ApplyPendingRestart(context.Background()) }()
	t.Cleanup(func() { _ = os.Remove(hold); <-done })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(checking); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("candidate did not reach validation")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Finish before validation returns: Active alone cannot detect this edit.
	finish := beginLocalConfigBatch()
	if err := database.GetDB().Model(ib).Update("tag", "stage-audit-final").Error; err != nil {
		finish()
		t.Fatal(err)
	}
	svc.SetToNeedRestart()
	finish()
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	<-done
	if !svc.restartFlag().Load() || stage5Starts(t, starts) != 1 || polls.Load() != 0 {
		t.Fatal("intermediate snapshot was applied")
	}
	if data, _ := os.ReadFile(svc.ProcessConfigPath()); string(data) != string(previous) {
		t.Fatal("stale candidate published")
	}
	svc.ApplyPendingRestart(context.Background())
	if svc.restartFlag().Load() || stage5Starts(t, starts) != 2 || !strings.Contains(string(svc.process().AppliedConfig()), "stage-audit-final") {
		t.Fatal("final configuration did not converge")
	}
}

func TestHiddifyCancelledPublicationPreservesFile(t *testing.T) {
	svc, _, _ := stage5ProcessFixture(t)
	previous := svc.process().AppliedConfig()
	seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none"}`, `{}`)
	candidate, err := svc.prepareConfigCandidate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := candidate.publish(ctx); err != context.Canceled {
		t.Fatalf("publish: %v", err)
	}
	if data, _ := os.ReadFile(svc.ProcessConfigPath()); string(data) != string(previous) {
		t.Fatal("cancelled candidate published")
	}
}

func TestHiddifyWatchdogRespectsStoppedAndDeselectedCore(t *testing.T) {
	svc, starts, _ := stage5ProcessFixture(t)
	if err := svc.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecoverIfCrashed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if svc.IsRunning() || stage5Starts(t, starts) != 1 {
		t.Fatal("watchdog undid manual stop")
	}
	svc.stoppedFlag().Store(false) // A previously observed crash.
	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "coreType").Update("value", CoreTypeXray).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RecoverIfCrashed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if svc.IsRunning() || stage5Starts(t, starts) != 1 {
		t.Fatal("watchdog restarted deselected core")
	}
}

func TestHiddifyPreviewDoesNotRequireExternalOutboundBinary(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	raw := `{"outbounds":[{"tag":"direct","protocol":"freedom"},{"tag":"preview-openflux","protocol":"openflux","settings":{"secret":"` + strings.Repeat("a", 64) + `","context":"preview","transports":[{"type":"direct","dial":"example.com:443","priority":100}]}}]}`
	if err := singBoxSettingService.SetXrayConfigTemplate(raw); err != nil {
		t.Fatal(err)
	}
	cfg, err := NativeCore(CoreTypeHiddify).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ManagedOutbounds) != 1 {
		t.Fatal("managed outbound plan was lost")
	}
	cfg2, err := NativeCore(CoreTypeHiddify).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	first, _ := cfg.Marshal()
	second, _ := cfg2.Marshal()
	if string(first) != string(second) {
		t.Fatal("preview changed bridge identity")
	}
	checkGeneratedHiddifyConfig(t, cfg)
	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"outbounds":[{"type":"direct","tag":"direct"}]}`); err != nil {
		t.Fatal(err)
	}
	overridden, err := NativeCore(CoreTypeHiddify).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(overridden.ManagedOutbounds) != 0 {
		t.Fatal("template removed bridge but kept a sidecar startup plan")
	}

}

func TestHiddifyCoreFailureRollsBackExternalOutbound(t *testing.T) {
	svc, starts, _ := stage5ProcessFixture(t)
	dir := filepath.Dir(svc.BinaryPath())
	if err := os.WriteFile(filepath.Join(dir, "openflux"), []byte("#!/bin/sh\nexec sleep 60\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { externalvpn.KeepOutbounds(nil) })
	template := func(context string) string {
		return `{"outbounds":[{"tag":"direct","protocol":"freedom"},{"tag":"audit-openflux","protocol":"openflux","settings":{"secret":"` + strings.Repeat("a", 64) + `","context":"` + context + `","transports":[{"type":"direct","dial":"example.com:443","priority":100}]}}]}`
	}
	if err := singBoxSettingService.SetXrayConfigTemplate(template("stable")); err != nil {
		t.Fatal(err)
	}
	svc.SetToNeedRestart()
	svc.ApplyPendingRestart(context.Background())
	if svc.restartFlag().Load() || stage5Starts(t, starts) != 2 {
		t.Fatal("valid sidecar configuration did not apply")
	}
	configs, err := filepath.Glob(filepath.Join(dir, "externalvpn", "outbound-*", "client.conf"))
	if err != nil || len(configs) != 1 {
		t.Fatalf("sidecar config: %v %v", configs, err)
	}
	// Only the sidecar options change; bridge/native JSON remains identical.
	if err := singBoxSettingService.SetXrayConfigTemplate(template("changed")); err != nil {
		t.Fatal(err)
	}
	unchanged := svc.process().AppliedConfig()
	svc.SetToNeedRestart()
	svc.ApplyPendingRestart(context.Background())
	if svc.restartFlag().Load() || stage5Starts(t, starts) != 3 || string(unchanged) != string(svc.process().AppliedConfig()) {
		t.Fatal("sidecar-only update did not apply through stable bridge")
	}
	stable := svc.process().AppliedConfig()
	before, err := os.ReadFile(configs[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "SessionContext = changed") {
		t.Fatal("native JSON no-op suppressed changed sidecar settings")
	}
	ib := seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none"}`, `{}`)
	if err := database.GetDB().Model(ib).Update("tag", "stage5-fail").Error; err != nil {
		t.Fatal(err)
	}
	if err := singBoxSettingService.SetXrayConfigTemplate(template("candidate")); err != nil {
		t.Fatal(err)
	}
	svc.SetToNeedRestart()
	svc.ApplyPendingRestart(context.Background())
	after, err := os.ReadFile(configs[0])
	if err != nil {
		t.Fatal(err)
	}
	if !svc.IsRunning() || !svc.restartFlag().Load() || string(stable) != string(svc.process().AppliedConfig()) || string(before) != string(after) {
		t.Fatal("failed core candidate displaced stable sidecar or configuration")
	}
}
