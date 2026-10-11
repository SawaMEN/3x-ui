//go:build linux

package coremigration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type trial struct {
	startErr      error
	running       bool
	starts, stops int
	check         func()
}

func (p *trial) Version(context.Context) (string, error) { return hiddify.TargetVersion, nil }
func (p *trial) Validate(context.Context) error          { return nil }
func (p *trial) Start(context.Context) error {
	p.starts++
	if p.check != nil {
		p.check()
	}
	p.running = p.startErr == nil
	return p.startErr
}
func (p *trial) Stop() error     { p.stops++; p.running = false; return nil }
func (p *trial) IsRunning() bool { return p.running }

func fixture(t *testing.T) (runner, *trial, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", filepath.Join(dir, "db"))
	t.Setenv("XUI_BIN_FOLDER", filepath.Join(dir, "bin"))
	t.Setenv("XUI_DB_TYPE", "sqlite")
	if err := os.MkdirAll(config.GetBinFolderPath(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	db := database.GetDB()
	ib := model.Inbound{Enable: true, Tag: "migration-vless", Protocol: model.VLESS, Port: 45009, Settings: `{"decryption":"none"}`, StreamSettings: `{"network":"xhttp","xhttpSettings":{"path":"/existing","mode":"stream-up"}}`}
	client := model.ClientRecord{Email: "migration-user", UUID: "11111111-1111-4111-8111-111111111111", Password: "preserve-password", SubID: "preserve-subscription", Enable: true}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	rows := []any{&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}, &coremodel.ClientTraffic{Email: client.Email, InboundId: ib.Id, Enable: true, Up: 12345, Down: 67890, Total: 900000}, &model.Node{Name: "encrypted-node", Address: "node.example", Port: 443, ApiToken: "opaque-encrypted-token"}, &model.ApiToken{Name: "existing-admin", Token: "existing-token-hash", Enabled: true, Scope: model.ApiScopeAdmin, CreatedAt: time.Now().Unix()}, &model.CoreTrafficCheckpoint{Name: "user>>>migration-user>>>traffic>>>uplink", Epoch: "existing-epoch", Value: 12345}}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&model.User{}).Where("id = 1").Updates(map[string]any{"username": "existing-admin", "password": "existing-password-hash"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []model.Setting{{Key: "coreType", Value: "xray"}, {Key: "smtpPassword", Value: "existing-secret"}, {Key: "subURI", Value: "https://subscription.example/existing"}} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
version) printf 'hiddify-core version %s\nTags: with_xui_panel,with_v2ray_api,with_awg,with_quic,with_wireguard,with_clash_api,with_utls,with_masque,with_gvisor,with_naive_outbound,with_purego,tfogo_checklinkname0\nRevision: %s\n' ;;
check) exit 0 ;;
*) exit 1 ;;
esac
`, hiddify.TargetVersion, hiddify.TargetRevision)
	if err := os.WriteFile(hiddify.GetBinaryPath(), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hiddify.GetConfigPath(), []byte("previous-native-config"), 0600); err != nil {
		t.Fatal(err)
	}
	p := &trial{}
	r := defaults()
	r.stopped = func() error { return nil }
	r.newRuntime = func(string) runtime { return p }
	r.files = func() []string {
		return []string{config.GetDBPath(), config.GetDBPath() + "-wal", config.GetDBPath() + "-shm", hiddify.GetConfigPath()}
	}
	r.gateway = func(context.Context, string) (bool, func(), error) { return false, func() {}, nil }
	backup := filepath.Join(dir, "snapshot")
	return r, p, backup
}
func preserved(t *testing.T) [32]byte {
	t.Helper()
	db := database.GetDB()
	capture := map[string]any{}
	for _, table := range []string{"users", "clients", "client_inbounds", "client_traffics", "nodes", "api_tokens", "core_traffic_checkpoints"} {
		var rows []map[string]any
		if err := db.Table(table).Find(&rows).Error; err != nil {
			t.Fatalf("table %s: %v", table, err)
		}
		sort.Slice(rows, func(i, j int) bool {
			a, _ := json.Marshal(rows[i])
			b, _ := json.Marshal(rows[j])
			return string(a) < string(b)
		})
		capture[table] = rows
	}
	var settings []model.Setting
	if err := db.Where("key NOT IN ?", []string{"coreType", "ownerCoreMigration"}).Order("id").Find(&settings).Error; err != nil {
		t.Fatal(err)
	}
	capture["settings"] = settings
	data, err := json.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}
func reopen(t *testing.T) {
	t.Helper()
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
}
func TestMigrationPreservesInstalledDataAndCommitsAfterTrial(t *testing.T) {
	r, p, dir := fixture(t)
	before := preserved(t)
	_ = database.CloseDB()
	p.check = func() {
		kind, err := (&service.SettingService{}).GetCoreType()
		if err != nil || kind != "xray" {
			t.Fatal("selected core changed before successful launch")
		}
	}
	if err := r.run(context.Background(), Options{BackupDir: dir}); err != nil {
		t.Fatal(err)
	}
	if p.starts != 1 || p.stops != 1 || p.running {
		t.Fatal("trial listeners were not released")
	}
	reopen(t)
	if before != preserved(t) {
		t.Fatal("credentials, subscriptions, nodes or traffic changed")
	}
	kind, err := (&service.SettingService{}).GetCoreType()
	if err != nil || kind != "sing-box" {
		t.Fatalf("selected core %q: %v", kind, err)
	}
	var marker model.Setting
	if err := database.GetDB().Where("key = ?", "ownerCoreMigration").First(&marker).Error; err != nil || marker.Value != hiddify.TargetRevision {
		t.Fatal("migration revision not recorded")
	}
	data, _ := os.ReadFile(hiddify.GetConfigPath())
	if !strings.Contains(string(data), "migration-user") || !strings.Contains(string(data), "xhttp") {
		t.Fatal("managed config lost owner extensions")
	}
	_ = database.CloseDB()
	if err := r.run(context.Background(), Options{BackupDir: dir}); err != nil || p.starts != 1 {
		t.Fatal("completed migration was not idempotent", err)
	}
	if err := r.run(context.Background(), Options{BackupDir: dir, Restore: true}); err != nil {
		t.Fatal(err)
	}
	reopen(t)
	kind, _ = (&service.SettingService{}).GetCoreType()
	if kind != "xray" || before != preserved(t) {
		t.Fatal("explicit recovery lost previous data")
	}
	old, _ := os.ReadFile(hiddify.GetConfigPath())
	if string(old) != "previous-native-config" {
		t.Fatal("native config not recovered")
	}
}
func TestMigrationStartupFailureRestoresExactDatabaseAndConfig(t *testing.T) {
	r, p, dir := fixture(t)
	before := preserved(t)
	_ = database.CloseDB()
	original, _ := os.ReadFile(config.GetDBPath())
	p.startErr = errors.New("listener bind failure")
	err := r.run(context.Background(), Options{BackupDir: dir})
	if err == nil || !strings.Contains(err.Error(), "listener bind failure") {
		t.Fatal("startup failure hidden", err)
	}
	after, _ := os.ReadFile(config.GetDBPath())
	if sha256.Sum256(original) != sha256.Sum256(after) {
		t.Fatal("rollback database differs from offline snapshot")
	}
	reopen(t)
	if preserved(t) != before {
		t.Fatal("rollback changed data")
	}
	kind, _ := (&service.SettingService{}).GetCoreType()
	if kind != "xray" {
		t.Fatal("rollback retained sing-box")
	}
	old, _ := os.ReadFile(hiddify.GetConfigPath())
	if string(old) != "previous-native-config" || p.stops != 1 {
		t.Fatal("rollback left trial or candidate")
	}
	if _, err := os.Stat(pendingPath()); !os.IsNotExist(err) {
		t.Fatal("successful rollback retained pending journal")
	}
}
func TestMigrationRejectsUnsupportedActiveInboundWithoutDisablingUsers(t *testing.T) {
	r, p, dir := fixture(t)
	if err := database.GetDB().Model(&model.Inbound{}).Where("tag = ?", "migration-vless").Update("protocol", model.WireGuard).Error; err != nil {
		t.Fatal(err)
	}
	before := preserved(t)
	_ = database.CloseDB()
	err := r.run(context.Background(), Options{BackupDir: dir})
	if err == nil || !strings.Contains(err.Error(), "cannot migrate") {
		t.Fatal("incompatible inbound accepted", err)
	}
	if p.starts != 0 {
		t.Fatal("incompatible installation started trial")
	}
	reopen(t)
	if preserved(t) != before {
		t.Fatal("preflight disabled or changed clients")
	}
}
func TestMigrationBackupOnlyInterruptedJournalAndCorruption(t *testing.T) {
	r, p, dir := fixture(t)
	_ = database.CloseDB()
	if err := r.run(context.Background(), Options{BackupDir: dir, BackupOnly: true}); err != nil {
		t.Fatal(err)
	}
	if p.starts != 0 {
		t.Fatal("backup starts core")
	}
	m, err := r.loadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.Phase = "applying"
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(pendingPath(), []byte(dir), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.run(context.Background(), Options{BackupDir: dir}); err == nil {
		t.Fatal("interrupted migration silently resumed")
	}
	if err := r.run(context.Background(), Options{BackupDir: dir, Restore: true}); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, m.Files[0].Backup)
	if err := os.WriteFile(backup, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(config.GetDBPath())
	if err := r.run(context.Background(), Options{BackupDir: dir, Restore: true}); err == nil {
		t.Fatal("corrupt snapshot accepted")
	}
	current, _ := os.ReadFile(config.GetDBPath())
	if sha256.Sum256(original) != sha256.Sum256(current) {
		t.Fatal("corrupt recovery changed live DB")
	}
}
func TestMigrationPanelLeaseExcludesConcurrentOperation(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	release, err := AcquirePanelLease()
	if err != nil {
		t.Fatal(err)
	}
	if second, err := AcquirePanelLease(); err == nil {
		second()
		t.Fatal("concurrent panel/migration accepted")
	}
	release()
	again, err := AcquirePanelLease()
	if err != nil {
		t.Fatal(err)
	}
	again()
}

func TestMigrationUsesSupervisedOwnerServerTrial(t *testing.T) {
	r, _, dir := fixture(t)
	before := preserved(t)
	_ = database.CloseDB()
	script, err := os.ReadFile(hiddify.GetBinaryPath())
	if err != nil {
		t.Fatal(err)
	}
	script = []byte(strings.Replace(string(script), "check) exit 0 ;;", "check) exit 0 ;;\nsrun) trap 'exit 0' TERM INT; while :; do sleep 0.05; done ;;", 1))
	if err := os.WriteFile(hiddify.GetBinaryPath(), script, 0700); err != nil {
		t.Fatal(err)
	}
	r.newRuntime = func(path string) runtime { return hiddify.NewProcess(path, true) }
	if err := r.run(context.Background(), Options{BackupDir: dir}); err != nil {
		t.Fatal(err)
	}
	process := hiddify.NewProcess(hiddify.GetConfigPath(), false)
	if process.IsRunning() {
		t.Fatal("isolated trial was left on panel listener")
	}
	reopen(t)
	if preserved(t) != before {
		t.Fatal("server trial changed installed data")
	}
}

func TestMigrationNewKernelCheckFailurePreservesExistingConfig(t *testing.T) {
	r, p, dir := fixture(t)
	before := preserved(t)
	_ = database.CloseDB()
	script, err := os.ReadFile(hiddify.GetBinaryPath())
	if err != nil {
		t.Fatal(err)
	}
	script = []byte(strings.Replace(string(script), "check) exit 0 ;;", "check) exit 7 ;;", 1))
	if err := os.WriteFile(hiddify.GetBinaryPath(), script, 0700); err != nil {
		t.Fatal(err)
	}
	if err := r.run(context.Background(), Options{BackupDir: dir}); err == nil {
		t.Fatal("kernel candidate rejection hidden")
	}
	if p.starts != 0 {
		t.Fatal("rejected candidate was started")
	}
	data, _ := os.ReadFile(hiddify.GetConfigPath())
	if string(data) != "previous-native-config" {
		t.Fatal("invalid candidate replaced native config")
	}
	reopen(t)
	if before != preserved(t) {
		t.Fatal("candidate rejection changed installed data")
	}
}

func TestMigrationRunsPinnedBinaryConstructorWhenAvailable(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_CONFIG_CHECK_BINARY for the actual owner constructor")
	}
	r, _, dir := fixture(t)
	_ = database.CloseDB()
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hiddify.GetBinaryPath(), data, 0700); err != nil {
		t.Fatal(err)
	}
	// The actual check -c validates generated config; the fake trial leaves privileged
	// netlink/TUN construction to host acceptance, which this test does not claim.
	if err := r.run(context.Background(), Options{BackupDir: dir}); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationConvertsLegacyInlineUsersWithoutChangingCredentialsOrTraffic(t *testing.T) {
	r, _, dir := fixture(t)
	db := database.GetDB()
	var ib model.Inbound
	if err := db.Where("tag = ?", "migration-vless").First(&ib).Error; err != nil {
		t.Fatal(err)
	}
	legacy := `{"decryption":"none","clients":[{"id":"11111111-1111-4111-8111-111111111111","email":"migration-user","password":"preserve-password","subId":"preserve-subscription","enable":true}]}`
	if err := db.Model(&ib).Update("settings", legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("inbound_id = ?", ib.Id).Delete(&model.ClientInbound{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("email = ?", "migration-user").Delete(&model.ClientRecord{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("seeder_name = ?", "ClientsTable").Delete(&model.HistoryOfSeeders{}).Error; err != nil {
		t.Fatal(err)
	}
	_ = database.CloseDB()
	if err := r.run(context.Background(), Options{BackupDir: dir}); err != nil {
		t.Fatal(err)
	}
	reopen(t)
	var client model.ClientRecord
	if err := database.GetDB().Where("email = ?", "migration-user").First(&client).Error; err != nil {
		t.Fatal(err)
	}
	if client.UUID != "11111111-1111-4111-8111-111111111111" || client.Password != "preserve-password" || client.SubID != "preserve-subscription" || !client.Enable {
		t.Fatal("legacy client identity changed")
	}
	var traffic coremodel.ClientTraffic
	if err := database.GetDB().Where("email = ?", "migration-user").First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Up != 12345 || traffic.Down != 67890 || traffic.Total != 900000 {
		t.Fatal("legacy traffic lost")
	}
	var links int64
	if err := database.GetDB().Model(&model.ClientInbound{}).Where("client_id = ? AND inbound_id = ?", client.Id, ib.Id).Count(&links).Error; err != nil || links != 1 {
		t.Fatal("legacy inbound attachment lost")
	}
}

type unkillable struct{ trial }

func (p *unkillable) Stop() error { return errors.New("cannot stop trial") }
func TestMigrationNeverRestoresWhileTrialStillOwnsListeners(t *testing.T) {
	r, _, dir := fixture(t)
	_ = database.CloseDB()
	p := &unkillable{}
	r.newRuntime = func(string) runtime { return p }
	err := r.run(context.Background(), Options{BackupDir: dir})
	if err == nil || !strings.Contains(err.Error(), "still owns listeners") {
		t.Fatal("unsafe rollback accepted", err)
	}
	m, err := r.loadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Phase != "applying" {
		t.Fatal("blocked recovery did not retain journal")
	}
	if _, err := os.Stat(pendingPath()); err != nil {
		t.Fatal("blocked recovery lost snapshot location")
	}
}

func TestPostgresDatabaseIdentityExcludesPasswordAndRejectsDifferentDatabase(t *testing.T) {
	t.Setenv("XUI_DB_TYPE", "postgres")
	t.Setenv("XUI_DB_DSN", "postgres://panel:old-password@localhost:5432/panel")
	old, err := databaseIdentity()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XUI_DB_DSN", "postgres://panel:rotated-password@localhost:5432/panel")
	rotated, err := databaseIdentity()
	if err != nil || rotated != old {
		t.Fatal("password rotation broke snapshot identity")
	}
	t.Setenv("XUI_DB_DSN", "postgres://panel:old-password@localhost:5432/another")
	another, err := databaseIdentity()
	if err != nil || another == old {
		t.Fatal("snapshot can target another database")
	}
}

func TestMigrationRejectsRestoreFromAnotherPendingOperationBeforeWriting(t *testing.T) {
	r, _, dir := fixture(t)
	_ = database.CloseDB()
	if err := r.run(context.Background(), Options{BackupDir: dir, BackupOnly: true}); err != nil {
		t.Fatal(err)
	}
	before, _ := hashFile(config.GetDBPath())
	other := filepath.Join(filepath.Dir(dir), "another-snapshot")
	if err := atomicWrite(pendingPath(), []byte(other), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.run(context.Background(), Options{BackupDir: dir, Restore: true}); err == nil {
		t.Fatal("restore overwrote another pending migration")
	}
	after, _ := hashFile(config.GetDBPath())
	if before != after {
		t.Fatal("foreign recovery changed database")
	}
}
func TestMigrationRejectsMissingDatabaseInsteadOfCreatingAnEmptyPanel(t *testing.T) {
	r, _, dir := fixture(t)
	_ = database.CloseDB()
	if err := os.Remove(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	if err := r.run(context.Background(), Options{BackupDir: dir}); err == nil || !strings.Contains(err.Error(), "database is unavailable") {
		t.Fatal("empty installation was created", err)
	}
	if _, err := os.Stat(config.GetDBPath()); !os.IsNotExist(err) {
		t.Fatal("missing database recreated")
	}
}
func TestMigrationCLIRejectsConflictingModesBeforeInitialization(t *testing.T) {
	called := false
	for _, args := range [][]string{{"--check-binary", "--restore"}, {"--verify-running", "--backup-dir", "/unrelated"}, {"--backup-only", "--restore"}} {
		if err := CLIWithInitializer(args, func() error { called = true; return nil }); err == nil {
			t.Fatal("conflicting CLI mode accepted")
		}
	}
	if called {
		t.Fatal("invalid arguments initialized installation state")
	}
}

func TestMigrationPreservesGatewayDesiredStateAcrossSuccessAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("startup-failure-%v", fail), func(t *testing.T) {
			r, p, dir := fixture(t)
			state := filepath.Join(filepath.Dir(dir), "gateway.env")
			original := "NETWORK_ENABLED=1\nLAN_IFACE=eth1\n"
			if err := os.WriteFile(state, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			oldFiles := r.files
			r.files = func() []string { return append(oldFiles(), state) }
			r.statePath = func() string { return state }
			r.gateway = func(context.Context, string) (bool, func(), error) {
				if err := os.WriteFile(state, []byte("NETWORK_ENABLED=0\nLAN_IFACE=eth1\n"), 0600); err != nil {
					return false, nil, err
				}
				err := (&service.SettingService{}).SetSingBoxConfigTemplate(`{"inbounds":[{"type":"tproxy","tag":"in-tproxy","listen":"0.0.0.0","listen_port":52345}]}`)
				return true, func() {}, err
			}
			if fail {
				p.startErr = errors.New("trial failure")
			}
			_ = database.CloseDB()
			err := r.run(context.Background(), Options{BackupDir: dir})
			if (err != nil) != fail {
				t.Fatal("unexpected migration result", err)
			}
			data, err := os.ReadFile(state)
			if err != nil || string(data) != original {
				t.Fatal("Gateway desired state was lost")
			}
			reopen(t)
			raw, err := (&service.SettingService{}).GetSingBoxConfigTemplate()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(raw, "in-tproxy") == fail {
				t.Fatal("Gateway template did not follow commit/rollback")
			}
		})
	}
}

func TestMigrationPendingJournalBlocksPanelStartup(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	if err := CheckPanelStartup(); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(pendingPath(), []byte("/private/recovery"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckPanelStartup(); err == nil {
		t.Fatal("panel accepted unfinished migration")
	}
	if err := os.Remove(pendingPath()); err != nil {
		t.Fatal(err)
	}
	if err := CheckPanelStartup(); err != nil {
		t.Fatal(err)
	}
}
