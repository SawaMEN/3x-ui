// Package coremigration upgrades an offline installation without changing identities,
// credentials or traffic totals. Its durable snapshot precedes even InitDB.
package coremigration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/gateway"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

type Options struct {
	BackupDir           string
	BackupOnly, Restore bool
}
type runtime interface {
	Version(context.Context) (string, error)
	Validate(context.Context) error
	Start(context.Context) error
	Stop() error
	IsRunning() bool
}
type runner struct {
	postgresFingerprint func(context.Context) (string, error)
	newRuntime          func(string) runtime
	stopped             func() error
	files               func() []string
	statePath           func() string
	gateway             func(context.Context, string) (bool, func(), error)
}

func defaults() runner {
	return runner{postgresFingerprint: service.CoreMigrationPostgresFingerprint, newRuntime: func(path string) runtime { return hiddify.NewProcess(path, true) }, stopped: assertStopped, files: migrationFiles, statePath: service.GatewayMigrationStatePath, gateway: prepareGateway}
}

func Run(ctx context.Context, opts Options) error {
	release, err := AcquirePanelLease()
	if err != nil {
		return err
	}
	defer release()
	return defaults().run(ctx, opts)
}

func migrationFiles() []string {
	files := []string{config.GetDBPath(), config.GetDBPath() + "-wal", config.GetDBPath() + "-shm", config.GetDBPath() + "-journal", xray.GetConfigPath(), singbox.GetConfigPath(), hiddify.GetConfigPath()}
	files = append(files, gateway.MigrationFiles()...)
	files = append(files, service.GatewayMigrationFiles()...)
	return files
}

func pendingPath() string {
	return filepath.Join(config.GetDBFolderPath(), "core-migration-pending.json")
}

func (r runner) loadManifest(dir string) (*manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	identity, err := databaseIdentity()
	if err != nil {
		return nil, err
	}
	if m.Schema != 1 || m.CoreType != "sing-box" || m.DBKind != config.GetDBKind() || m.DatabaseID != identity {
		return nil, fmt.Errorf("snapshot does not match this installation")
	}
	allowed := map[string]bool{}
	for _, p := range r.files() {
		a, _ := filepath.Abs(p)
		allowed[a] = true
	}
	for _, f := range m.Files {
		if !allowed[f.Path] {
			return nil, fmt.Errorf("snapshot path is outside this installation: %s", f.Path)
		}
	}
	unique := make([]savedFile, 0, len(m.Files))
	seen := map[string]savedFile{}
	for _, f := range m.Files {
		if previous, exists := seen[f.Path]; exists {
			if previous.Exists != f.Exists || previous.SHA256 != f.SHA256 || previous.Mode != f.Mode {
				return nil, fmt.Errorf("conflicting duplicate snapshot path: %s", f.Path)
			}
			continue
		}
		seen[f.Path] = f
		unique = append(unique, f)
	}
	if len(unique) != len(allowed) {
		return nil, fmt.Errorf("incomplete migration snapshot")
	}
	if err := verifyPostgres(dir, &m); err != nil {
		return nil, err
	}
	if err := verifyFiles(dir, m.Files); err != nil {
		return nil, err
	}
	// Legacy snapshots listed the shared sing-box/hiddify config twice.
	// Verify every copy above before restoring each destination just once.
	m.Files = unique
	return &m, nil
}

func (r runner) run(ctx context.Context, o Options) (result error) {
	if err := r.stopped(); err != nil {
		return err
	}
	dir, err := filepath.Abs(o.BackupDir)
	if err != nil {
		return err
	}
	if o.BackupDir == "" {
		return errors.New("--backup-dir is required")
	}
	if data, err := os.ReadFile(pendingPath()); err == nil && string(data) != dir {
		return fmt.Errorf("unfinished migration; restore snapshot %s before continuing", string(data))
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if o.Restore {
		m, err := r.loadManifest(dir)
		if err != nil {
			return err
		}
		return restore(ctx, dir, m)
	}
	m, err := r.loadManifest(dir)
	if os.IsNotExist(err) {
		if config.GetDBKind() == "sqlite" {
			if _, err := os.Stat(config.GetDBPath()); err != nil {
				return fmt.Errorf("existing installation database is unavailable: %w", err)
			}
		}
		if err := os.Mkdir(dir, 0o700); err != nil {
			return fmt.Errorf("create private migration snapshot: %w", err)
		}
		identity, err := databaseIdentity()
		if err != nil {
			return err
		}
		m = &manifest{DatabaseID: identity, Schema: 1, Revision: hiddify.TargetRevision, CoreType: "sing-box", DBKind: config.GetDBKind(), Phase: "backed-up"}
		if m.DBKind == "postgres" {
			if m.PostgresTables, err = service.CoreMigrationPostgresTables(ctx); err != nil {
				return err
			}
			tables := map[string]bool{}
			for _, name := range m.PostgresTables {
				tables[name] = true
			}
			if !tables["settings"] || !tables["users"] || !tables["inbounds"] {
				return errors.New("PostgreSQL database does not contain an installed panel")
			}
			if m.PostgresFingerprint, err = r.postgresFingerprint(ctx); err != nil {
				return err
			}
			if err := service.CoreMigrationPostgresSnapshot(ctx, filepath.Join(dir, "database.dump")); err != nil {
				return err
			}
		}
		if m.DBKind == "postgres" {
			if m.PostgresSHA256, err = hashFile(filepath.Join(dir, "database.dump")); err != nil {
				return err
			}
		}
		if m.Files, err = snapshotFiles(dir, r.files()); err != nil {
			return err
		}
		if err := saveManifest(dir, m); err != nil {
			return err
		}
		if m, err = r.loadManifest(dir); err != nil {
			return fmt.Errorf("verify created migration snapshot: %w", err)
		}
	} else if err != nil {
		return err
	}
	if m.Revision != hiddify.TargetRevision {
		return fmt.Errorf("snapshot was prepared for a different owner core revision")
	}
	if m.Phase == "committed" {
		if _, err := r.newRuntime(hiddify.GetConfigPath()).Version(ctx); err != nil {
			return err
		}
		current, err := completed(ctx)
		if err != nil {
			return err
		}
		if !current {
			return errors.New("committed snapshot does not describe the current core; use a new backup directory")
		}
		return nil
	}
	if m.Phase != "backed-up" {
		return fmt.Errorf("snapshot phase %s requires --restore or a new backup directory", m.Phase)
	}
	if m.DBKind == "postgres" {
		current, err := r.postgresFingerprint(ctx)
		if err != nil {
			return err
		}
		if m.PostgresFingerprint == "" || current != m.PostgresFingerprint {
			return errors.New("PostgreSQL database changed since backup; use a new backup directory")
		}
	}
	if err := verifySQLiteSource(m); err != nil {
		return err
	}
	if o.BackupOnly {
		return nil
	}
	if err := atomicWrite(pendingPath(), []byte(dir), 0o600); err != nil {
		return err
	}
	m.Phase = "applying"
	if err := saveManifest(dir, m); err != nil {
		return err
	}
	unsafeRecovery := false
	defer func() {
		if err := database.CloseDB(); err != nil {
			unsafeRecovery = true
			result = errors.Join(result, fmt.Errorf("close database before recovery: %w", err))
		}
		if result != nil && !unsafeRecovery {
			recovery, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			result = errors.Join(result, restore(recovery, dir, m))
		}
	}()
	// Check the pinned executable before schema migration or listener changes.
	rt := r.newRuntime(hiddify.GetConfigPath())
	defer func() {
		if rt.IsRunning() {
			result = errors.Join(result, rt.Stop())
			if rt.IsRunning() {
				unsafeRecovery = true
				result = errors.Join(result, errors.New("trial core still owns listeners; stop it before restoring the retained snapshot"))
			}
		}
	}()
	if _, err := rt.Version(ctx); err != nil {
		return err
	}
	if err := database.InitDB(config.GetDBPath()); err != nil {
		return err
	}
	settings := &service.SettingService{}
	old, err := settings.GetCoreType()
	if err != nil {
		return err
	}
	// Before core selection existed, an absent value meant Xray. Preserve that
	// source interpretation even though new panel settings default to sing-box.
	var storedCount int64
	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "coreType").Count(&storedCount).Error; err != nil {
		return err
	}
	if storedCount == 0 {
		old = service.CoreTypeXray
	}
	var inbounds []model.Inbound
	if err := database.GetDB().Where("enable = ? AND node_id IS NULL", true).Find(&inbounds).Error; err != nil {
		return err
	}
	for _, ib := range inbounds {
		if !service.CoreSupportsInboundProtocol(service.CoreTypeSingBox, ib.Protocol) {
			return fmt.Errorf("enabled inbound %q (%s) cannot migrate to sing-box; no users were disabled", ib.Tag, ib.Protocol)
		}
	}
	configured, release, err := r.gateway(ctx, old)
	if err != nil {
		return err
	}
	defer release()
	svc := service.NativeCore(service.CoreTypeSingBox)
	if err := svc.WriteConfig(); err != nil {
		return err
	}
	if err := rt.Validate(ctx); err != nil {
		return err
	}
	// Server mode performs the actual constructor/bind check, not just JSON parsing.
	// Always stop this isolated trial before handing listeners to the panel.
	if err := rt.Start(ctx); err != nil {
		_ = rt.Stop()
		return err
	}
	if !rt.IsRunning() {
		_ = rt.Stop()
		return errors.New("owner sing-box is not running after trial start")
	}
	if err := rt.Stop(); err != nil {
		return err
	}
	if configured {
		// Keep the original desired Gateway state. Interception stays paused until
		// the restarted panel has the successfully tested listener.
		for _, f := range m.Files {
			if f.Path == r.statePath() {
				if err := restoreFiles(dir, []savedFile{f}); err != nil {
					return err
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		for _, row := range []model.Setting{{Key: "coreType", Value: "sing-box"}, {Key: "ownerCoreMigration", Value: hiddify.TargetRevision}} {
			changed := tx.Model(&model.Setting{}).Where("key = ?", row.Key).Update("value", row.Value)
			if changed.Error != nil {
				return changed.Error
			}
			if changed.RowsAffected == 0 {
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := database.CloseDB(); err != nil {
		return err
	}
	m.Phase = "committed"
	if err := saveManifest(dir, m); err != nil {
		return err
	}
	return removePending(dir)
}

func restore(ctx context.Context, dir string, m *manifest) error {
	if err := verifyPostgres(dir, m); err != nil {
		return err
	}
	if err := database.CloseDB(); err != nil {
		return err
	}
	if err := verifyFiles(dir, m.Files); err != nil {
		return err
	}
	if m.DBKind == "postgres" {
		if err := service.CoreMigrationPostgresRemoveAddedTables(ctx, m.PostgresTables); err != nil {
			return err
		}
		if err := service.CoreMigrationPostgresRestore(ctx, filepath.Join(dir, "database.dump")); err != nil {
			return err
		}
	}
	if err := restoreFiles(dir, m.Files); err != nil {
		return err
	}
	m.Phase = "rolled-back"
	if err := saveManifest(dir, m); err != nil {
		return err
	}
	return removePending(dir)
}

func removePending(dir string) error {
	data, err := os.ReadFile(pendingPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if string(data) != dir {
		return fmt.Errorf("another migration owns pending journal")
	}
	return os.Remove(pendingPath())
}

func prepareGateway(ctx context.Context, old string) (bool, func(), error) {
	network := &service.GatewayNetworkService{}
	status := network.Status(ctx)
	if status.Error != "" {
		return false, nil, fmt.Errorf("Gateway recovery required: %s", status.Error)
	}
	release, err := gateway.AcquireOperation(ctx)
	if err != nil {
		return false, nil, err
	}
	if status.Configured {
		if err := network.Suspend(ctx); err != nil {
			release()
			return false, nil, err
		}
	}
	if _, err := gateway.MigrateCore(old, service.CoreTypeSingBox); err != nil {
		release()
		return false, nil, err
	}
	return status.Configured, release, nil
}

func databaseIdentity() (string, error) {
	if config.GetDBKind() == "postgres" {
		parsed, err := url.Parse(config.GetDBDSN())
		if err != nil {
			return "", errors.New("invalid PostgreSQL DSN")
		}
		if parsed.Host == "" || parsed.Path == "" {
			return "", errors.New("PostgreSQL DSN must identify a host and database")
		}
		user := ""
		if parsed.User != nil {
			user = parsed.User.Username()
		}
		identity := fmt.Sprintf("postgres:%s:%s:%s", parsed.Host, parsed.Path, user)
		return fmt.Sprintf("%x", sha256.Sum256([]byte(identity))), nil
	}
	path, err := filepath.Abs(config.GetDBPath())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(path))), nil
}
