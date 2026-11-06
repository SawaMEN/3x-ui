package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"

	"gorm.io/gorm"
)

func migrateClientTrafficLastSubFetchColumn() error {
	migrator := db.Migrator()
	if !migrator.HasTable(&coremodel.ClientTraffic{}) || migrator.HasColumn(&coremodel.ClientTraffic{}, "last_sub_fetch") {
		return nil
	}
	return migrator.AddColumn(&coremodel.ClientTraffic{}, "LastSubFetch")
}

func migrateOutboundSubscriptionUserAgentColumn() error {
	migrator := db.Migrator()
	if !migrator.HasTable(&model.OutboundSubscription{}) || migrator.HasColumn(&model.OutboundSubscription{}, "user_agent") {
		return nil
	}
	return migrator.AddColumn(&model.OutboundSubscription{}, "UserAgent")
}

func migrateShadowTLSInnerKeys() error {
	var inbounds []model.Inbound
	if err := db.Model(&model.Inbound{}).Select("id", "settings").Where("protocol = ?", model.ShadowTLS).Find(&inbounds).Error; err != nil {
		return err
	}
	for _, inbound := range inbounds {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(inbound.Settings), &fields); err != nil {
			return fmt.Errorf("ShadowTLS inbound %d settings: %w", inbound.Id, err)
		}
		var key string
		_ = json.Unmarshal(fields[model.ShadowTLSInnerKeyField], &key)
		if model.ValidShadowTLSInnerKey(key) {
			continue
		}
		settings, err := model.EnsureShadowTLSInnerKey(inbound.Settings, "")
		if err != nil {
			return err
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).UpdateColumn("settings", settings).Error; err != nil {
			return err
		}
	}
	return nil
}

func dropLegacyForeignKeys() error {
	if !IsPostgres() {
		return nil
	}
	if err := db.Exec("ALTER TABLE client_traffics DROP CONSTRAINT IF EXISTS fk_inbounds_client_stats").Error; err != nil {
		log.Printf("Error dropping legacy foreign key fk_inbounds_client_stats: %v", err)
		return err
	}
	return nil
}

// dropLegacyInboundPortUnique removes the pre-multi-node UNIQUE on inbounds.port,
// which AutoMigrate never drops and which blocks cross-node port reuse on old SQLite DBs.
func dropLegacyInboundPortUnique() error {
	if IsPostgres() {
		return nil
	}
	autoIndexes, explicitIndexes, err := sqliteUniquePortIndexes()
	if err != nil {
		return err
	}
	for _, name := range explicitIndexes {
		if err := db.Exec(`DROP INDEX IF EXISTS "` + name + `"`).Error; err != nil {
			return err
		}
	}
	if len(autoIndexes) == 0 {
		return nil
	}
	log.Printf("Rebuilding inbounds table to drop the legacy UNIQUE constraint on port")
	return rebuildInboundsWithoutInlineUniquePort()
}

func rebuildInboundsWithoutInlineUniquePort() error {
	return db.Transaction(func(tx *gorm.DB) error {
		var list []sqliteIndexListRow
		if err := tx.Raw(`PRAGMA index_list('inbounds')`).Scan(&list).Error; err != nil {
			return err
		}
		for _, idx := range list {
			if idx.Origin != "c" {
				continue
			}
			if err := tx.Exec(`DROP INDEX IF EXISTS "` + idx.Name + `"`).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`ALTER TABLE inbounds RENAME TO inbounds_legacy_rebuild`).Error; err != nil {
			return err
		}
		if err := tx.Migrator().CreateTable(&model.Inbound{}); err != nil {
			return err
		}
		newCols, err := sqliteTableColumns(tx, "inbounds")
		if err != nil {
			return err
		}
		oldCols, err := sqliteTableColumns(tx, "inbounds_legacy_rebuild")
		if err != nil {
			return err
		}
		oldSet := make(map[string]struct{}, len(oldCols))
		for _, c := range oldCols {
			oldSet[c] = struct{}{}
		}
		shared := make([]string, 0, len(newCols))
		for _, c := range newCols {
			if _, ok := oldSet[c]; ok {
				shared = append(shared, `"`+c+`"`)
			}
		}
		colList := strings.Join(shared, ", ")
		if err := tx.Exec(`INSERT INTO inbounds (` + colList + `) SELECT ` + colList + ` FROM inbounds_legacy_rebuild`).Error; err != nil {
			return err
		}
		return tx.Exec(`DROP TABLE inbounds_legacy_rebuild`).Error
	})
}

// AutoMigrate adds the columns; an older SQLite ALTER TABLE leaves them NULL,
// and a NULL traffic_reset fails every ClientRecord scan, not just the new query.
func migrateClientTrafficResetColumns() error {
	if db.Migrator().HasColumn(&model.ClientRecord{}, "traffic_reset") {
		if err := db.Exec("UPDATE clients SET traffic_reset = 'never' WHERE traffic_reset IS NULL").Error; err != nil {
			return err
		}
	}
	if db.Migrator().HasColumn(&model.ClientRecord{}, "traffic_reset_day") {
		if err := db.Exec("UPDATE clients SET traffic_reset_day = 1 WHERE traffic_reset_day IS NULL").Error; err != nil {
			return err
		}
	}
	return nil
}

// AutoMigrate adds the column; this only backfills the NULLs an older SQLite
// ALTER TABLE leaves behind, so the reaper's predicate never compares to NULL.
func migrateSyncOrphanColumns() error {
	if !db.Migrator().HasColumn(&model.ClientRecord{}, "sync_orphaned_at") {
		return nil
	}
	return db.Exec("UPDATE clients SET sync_orphaned_at = 0 WHERE sync_orphaned_at IS NULL").Error
}

// The client identity checks match emails case-insensitively; without an
// expression index (which no GORM struct tag can declare) they seq-scan.
func migrateNodePanelEdition() error {
	if !db.Migrator().HasColumn(&model.Node{}, "panel_edition") {
		return nil
	}
	return db.Model(&model.Node{}).Where("panel_edition IS NULL OR panel_edition = ?", "").Update("panel_edition", "unknown").Error
}

func migrateClientEmailLowerIndex() error {
	if db.Migrator().HasIndex(&model.ClientRecord{}, "idx_clients_email_lower") {
		return nil
	}
	return db.Exec("CREATE INDEX IF NOT EXISTS idx_clients_email_lower ON clients (LOWER(email))").Error
}

func migrateHostVerifyPeerCertByNameColumn() error {
	if !db.Migrator().HasColumn(&model.Host{}, "verify_peer_cert_by_name") {
		return nil
	}
	if IsPostgres() {

		var dataType string
		if err := db.Raw(
			`SELECT data_type FROM information_schema.columns WHERE table_name = 'hosts' AND column_name = 'verify_peer_cert_by_name'`,
		).Scan(&dataType).Error; err != nil {
			return err
		}
		if dataType != "boolean" {
			return nil
		}
		if err := db.Exec(`ALTER TABLE hosts ALTER COLUMN verify_peer_cert_by_name DROP DEFAULT`).Error; err != nil {
			return err
		}
		return db.Exec(`ALTER TABLE hosts ALTER COLUMN verify_peer_cert_by_name TYPE text USING ''`).Error
	}

	return db.Exec(`UPDATE hosts SET verify_peer_cert_by_name = '' WHERE verify_peer_cert_by_name IS NULL OR typeof(verify_peer_cert_by_name) <> 'text'`).Error
}

// AutoMigrate casts the boolean-era column to text before the migration above runs,
// so a saved false/true reads back as the cert name "0"/"false"/"1"/"true".
func blankLegacyBoolVerifyPeerCertByName() error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Host{}).
			Where("verify_peer_cert_by_name IN ?", []string{"0", "1", "false", "true"}).
			Update("verify_peer_cert_by_name", "").Error; err != nil {
			return err
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "HostVerifyPeerCertByNameBoolFix"}).Error
	})
}

// migrateLegacySocksInboundsToMixed renames legacy socks inbounds to mixed.
// The protocol enum dropped socks in favor of mixed (identical settings shape,
// same behavior plus HTTP on the shared port), so rows predating the rename
// fail model validation — most visibly when pushed to a node, where one legacy
// inbound stalled the entire node's config and traffic sync (#5685).
func migrateLegacySocksInboundsToMixed() error {
	res := db.Exec("UPDATE inbounds SET protocol = 'mixed' WHERE protocol = 'socks'")
	if res.Error != nil {
		log.Printf("Error migrating legacy socks inbounds to mixed: %v", res.Error)
		return res.Error
	}
	if res.RowsAffected > 0 {
		log.Printf("Migrated %d legacy socks inbound(s) to mixed", res.RowsAffected)
	}
	return nil
}

// migrateShadowsocksRemovedCiphers rewrites shadowsocks inbounds still using
// the "none"/"plain" ciphers that xray-core v26.7.11 removed; one such row
// makes the whole generated config unbuildable and keeps xray from starting.
func migrateShadowsocksRemovedCiphers() error {
	var inbounds []model.Inbound
	if err := db.Where("protocol = ?", model.Shadowsocks).Find(&inbounds).Error; err != nil {
		return err
	}
	migrated := int64(0)
	for _, inbound := range inbounds {
		if strings.TrimSpace(inbound.Settings) == "" {
			continue
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			continue
		}
		changed := false
		if method, _ := settings["method"].(string); method != "" {
			if replacement, removed := model.ReplaceRemovedShadowsocksCipher(method); removed {
				settings["method"] = replacement
				changed = true
			}
		}
		if clients, ok := settings["clients"].([]any); ok {
			for i := range clients {
				cm, ok := clients[i].(map[string]any)
				if !ok {
					continue
				}
				method, _ := cm["method"].(string)
				if replacement, removed := model.ReplaceRemovedShadowsocksCipher(method); removed {
					cm["method"] = replacement
					clients[i] = cm
					changed = true
				}
			}
		}
		if !changed {
			continue
		}
		newSettings, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			log.Printf("migrateShadowsocksRemovedCiphers: skip inbound %d (marshal failed): %v", inbound.Id, err)
			continue
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
			Update("settings", string(newSettings)).Error; err != nil {
			return err
		}
		migrated++
	}
	if migrated > 0 {
		log.Printf("Rewrote removed shadowsocks cipher on %d inbound(s)", migrated)
	}
	return nil
}

// migrateVmessRemovedSecurities rewrites the vmess "none"/"zero" security
// values that xray-core v26.7.11 removed to "auto" (what the core now treats
// them as), on both the clients column and each vmess inbound's settings.
func migrateVmessRemovedSecurities() error {
	res := db.Exec("UPDATE clients SET security = 'auto' WHERE security IN ('none', 'zero')")
	if res.Error != nil {
		log.Printf("Error migrating removed vmess security values on clients: %v", res.Error)
		return res.Error
	}
	if res.RowsAffected > 0 {
		log.Printf("Migrated %d client(s) off removed vmess security values", res.RowsAffected)
	}
	var inbounds []model.Inbound
	if err := db.Where("protocol = ?", model.VMESS).Find(&inbounds).Error; err != nil {
		return err
	}
	migrated := int64(0)
	for _, inbound := range inbounds {
		if strings.TrimSpace(inbound.Settings) == "" {
			continue
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			continue
		}
		clients, ok := settings["clients"].([]any)
		if !ok {
			continue
		}
		changed := false
		for i := range clients {
			cm, ok := clients[i].(map[string]any)
			if !ok {
				continue
			}
			if security, _ := cm["security"].(string); security == "none" || security == "zero" {
				cm["security"] = "auto"
				clients[i] = cm
				changed = true
			}
		}
		if !changed {
			continue
		}
		newSettings, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			log.Printf("migrateVmessRemovedSecurities: skip inbound %d (marshal failed): %v", inbound.Id, err)
			continue
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
			Update("settings", string(newSettings)).Error; err != nil {
			return err
		}
		migrated++
	}
	if migrated > 0 {
		log.Printf("Rewrote removed vmess security values on %d inbound(s)", migrated)
	}
	return nil
}

// migrateTgIDIndex creates an index on the clients.tg_id column so that
// lookups by Telegram ID do not require a full table scan. The index tag
// on the struct field already causes AutoMigrate to create it on new
// installations; the explicit migration ensures existing databases get it.
func migrateTgIDIndex() error {
	if db.Migrator().HasIndex(&model.ClientRecord{}, "idx_clients_tg_id") {
		return nil
	}
	return db.Migrator().CreateIndex(&model.ClientRecord{}, "TgID")
}

// repairOverflowedTrafficCounters heals traffic counters that historic
// compounding bugs pushed past int64: on SQLite an overflowing INTEGER is
// silently promoted to REAL, after which the column no longer scans into the
// Go int64 field and every reader of the table fails (#5762). REAL cells are
// cast back to INTEGER (SQLite caps the cast at math.MaxInt64), then values
// are clamped into [0, TrafficMax] on both backends so the next delta cannot
// overflow again.
func repairOverflowedTrafficCounters() error {
	targets := []struct {
		table   string
		columns []string
	}{
		{"client_traffics", []string{"up", "down"}},
		{"inbounds", []string{"up", "down"}},
		{"outbound_traffics", []string{"up", "down", "total"}},
		{"node_client_traffics", []string{"up", "down"}},
	}
	for _, target := range targets {
		for _, col := range target.columns {
			statements := []string{
				fmt.Sprintf("UPDATE %s SET %s = %d WHERE %s > %d", target.table, col, TrafficMax, col, TrafficMax),
				fmt.Sprintf("UPDATE %s SET %s = 0 WHERE %s < 0", target.table, col, col),
			}
			if !IsPostgres() {
				statements = append([]string{
					fmt.Sprintf("UPDATE %s SET %s = CAST(%s AS INTEGER) WHERE typeof(%s) = 'real'", target.table, col, col, col),
				}, statements...)
			}
			var repaired int64
			for _, statement := range statements {
				res := db.Exec(statement)
				if res.Error != nil {
					log.Printf("Error repairing %s.%s: %v", target.table, col, res.Error)
					return res.Error
				}
				repaired += res.RowsAffected
			}
			if repaired > 0 {
				log.Printf("Repaired %d overflowed %s.%s value(s)", repaired, target.table, col)
			}
		}
	}
	return nil
}

func migrateVKTurnAcceptClientKeysDefaultOn() error {
	const seederName = "VKTurnAcceptClientKeysDefaultOn"
	var history []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &history).Error; err != nil {
		return err
	}
	if slices.Contains(history, seederName) {
		return nil
	}
	var sql string
	if IsPostgres() {
		sql = `UPDATE inbounds SET settings = jsonb_set(settings::jsonb, '{wrapAcceptClientKeys}', 'true'::jsonb)::text WHERE protocol = ?`
	} else {
		sql = `UPDATE inbounds SET settings = json_set(settings, '$.wrapAcceptClientKeys', json('true')) WHERE protocol = ?`
	}
	if err := db.Exec(sql, model.VKTurnProxy).Error; err != nil {
		return err
	}
	return db.Create(&model.HistoryOfSeeders{SeederName: seederName}).Error
}

func migrateOutboundRemovedKeys() error {
	var setting model.Setting
	err := db.Model(model.Setting{}).Where("key = ?", "xrayTemplateConfig").First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.HistoryOfSeeders{SeederName: "OutboundRemovedKeysFix"}).Error
	}
	if err != nil {
		return err
	}

	updated, changed, rErr := rewriteRemovedOutboundKeys(setting.Value)
	if rErr != nil {
		log.Printf("OutboundRemovedKeysFix: skip (invalid xrayTemplateConfig json): %v", rErr)
		return db.Create(&model.HistoryOfSeeders{SeederName: "OutboundRemovedKeysFix"}).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if changed {
			if err := tx.Model(&model.Setting{}).Where("key = ?", "xrayTemplateConfig").
				Update("value", updated).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "OutboundRemovedKeysFix"}).Error
	})
}

func migrateFreedomDomainStrategy() error {
	var setting model.Setting
	err := db.Model(model.Setting{}).Where("key = ?", "xrayTemplateConfig").First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.HistoryOfSeeders{SeederName: "FreedomDomainStrategyFix"}).Error
	}
	if err != nil {
		return err
	}

	updated, changed, rErr := rewriteFreedomDomainStrategy(setting.Value)
	if rErr != nil {
		log.Printf("FreedomDomainStrategyFix: skip (invalid xrayTemplateConfig json): %v", rErr)
		return db.Create(&model.HistoryOfSeeders{SeederName: "FreedomDomainStrategyFix"}).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if changed {
			if err := tx.Model(&model.Setting{}).Where("key = ?", "xrayTemplateConfig").
				Update("value", updated).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "FreedomDomainStrategyFix"}).Error
	})
}

// migrateDNSOutboundLegacyKeys rewrites stored dns outbounds once, because the
// core logs nonIPQuery/blockTypes as deprecated on every config load.
func migrateDNSOutboundLegacyKeys() error {
	var setting model.Setting
	err := db.Model(model.Setting{}).Where("key = ?", "xrayTemplateConfig").First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.HistoryOfSeeders{SeederName: "DNSOutboundLegacyKeysFix"}).Error
	}
	if err != nil {
		return err
	}

	updated, changed, rErr := rewriteDNSOutboundLegacyKeys(setting.Value)
	if rErr != nil {
		log.Printf("DNSOutboundLegacyKeysFix: skip (invalid xrayTemplateConfig json): %v", rErr)
		return db.Create(&model.HistoryOfSeeders{SeederName: "DNSOutboundLegacyKeysFix"}).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if changed {
			if err := tx.Model(&model.Setting{}).Where("key = ?", "xrayTemplateConfig").
				Update("value", updated).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "DNSOutboundLegacyKeysFix"}).Error
	})
}

// migrateDNSOutboundQTypeZero repairs the numeric qType 0 that 3.8.0's legacy-keys
// seeder stored, which that seeder's own history row keeps it from revisiting.
func migrateDNSOutboundQTypeZero() error {
	var setting model.Setting
	err := db.Model(model.Setting{}).Where("key = ?", "xrayTemplateConfig").First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.HistoryOfSeeders{SeederName: "DNSOutboundQTypeZeroFix"}).Error
	}
	if err != nil {
		return err
	}

	updated, changed, rErr := RewriteDNSOutboundQTypeZero(setting.Value)
	if rErr != nil {
		log.Printf("DNSOutboundQTypeZeroFix: skip (invalid xrayTemplateConfig json): %v", rErr)
		return db.Create(&model.HistoryOfSeeders{SeederName: "DNSOutboundQTypeZeroFix"}).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if changed {
			if err := tx.Model(&model.Setting{}).Where("key = ?", "xrayTemplateConfig").
				Update("value", updated).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "DNSOutboundQTypeZeroFix"}).Error
	})
}

func migrateApiTokenScopeAndExpiry() error {
	m := db.Migrator()
	if !m.HasColumn(&model.ApiToken{}, "Scope") {
		if err := m.AddColumn(&model.ApiToken{}, "Scope"); err != nil {
			return err
		}
	}
	if !m.HasColumn(&model.ApiToken{}, "ExpiresAt") {
		if err := m.AddColumn(&model.ApiToken{}, "ExpiresAt"); err != nil {
			return err
		}
	}
	return db.Model(&model.ApiToken{}).Where("scope IS NULL OR TRIM(scope) = ''").
		Updates(map[string]any{"scope": model.ApiScopeAdmin, "expires_at": 0}).Error
}

func migrateClientKeepAliveRangeColumn() error {
	migrator := db.Migrator()
	if !migrator.HasTable(&model.ClientRecord{}) || migrator.HasColumn(&model.ClientRecord{}, "wg_keep_alive_range") {
		return nil
	}
	return migrator.AddColumn(&model.ClientRecord{}, "KeepAliveRange")
}

func migrateManagerGracePeriod() error {
	return db.Model(&model.ManagerSubscription{}).Where("grace_minutes IS NULL OR grace_minutes = 0").Update("grace_minutes", 120).Error
}
