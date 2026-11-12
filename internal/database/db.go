package database

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/crypto"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"github.com/SawaMEN/3x-ui/v3/internal/util/random"
	"github.com/SawaMEN/3x-ui/v3/internal/util/sys"
)

var db *gorm.DB

var backupSQLiteTimeout = 2 * time.Minute

const (
	DialectSQLite   = "sqlite"
	DialectPostgres = "postgres"
)

func IsPostgres() bool {
	if db == nil {
		return config.GetDBKind() == "postgres"
	}
	return db.Name() == "postgres"
}

func Dialect() string {
	if db == nil {
		return ""
	}
	return db.Name()
}

func allModels() []any {
	return []any{
		&model.User{},
		&model.Inbound{},
		&model.OutboundTraffics{},
		&model.Setting{},
		&model.InboundClientIps{},
		&coremodel.ClientTraffic{},
		&model.HistoryOfSeeders{},
		&model.Node{},
		&model.ApiToken{},
		&model.RoutingPreset{},
		&model.RoutingList{},
		&model.UserSession{},
		&model.ClientRecord{},
		&model.ClientInbound{},
		&model.ClientHwid{},
		&model.ClientExternalLink{},
		&model.ClientGroup{},
		&model.InboundFallback{},
		&model.Host{},
		&model.HostTombstone{},
		&model.NodeClientTraffic{},
		&model.NodeClientIp{},
		&model.ClientGlobalTraffic{},
		&model.TelemtIPHistory{},
		&model.TelemtUserHistory{},
		&model.TelemtIPGeo{},
		&model.OutboundSubscription{},
		&model.SubBalancer{},
		&model.TuicTrafficReceipt{},
		&model.SidecarTrafficReceipt{},
		&model.CoreTrafficCheckpoint{},
		// AutoMigrate adds grace_minutes and retired; zero grace retains the safe runtime default.
		&model.ManagerSubscription{}, &model.ManagerOperation{}, &model.ManagerRelay{}, &model.ManagerDomain{},
	}
}

func initModels() error {
	if err := migrateClientKeepAliveRangeColumn(); err != nil {
		return err
	}
	if err := migrateClientTrafficLastSubFetchColumn(); err != nil {
		return err
	}
	if err := migrateOutboundSubscriptionUserAgentColumn(); err != nil {
		return err
	}
	models := allModels()
	for _, mdl := range models {
		if IsPostgres() && postgresModelSettled(mdl) {
			continue
		}
		if err := db.AutoMigrate(mdl); err != nil {
			if isIgnorableDuplicateColumnErr(db, err, mdl) {
				log.Printf("Ignoring duplicate column during auto migration for %T: %v", mdl, err)
				continue
			}
			log.Printf("Error auto migrating model: %v", err)
			return err
		}
	}
	if err := migrateManagerGracePeriod(); err != nil {
		return err
	}
	if err := dropLegacyInboundPortUnique(); err != nil {
		return err
	}
	if err := migrateHostVerifyPeerCertByNameColumn(); err != nil {
		return err
	}
	if err := normalizeApiTokenCreatedAtSeconds(); err != nil {
		return err
	}
	if err := migrateApiTokenScopeAndExpiry(); err != nil {
		return err
	}
	if err := dropLegacyForeignKeys(); err != nil {
		return err
	}
	if err := pruneOrphanedClientInbounds(); err != nil {
		return err
	}
	if err := pruneOrphanedHosts(); err != nil {
		return err
	}
	if err := normalizeInboundSubSortIndex(); err != nil {
		return err
	}
	if err := normalizeClientExternalLinkEnable(); err != nil {
		return err
	}
	if err := normalizeClientExternalLinkTimestamps(); err != nil {
		return err
	}
	if err := repairOverflowedTrafficCounters(); err != nil {
		return err
	}
	if err := dedupeInboundSettingsClients(); err != nil {
		return err
	}
	if err := migrateShadowTLSInnerKeys(); err != nil {
		return err
	}
	if err := migrateLegacySocksInboundsToMixed(); err != nil {
		return err
	}
	if err := migrateShadowsocksRemovedCiphers(); err != nil {
		return err
	}
	if err := migrateVmessRemovedSecurities(); err != nil {
		return err
	}
	if err := migrateTgIDIndex(); err != nil {
		return err
	}
	if err := migrateClientTrafficResetColumns(); err != nil {
		return err
	}
	if err := migrateSyncOrphanColumns(); err != nil {
		return err
	}
	if err := migrateClientEmailLowerIndex(); err != nil {
		return err
	}
	if err := migrateNodePanelEdition(); err != nil {
		return err
	}
	if IsPostgres() {
		if err := resyncPostgresSequences(db, models); err != nil {
			log.Printf("Error resyncing postgres sequences: %v", err)
			return err
		}
	}
	return nil
}

func postgresModelSettled(mdl any) bool {
	migrator := db.Migrator()
	if !migrator.HasTable(mdl) {
		return false
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(mdl); err != nil || stmt.Schema == nil {
		return false
	}
	for _, dbName := range stmt.Schema.DBNames {
		if !migrator.HasColumn(mdl, dbName) {
			return false
		}
	}
	for _, idx := range stmt.Schema.ParseIndexes() {
		if !migrator.HasIndex(mdl, idx.Name) {
			return false
		}
	}
	return true
}

type sqliteIndexListRow struct {
	Name   string `gorm:"column:name"`
	Unique int    `gorm:"column:unique"`
	Origin string `gorm:"column:origin"`
}

func sqliteUniquePortIndexes() (autoIndexes, explicitIndexes []string, err error) {
	var list []sqliteIndexListRow
	if err = db.Raw(`PRAGMA index_list('inbounds')`).Scan(&list).Error; err != nil {
		return nil, nil, err
	}
	for _, idx := range list {
		if idx.Unique != 1 {
			continue
		}
		var cols []struct {
			Name string `gorm:"column:name"`
		}
		if err = db.Raw(`PRAGMA index_info(` + quoteSQLiteIdentifier(idx.Name) + `)`).Scan(&cols).Error; err != nil {
			return nil, nil, err
		}
		if len(cols) != 1 || cols[0].Name != "port" {
			continue
		}
		if idx.Origin == "c" {
			explicitIndexes = append(explicitIndexes, idx.Name)
		} else {
			autoIndexes = append(autoIndexes, idx.Name)
		}
	}
	return autoIndexes, explicitIndexes, nil
}

func sqliteTableColumns(tx *gorm.DB, table string) ([]string, error) {
	var rows []struct {
		Name string `gorm:"column:name"`
	}
	if err := tx.Raw(`PRAGMA table_info(` + quoteSQLiteIdentifier(table) + `)`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	cols := make([]string, 0, len(rows))
	for _, r := range rows {
		cols = append(cols, r.Name)
	}
	return cols, nil
}

func CreateHostFromMtprotoCustomShareAddr(tx *gorm.DB, inboundId int, rawAddress string) error {
	address := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(rawAddress), "]"), "[")
	if address == "" {
		return nil
	}
	var sameAddress []model.Host
	if err := tx.Where("inbound_id = ? AND address = ? AND is_disabled = ?", inboundId, address, false).
		Find(&sameAddress).Error; err != nil {
		return err
	}
	for _, host := range sameAddress {
		if !slices.Contains(host.ExcludeFromSubTypes, "raw") {
			return nil
		}
	}
	return tx.Create(&model.Host{
		GroupId: random.NumLower(16), InboundId: inboundId,
		Remark: address, Address: address, Security: "same",
	}).Error
}

func wireguardPeerEmail(remark string, peer map[string]any, index int, used map[string]struct{}) string {
	base := strings.TrimSpace(remark)
	if base == "" {
		base = "wg"
	}
	suffix := strconv.Itoa(index + 1)
	if c, ok := peer["comment"].(string); ok && strings.TrimSpace(c) != "" {
		suffix = strings.TrimSpace(c)
	}
	email := strings.ReplaceAll(base+"-"+suffix, " ", "-")
	candidate := email
	for n := 2; ; n++ {
		if _, taken := used[candidate]; !taken {
			return candidate
		}
		candidate = email + "-" + strconv.Itoa(n)
	}
}

// stripMtprotoInboundSecrets removes the vestigial inbound-level `secret` from
// every mtproto inbound. seedMtprotoSecretsToClients already drops it while
// converting legacy single-secret inbounds, but inbounds that already had clients
// kept the dead field, and the old HealMtprotoSecret regenerated it on every
// save. mtg and every share link read only per-client secrets, so the
// inbound-level value is dead data that once leaked into stale, unusable links.
// One-time, self-gated on the "StripMtprotoInboundSecrets" seeder row.
func stripMtprotoInboundSecrets() error {
	var history []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &history).Error; err != nil {
		return err
	}
	if slices.Contains(history, "StripMtprotoInboundSecrets") {
		return nil
	}

	var inbounds []model.Inbound
	if err := db.Where("protocol = ?", string(model.MTProto)).Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, inbound := range inbounds {
			stripped, ok := model.StripMtprotoInboundSecret(inbound.Settings)
			if !ok {
				continue
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", stripped).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "StripMtprotoInboundSecrets"}).Error
	})
}

// mtprotoInboundClientEmail derives a stable, unique client email for a migrated
// mtproto inbound from its remark.
func mtprotoInboundClientEmail(remark string, used map[string]struct{}) string {
	base := strings.TrimSpace(remark)
	if base == "" {
		base = "mtproto"
	}
	email := strings.ReplaceAll(base, " ", "-")
	candidate := email
	for n := 2; ; n++ {
		if _, taken := used[candidate]; !taken {
			return candidate
		}
		candidate = email + "-" + strconv.Itoa(n)
	}
}

// CreateHostsFromExternalProxy parses a legacy streamSettings.externalProxy array
// and inserts one Host row per entry on tx, returning the number of rows created.
// It is the shared core of both the one-time seedHostsFromExternalProxy startup
// migration and the inbound-import path: an inbound exported from a build that
// predated the hosts table carries its external proxies inline in
// streamSettings.externalProxy, and the startup migration is gated off after its
// first run, so a freshly imported inbound must be converted here instead. Blank
// or malformed streamSettings, or one without externalProxy entries, is a no-op.
func CreateHostsFromExternalProxy(tx *gorm.DB, inboundId int, streamSettings string) (int, error) {
	imports, _ := externalProxyHosts(inboundId, streamSettings)
	created := 0
	for _, host := range imports {
		if err := tx.Create(host).Error; err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

// Preserve the complete legacy endpoint contract when moving it into Hosts.
func externalProxyEntryToHost(inboundId, index int, ep map[string]any) *model.Host {
	host := legacyExternalProxyEntryToHost(inboundId, index, ep)
	if remark, _ := ep["remark"].(string); strings.TrimSpace(remark) != "" {
		runes := []rune(remark)
		if len(runes) > 256 {
			runes = runes[:256]
		}
		host.Remark = string(runes)
	}
	host.Alpn = externalProxyStringList(ep["alpn"])
	host.PinnedPeerCertSha256 = externalProxyStringList(ep["pinnedPeerCertSha256"])
	host.VerifyPeerCertByName, _ = ep["verifyPeerCertByName"].(string)
	host.CipherSuites, _ = ep["cipherSuites"].(string)
	host.VlessRoute, _ = ep["vlessRoute"].(string)
	host.AllowInsecure, _ = ep["allowInsecure"].(bool)
	host.ServerDescription, _ = ep["serverDescription"].(string)
	return host
}

func externalProxyStringList(value any) []string {
	if text, ok := value.(string); ok {
		var values []string
		for _, part := range strings.Split(text, ",") {
			if part = strings.TrimSpace(part); part != "" {
				values = append(values, part)
			}
		}
		return values
	}
	return anyToNonEmptyStrings(value)
}

// Frozen mapping used only to recognize untouched rows made by older builds.
func legacyExternalProxyEntryToHost(inboundId, index int, ep map[string]any) *model.Host {
	security, _ := ep["forceTls"].(string)
	switch security {
	case "same", "tls", "none":
	default:
		security = "same"
	}
	dest, _ := ep["dest"].(string)
	port := 0
	if p, ok := ep["port"].(float64); ok {
		port = int(p)
	}
	remark, _ := ep["remark"].(string)
	if strings.TrimSpace(remark) == "" {
		remark = "imported " + strconv.Itoa(index+1)
	}
	if len(remark) > 256 {
		remark = remark[:256]
	}
	sni, _ := ep["sni"].(string)
	fingerprint, _ := ep["fingerprint"].(string)
	ech, _ := ep["echConfigList"].(string)
	return &model.Host{
		GroupId:              random.NumLower(16),
		InboundId:            inboundId,
		SortOrder:            index,
		Remark:               remark,
		Address:              dest,
		Port:                 port,
		Security:             security,
		Sni:                  sni,
		Snis:                 anyToNonEmptyStrings([]any{sni}),
		Fingerprint:          fingerprint,
		Alpn:                 anyToNonEmptyStrings(ep["alpn"]),
		PinnedPeerCertSha256: anyToNonEmptyStrings(ep["pinnedPeerCertSha256"]),
		EchConfigList:        ech,
		RandomCount:          1,
	}
}

func anyToNonEmptyStrings(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func pruneOrphanedHosts() error {
	var pruned int64
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		pruned, err = DeleteHosts(tx, "inbound_id NOT IN (SELECT id FROM inbounds)")
		return err
	})
	if err != nil {
		log.Printf("Error pruning orphaned hosts rows: %v", err)
		return err
	}
	if pruned > 0 {
		log.Printf("Pruned %d orphaned hosts row(s)", pruned)
	}
	return nil
}

func pruneOrphanedClientInbounds() error {
	res := db.Exec("DELETE FROM client_inbounds WHERE inbound_id NOT IN (SELECT id FROM inbounds)")
	if res.Error != nil {
		log.Printf("Error pruning orphaned client_inbounds rows: %v", res.Error)
		return res.Error
	}
	if res.RowsAffected > 0 {
		log.Printf("Pruned %d orphaned client_inbounds row(s)", res.RowsAffected)
	}
	return nil
}

// normalizeInboundSubSortIndex lifts legacy zero defaults to 1.
// Explicit negatives are left alone so primary inbounds can sort first.
func normalizeInboundSubSortIndex() error {
	res := db.Exec("UPDATE inbounds SET sub_sort_index = 1 WHERE sub_sort_index = 0")
	if res.Error != nil {
		log.Printf("Error normalizing inbound sub_sort_index: %v", res.Error)
		return res.Error
	}
	if res.RowsAffected > 0 {
		log.Printf("Normalized sub_sort_index on %d inbound(s)", res.RowsAffected)
	}
	return nil
}

// normalizeClientExternalLinkEnable keeps external-link rows written before the
// enable column existed enabled; disabled rows from newer builds stay false.
func normalizeClientExternalLinkEnable() error {
	res := db.Exec("UPDATE client_external_links SET enable = ? WHERE enable IS NULL", true)
	if res.Error != nil {
		log.Printf("Error normalizing client external link enable: %v", res.Error)
		return res.Error
	}
	if res.RowsAffected > 0 {
		log.Printf("Normalized enable on %d client external link(s)", res.RowsAffected)
	}
	return nil
}

// normalizeClientExternalLinkTimestamps zeroes the NULLs an older build could
// leave behind, so the sub-side expiry predicate never drops a legacy row.
func normalizeClientExternalLinkTimestamps() error {
	res := db.Exec("UPDATE client_external_links SET expiry_time = 0 WHERE expiry_time IS NULL")
	if res.Error != nil {
		log.Printf("Error normalizing client external link expiry_time: %v", res.Error)
		return res.Error
	}
	expiryRows := res.RowsAffected
	res = db.Exec("UPDATE client_external_links SET last_fetch_at = 0 WHERE last_fetch_at IS NULL")
	if res.Error != nil {
		log.Printf("Error normalizing client external link last_fetch_at: %v", res.Error)
		return res.Error
	}
	if expiryRows+res.RowsAffected > 0 {
		log.Printf("Normalized timestamps on %d client external link(s)", expiryRows+res.RowsAffected)
	}
	return nil
}

// dedupeInboundSettingsClients collapses duplicate same-email entries inside
// every inbound's settings.clients array, keeping the first occurrence.
// Retried or raced multi-node client adds on older builds appended the same
// client several times (#5770), which the client lists then rendered as
// phantom duplicates. Runs on every start (idempotent, writes only changed
// rows) because a restored backup or a not-yet-upgraded node's snapshot can
// reintroduce duplicates.
func dedupeInboundSettingsClients() error {
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}
	repaired := int64(0)
	for _, inbound := range inbounds {
		if strings.TrimSpace(inbound.Settings) == "" {
			continue
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			continue
		}
		clients, _ := settings["clients"].([]any)
		if len(clients) < 2 {
			continue
		}
		seen := make(map[string]struct{}, len(clients))
		kept := make([]any, 0, len(clients))
		for _, c := range clients {
			if cm, ok := c.(map[string]any); ok {
				if email, _ := cm["email"].(string); email != "" {
					key := strings.ToLower(email)
					if _, dup := seen[key]; dup {
						continue
					}
					seen[key] = struct{}{}
				}
			}
			kept = append(kept, c)
		}
		if len(kept) == len(clients) {
			continue
		}
		settings["clients"] = kept
		newSettings, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			log.Printf("dedupeInboundSettingsClients: skip inbound %d (marshal failed): %v", inbound.Id, err)
			continue
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
			Update("settings", string(newSettings)).Error; err != nil {
			return err
		}
		repaired++
	}
	if repaired > 0 {
		log.Printf("Removed duplicate client entries from %d inbound(s)", repaired)
	}
	return nil
}

func isIgnorableDuplicateColumnErr(gdb *gorm.DB, err error, mdl any) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())

	const sqlitePrefix = "duplicate column name:"
	if _, after, ok := strings.Cut(errMsg, sqlitePrefix); ok {
		col := strings.TrimSpace(after)
		col = strings.Trim(col, "`\"[]")
		return col != "" && gdb != nil && gdb.Migrator().HasColumn(mdl, col)
	}
	if strings.Contains(errMsg, "already exists") && strings.Contains(errMsg, "column ") {
		if _, after, ok := strings.Cut(errMsg, "column \""); ok {
			rest := after
			if e := strings.Index(rest, "\""); e > 0 {
				col := rest[:e]
				return col != "" && gdb != nil && gdb.Migrator().HasColumn(mdl, col)
			}
		}
	}
	return false
}

// backfillEmptyHostGroupIds is idempotent and not seeder-gated: builds that
// predate group ids on the inbound-import path (and restored backups) can
// re-introduce hosts rows with an empty group_id, and such rows render as a
// synthetic fallback_<id> group the update/delete API cannot address, so
// re-check on every start.
func backfillEmptyHostGroupIds() error {
	var hosts []*model.Host
	if err := db.Where("group_id = '' OR group_id IS NULL").Find(&hosts).Error; err != nil {
		return err
	}
	if len(hosts) == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, h := range hosts {
			if err := tx.Model(h).Update("group_id", random.NumLower(16)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func resetIpLimitsWithoutFail2ban() error {
	var history []string
	if err := db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &history).Error; err != nil {
		return err
	}
	if slices.Contains(history, "ResetIpLimitNoFail2ban") {
		return nil
	}

	state, probeErr := fail2banEnforcementState()
	if state == fail2banEnforcing {
		return db.Create(&model.HistoryOfSeeders{SeederName: "ResetIpLimitNoFail2ban"}).Error
	}
	if state == fail2banUnknown {
		log.Printf("ResetIpLimitNoFail2ban: fail2ban-client present but not runnable (%v); keeping configured IP limits, will retry next start", probeErr)
		return nil
	}

	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("ResetIpLimitNoFail2ban: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			clients, ok := settings["clients"].([]any)
			if !ok {
				continue
			}
			mutated := false
			for i, raw := range clients {
				obj, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				v, present := obj["limitIp"]
				if !present {
					continue
				}
				if n, isNum := v.(float64); isNum && n == 0 {
					continue
				}
				obj["limitIp"] = 0
				clients[i] = obj
				mutated = true
			}
			if !mutated {
				continue
			}
			settings["clients"] = clients
			newSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				log.Printf("ResetIpLimitNoFail2ban: skip inbound %d (marshal failed): %v", inbound.Id, err)
				continue
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", string(newSettings)).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.ClientRecord{}).Where("limit_ip <> ?", 0).
			Update("limit_ip", 0).Error; err != nil {
			return err
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "ResetIpLimitNoFail2ban"}).Error
	})
}

type fail2banState int

const (
	fail2banEnforcing fail2banState = iota
	fail2banAbsent
	fail2banUnknown
)

// fail2banEnforcementState separates "fail2ban is not installed" from "the probe
// itself failed", so a transient failure never drives an irreversible cleanup.
func fail2banEnforcementState() (fail2banState, error) {
	if v, ok := os.LookupEnv("XUI_ENABLE_FAIL2BAN"); ok && v != "true" {
		return fail2banAbsent, nil
	}
	if runtime.GOOS == "windows" {
		return fail2banAbsent, nil
	}
	if _, err := exec.LookPath("fail2ban-client"); err != nil {
		return fail2banAbsent, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "fail2ban-client", "-h").Run(); err != nil {
		if ctx.Err() != nil {
			return fail2banUnknown, ctx.Err()
		}
		return fail2banUnknown, err
	}
	return fail2banEnforcing, nil
}

func clearLegacyProxySettings() error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("key IN ?", []string{"panelProxy", "tgBotProxy"}).
			Delete(&model.Setting{}).Error; err != nil {
			return err
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "LegacyProxySettingsCleanup"}).Error
	})
}

// rewriteRemovedOutboundKeys moves outbound proxySettings.tag to sockopt.dialerProxy
// and drops freedom sockopt.addressPortStrategy: xray-core v26.9.8 refuses both.
func rewriteRemovedOutboundKeys(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return raw, false, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, false, err
	}
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		return raw, false, nil
	}
	changed := false
	for _, ob := range outbounds {
		obj, ok := ob.(map[string]any)
		if !ok {
			continue
		}
		if proxySettings, present := obj["proxySettings"]; present {
			ps, _ := proxySettings.(map[string]any)
			if tag, _ := ps["tag"].(string); tag != "" {
				sockopt := outboundSockopt(obj, true)
				if current, _ := sockopt["dialerProxy"].(string); current == "" {
					sockopt["dialerProxy"] = tag
				}
			}
			delete(obj, "proxySettings")
			changed = true
		}
		if proto, _ := obj["protocol"].(string); strings.EqualFold(proto, "freedom") {
			if sockopt := outboundSockopt(obj, false); sockopt != nil {
				if _, present := sockopt["addressPortStrategy"]; present {
					delete(sockopt, "addressPortStrategy")
					changed = true
				}
			}
		}
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return raw, false, err
	}
	return string(out), true, nil
}

func outboundSockopt(obj map[string]any, create bool) map[string]any {
	stream, _ := obj["streamSettings"].(map[string]any)
	if stream == nil {
		if !create {
			return nil
		}
		stream = map[string]any{}
		obj["streamSettings"] = stream
	}
	sockopt, _ := stream["sockopt"].(map[string]any)
	if sockopt == nil {
		if !create {
			return nil
		}
		sockopt = map[string]any{}
		stream["sockopt"] = sockopt
	}
	return sockopt
}

// rewriteFreedomDomainStrategy moves a freedom outbound's legacy strategy keys
// into sockopt.domainStrategy, the placement the core's deprecation warning names.
func rewriteFreedomDomainStrategy(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return raw, false, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, false, err
	}
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		return raw, false, nil
	}
	changed := false
	for _, ob := range outbounds {
		obj, ok := ob.(map[string]any)
		if !ok {
			continue
		}
		if proto, _ := obj["protocol"].(string); !strings.EqualFold(proto, "freedom") {
			continue
		}
		settings, hasSettings := obj["settings"].(map[string]any)
		_, hasRoot := obj["targetStrategy"]
		_, hasSettingsTarget := settings["targetStrategy"]
		_, hasSettingsDomain := settings["domainStrategy"]
		if !hasRoot && !hasSettingsTarget && !hasSettingsDomain {
			continue
		}
		strategy := freedomMigratedStrategy(obj, settings)
		delete(obj, "targetStrategy")
		if hasSettings {
			delete(settings, "targetStrategy")
			delete(settings, "domainStrategy")
		}
		if strategy != "" {
			outboundSockopt(obj, true)["domainStrategy"] = strategy
		}
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return raw, false, err
	}
	return string(out), true, nil
}

// freedomMigratedStrategy clones the core's own resolution order for a freedom
// outbound (infra/conf/freedom.go), returning "" when none of them holds one.
func freedomMigratedStrategy(obj, settings map[string]any) string {
	if s, ok := freedomStrategyValue(obj["targetStrategy"]); ok && !strings.EqualFold(s, "asis") {
		return s
	}
	legacy := settings["targetStrategy"]
	if s, ok := legacy.(string); !ok || s == "" {
		legacy = settings["domainStrategy"]
	}
	if s, ok := freedomStrategyValue(legacy); ok && !strings.EqualFold(s, "asis") {
		return s
	}
	return ""
}

// freedomStrategyValue reports a strategy the core accepts -- anything else is a
// hard load error in freedom and sockopt alike, so it cannot be migrated.
func freedomStrategyValue(value any) (string, bool) {
	s, ok := value.(string)
	if !ok || s == "" {
		return "", false
	}
	if !freedomDomainStrategies[strings.ToLower(s)] {
		return "", false
	}
	return s, true
}

var freedomDomainStrategies = map[string]bool{
	"asis": true, "useip": true, "useipv4": true, "useipv6": true,
	"useipv4v6": true, "useipv6v4": true, "forceip": true, "forceipv4": true,
	"forceipv6": true, "forceipv4v6": true, "forceipv6v4": true,
}

// rewriteDNSOutboundLegacyKeys turns a dns outbound's legacy nonIPQuery and
// blockTypes into rules, in the order the core's legacy builder used.
func rewriteDNSOutboundLegacyKeys(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return raw, false, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, false, err
	}
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		return raw, false, nil
	}
	changed := false
	for _, ob := range outbounds {
		obj, ok := ob.(map[string]any)
		if !ok {
			continue
		}
		if proto, _ := obj["protocol"].(string); !strings.EqualFold(proto, "dns") {
			continue
		}
		settings, _ := obj["settings"].(map[string]any)
		if settings == nil {
			continue
		}
		nonIPQuery, hasMode := settings["nonIPQuery"]
		blockTypes, hasTypes := settings["blockTypes"]
		// JSON null is absent to the core, which decides on nil pointers.
		hasMode = hasMode && nonIPQuery != nil
		hasTypes = hasTypes && blockTypes != nil
		if !hasMode && !hasTypes {
			continue
		}
		// The core refuses legacy keys next to real rules, so existing rules win.
		if rules, hasRules := settings["rules"]; !hasRules || rules == nil {
			settings["rules"] = legacyDNSOutboundRules(dnsNonIPQueryMode(nonIPQuery), legacyDNSBlockTypes(blockTypes))
		}
		delete(settings, "nonIPQuery")
		delete(settings, "blockTypes")
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return raw, false, err
	}
	return string(out), true, nil
}

// dnsNonIPQueryMode reports the mode the core resolved: everything but drop and
// skip meant reject, and any other value never loaded in the first place.
func dnsNonIPQueryMode(value any) string {
	mode, _ := value.(string)
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "drop" || mode == "skip" {
		return mode
	}
	return "reject"
}

// legacyDNSBlockTypes accepts every shape the old card could save: a list, a
// bare number, or a comma-separated string, minus the qTypes the core rejects.
func legacyDNSBlockTypes(value any) []int {
	items, ok := value.([]any)
	if !ok && value != nil {
		items = []any{value}
	}
	var out []int
	for _, item := range items {
		for _, part := range strings.Split(fmt.Sprint(item), ",") {
			qType, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || qType < 0 || qType > 65535 {
				continue
			}
			out = append(out, qType)
		}
	}
	return out
}

// legacyDNSOutboundRules mirrors the core's own legacy dns policy: the blocked
// qTypes, then the hijack, then the mode's answer for everything else.
func legacyDNSOutboundRules(mode string, blockTypes []int) []any {
	rules := make([]any, 0, 3)
	if len(blockTypes) > 0 {
		rule := map[string]any{"action": "drop", "qType": dnsQTypeValue(blockTypes)}
		if mode == "reject" {
			rule["action"] = "return"
			rule["rCode"] = 5
		}
		rules = append(rules, rule)
	}
	rules = append(rules, map[string]any{"action": "hijack", "qType": "1,28"})
	fallback := map[string]any{"action": "direct"}
	switch mode {
	case "reject":
		fallback["action"] = "return"
		fallback["rCode"] = 5
	case "drop":
		fallback["action"] = "drop"
	}
	return append(rules, fallback)
}

// dnsQTypeValue keeps a lone qType a number the way the core marshals one, except
// 0: the core drops a numeric 0, and a rule with no qTypes matches every query.
func dnsQTypeValue(blockTypes []int) any {
	if len(blockTypes) == 1 && blockTypes[0] != 0 {
		return blockTypes[0]
	}
	parts := make([]string, 0, len(blockTypes))
	for _, qType := range blockTypes {
		parts = append(parts, strconv.Itoa(qType))
	}
	return strings.Join(parts, ",")
}

// RewriteDNSOutboundQTypeZero spells a dns rule's numeric qType 0 as "0", the one
// form the core reads as query type 0 rather than as every query.
func RewriteDNSOutboundQTypeZero(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return raw, false, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, false, err
	}
	outbounds, _ := cfg["outbounds"].([]any)
	changed := false
	for _, ob := range outbounds {
		obj, _ := ob.(map[string]any)
		if proto, _ := obj["protocol"].(string); !strings.EqualFold(proto, "dns") {
			continue
		}
		settings, _ := obj["settings"].(map[string]any)
		rules, _ := settings["rules"].([]any)
		for _, r := range rules {
			rule, _ := r.(map[string]any)
			if qType, ok := rule["qType"].(float64); ok && qType == 0 {
				rule["qType"] = "0"
				changed = true
			}
		}
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return raw, false, err
	}
	return string(out), true, nil
}

func normalizeSettingPaths() error {
	pathKeys := []string{"webBasePath", "subPath", "subJsonPath", "subClashPath"}
	var rows []model.Setting
	if err := db.Where("key IN ?", pathKeys).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		fixed := strings.Map(func(r rune) rune {
			if strings.ContainsRune(`\ :*?#%;`, r) || r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, row.Value)
		parts := strings.Split(fixed, "/")
		clean := make([]string, 0, len(parts))
		for _, part := range parts {
			if part != "" && part != "." && part != ".." {
				clean = append(clean, part)
			}
		}
		fixed = "/" + strings.Join(clean, "/")
		if len(clean) > 0 {
			fixed += "/"
		}
		if fixed != row.Value {
			log.Printf("Normalized URI path setting %s", row.Key)
			if strings.Trim(fixed, "/") == "" && strings.Trim(row.Value, "/") != "" {
				fixed = "/" + random.Seq(16) + "/"
			}
		}
		if !strings.HasPrefix(fixed, "/") {
			fixed = "/" + fixed
		}
		if !strings.HasSuffix(fixed, "/") {
			fixed += "/"
		}
		if fixed == row.Value {
			continue
		}
		if err := db.Model(&model.Setting{}).Where("id = ?", row.Id).
			Update("value", fixed).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizeInboundClientTgId() error {
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("InboundClientTgIdFix: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			clients, ok := settings["clients"].([]any)
			if !ok {
				continue
			}
			mutated := false
			for i, raw := range clients {
				obj, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				tgRaw, present := obj["tgId"]
				if !present {
					continue
				}
				v, isFloat := tgRaw.(float64)
				if isFloat && !math.IsNaN(v) && !math.IsInf(v, 0) && v == math.Trunc(v) {
					continue
				}
				obj["tgId"] = int64(0)
				if s, isStr := tgRaw.(string); isStr {
					if id, err := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(s), " ", ""), 10, 64); err == nil {
						obj["tgId"] = id
					}
				}
				clients[i] = obj
				mutated = true
			}
			if !mutated {
				continue
			}
			settings["clients"] = clients
			newSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				log.Printf("InboundClientTgIdFix: skip inbound %d (marshal failed): %v", inbound.Id, err)
				continue
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", string(newSettings)).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "InboundClientTgIdFix2"}).Error
	})
}

func normalizeInboundClientSubId() error {
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("InboundClientSubIdFix: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			clients, ok := settings["clients"].([]any)
			if !ok {
				continue
			}
			mutated := false
			for i, raw := range clients {
				obj, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				existing, _ := obj["subId"].(string)
				if strings.TrimSpace(existing) != "" {
					continue
				}
				obj["subId"] = random.NumLower(16)
				clients[i] = obj
				mutated = true
			}
			if !mutated {
				continue
			}
			settings["clients"] = clients
			newSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				log.Printf("InboundClientSubIdFix: skip inbound %d (marshal failed): %v", inbound.Id, err)
				continue
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", string(newSettings)).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "InboundClientSubIdFix"}).Error
	})
}

func normalizeInboundClientsArray() error {
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, inbound := range inbounds {
			if strings.TrimSpace(inbound.Settings) == "" {
				continue
			}
			var settings map[string]any
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
				log.Printf("InboundClientsArrayFix: skip inbound %d (invalid settings json): %v", inbound.Id, err)
				continue
			}
			raw, exists := settings["clients"]
			if !exists || raw != nil {
				continue
			}
			settings["clients"] = []any{}
			newSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				log.Printf("InboundClientsArrayFix: skip inbound %d (marshal failed): %v", inbound.Id, err)
				continue
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).
				Update("settings", string(newSettings)).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "InboundClientsArrayFix"}).Error
	})
}

func normalizeFreedomFinalRules() error {
	var setting model.Setting
	err := db.Model(model.Setting{}).Where("key = ?", "xrayTemplateConfig").First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesReverseFix"}).Error
	}
	if err != nil {
		return err
	}

	updated, changed, rErr := rewriteFreedomFinalRules(setting.Value)
	if rErr != nil {
		log.Printf("FreedomFinalRulesReverseFix: skip (invalid xrayTemplateConfig json): %v", rErr)
		return db.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesReverseFix"}).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if changed {
			if err := tx.Model(&model.Setting{}).Where("key = ?", "xrayTemplateConfig").
				Update("value", updated).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesReverseFix"}).Error
	})
}

func rewriteFreedomFinalRules(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return raw, false, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, false, err
	}
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		return raw, false, nil
	}
	changed := false
	for _, ob := range outbounds {
		obj, ok := ob.(map[string]any)
		if !ok {
			continue
		}
		if proto, _ := obj["protocol"].(string); !strings.EqualFold(proto, "freedom") {
			continue
		}
		settings, ok := obj["settings"].(map[string]any)
		if !ok {
			continue
		}
		if !isLegacyPrivateOnlyFinalRules(settings["finalRules"]) {
			continue
		}
		settings["finalRules"] = []any{map[string]any{"action": "allow"}}
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return raw, false, err
	}
	return string(out), true, nil
}

func isLegacyPrivateOnlyFinalRules(v any) bool {
	rules, ok := v.([]any)
	if !ok || len(rules) != 1 {
		return false
	}
	rule, ok := rules[0].(map[string]any)
	if !ok {
		return false
	}
	if action, _ := rule["action"].(string); action != "allow" {
		return false
	}
	ips, ok := rule["ip"].([]any)
	if !ok || len(ips) != 1 {
		return false
	}
	if s, _ := ips[0].(string); s != "geoip:private" {
		return false
	}
	for k := range rule {
		if k != "action" && k != "ip" {
			return false
		}
	}
	return true
}

func isUnrestrictedFreedomFinalRules(v any, present bool) bool {
	if !present || v == nil {
		return true
	}
	rules, ok := v.([]any)
	return ok && len(rules) == 0
}

func hardenFreedomFinalRules() error {
	var setting model.Setting
	err := db.Model(model.Setting{}).Where("key = ?", "xrayTemplateConfig").First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesPrivateEgressBlock"}).Error
	}
	if err != nil {
		return err
	}

	updated, changed, rErr := rewriteFreedomFinalRulesPrivateEgress(setting.Value)
	if rErr != nil {
		log.Printf("FreedomFinalRulesPrivateEgressBlock: skip (invalid xrayTemplateConfig json): %v", rErr)
		return db.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesPrivateEgressBlock"}).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if changed {
			if err := tx.Model(&model.Setting{}).Where("key = ?", "xrayTemplateConfig").
				Update("value", updated).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "FreedomFinalRulesPrivateEgressBlock"}).Error
	})
}

func rewriteFreedomFinalRulesPrivateEgress(raw string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return raw, false, nil
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, false, err
	}
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		return raw, false, nil
	}
	changed := false
	for _, ob := range outbounds {
		obj, ok := ob.(map[string]any)
		if !ok {
			continue
		}
		if proto, _ := obj["protocol"].(string); !strings.EqualFold(proto, "freedom") {
			continue
		}
		settings, ok := obj["settings"].(map[string]any)
		if !ok {
			continue
		}
		finalRules, present := settings["finalRules"]
		if !isUnrestrictedFreedomFinalRules(finalRules, present) &&
			!isAllowOnlyFinalRules(finalRules) &&
			!isLegacyPrivateOnlyFinalRules(finalRules) {
			continue
		}
		settings["finalRules"] = []any{
			map[string]any{"action": "block", "ip": []any{"geoip:private"}},
			map[string]any{"action": "allow"},
		}
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return raw, false, err
	}
	return string(out), true, nil
}

func fixUppercaseFreedomFinalRules() error {
	var setting model.Setting
	err := db.Model(model.Setting{}).Where("key = ?", "xrayTemplateConfig").First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&model.HistoryOfSeeders{SeederName: "UppercaseFreedomFinalRulesFix"}).Error
	}
	if err != nil {
		return err
	}

	updated, changed, rErr := rewriteUppercaseFreedomFinalRules(setting.Value)
	if rErr != nil {
		log.Printf("UppercaseFreedomFinalRulesFix: skip (invalid xrayTemplateConfig json): %v", rErr)
		return db.Create(&model.HistoryOfSeeders{SeederName: "UppercaseFreedomFinalRulesFix"}).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if changed {
			if err := tx.Model(&model.Setting{}).Where("key = ?", "xrayTemplateConfig").
				Update("value", updated).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "UppercaseFreedomFinalRulesFix"}).Error
	})
}

// Re-runs both finalRules rewrites, because the rows of the two seeders above it
// already exist on any panel that walked past a differently spelled outbound.
func rewriteUppercaseFreedomFinalRules(raw string) (string, bool, error) {
	if !hasNonLowercaseFreedomOutbound(raw) {
		return raw, false, nil
	}
	reversed, reversedChanged, err := rewriteFreedomFinalRules(raw)
	if err != nil {
		return raw, false, err
	}
	hardened, hardenedChanged, err := rewriteFreedomFinalRulesPrivateEgress(reversed)
	if err != nil {
		return raw, false, err
	}
	if !reversedChanged && !hardenedChanged {
		return raw, false, nil
	}
	return hardened, true, nil
}

func hasNonLowercaseFreedomOutbound(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return false
	}
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok {
		return false
	}
	for _, ob := range outbounds {
		obj, ok := ob.(map[string]any)
		if !ok {
			continue
		}
		if proto, _ := obj["protocol"].(string); strings.EqualFold(proto, "freedom") && proto != "freedom" {
			return true
		}
	}
	return false
}

func stripRealityFinalmaskTcp() error {
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for i := range inbounds {
			updated, changed := stripRealityFinalmaskTcpFromStream(inbounds[i].StreamSettings)
			if !changed {
				continue
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbounds[i].Id).
				Update("stream_settings", updated).Error; err != nil {
				return err
			}
			log.Printf("InboundRealityFinalmaskTcpStrip: removed finalmask.tcp from REALITY inbound %d (%s)", inbounds[i].Id, inbounds[i].Tag)
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "InboundRealityFinalmaskTcpStrip"}).Error
	})
}

func stripRealityFinalmaskTcpFromStream(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return raw, false
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(raw), &stream); err != nil {
		return raw, false
	}
	if sec, _ := stream["security"].(string); sec != "reality" {
		return raw, false
	}
	finalmask, ok := stream["finalmask"].(map[string]any)
	if !ok {
		return raw, false
	}
	if tcp, _ := finalmask["tcp"].([]any); len(tcp) == 0 {
		return raw, false
	}
	delete(finalmask, "tcp")
	if len(finalmask) == 0 {
		delete(stream, "finalmask")
	}
	out, err := json.Marshal(stream)
	if err != nil {
		return raw, false
	}
	return string(out), true
}

func isAllowOnlyFinalRules(v any) bool {
	rules, ok := v.([]any)
	if !ok || len(rules) != 1 {
		return false
	}
	rule, ok := rules[0].(map[string]any)
	if !ok {
		return false
	}
	if action, _ := rule["action"].(string); action != "allow" {
		return false
	}
	for k := range rule {
		if k != "action" {
			return false
		}
	}
	return true
}

func normalizeClientJSONFields(obj map[string]any) {
	normalizeInt := func(key string) {
		raw, exists := obj[key]
		if !exists {
			return
		}
		s, ok := raw.(string)
		if !ok {
			return
		}
		trimmed := strings.ReplaceAll(strings.TrimSpace(s), " ", "")
		if trimmed == "" {
			delete(obj, key)
			return
		}
		if n, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			obj[key] = n
		} else {
			delete(obj, key)
		}
	}
	for _, k := range []string{"tgId", "limitIp", "totalGB", "expiryTime", "reset", "created_at", "updated_at"} {
		normalizeInt(k)
	}
}

func isTableEmpty(tableName string) (bool, error) {
	var count int64
	err := db.Table(tableName).Count(&count).Error
	return count == 0, err
}

func InitDB(dbPath string) error {
	var gormLogger logger.Interface
	if config.IsDebug() {
		gormLogger = logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold:             time.Second,
				LogLevel:                  logger.Info,
				IgnoreRecordNotFoundError: true,
				Colorful:                  true,
			},
		)
	} else {
		gormLogger = logger.Discard
	}
	c := &gorm.Config{Logger: gormLogger, DisableForeignKeyConstraintWhenMigrating: true}

	var err error
	switch config.GetDBKind() {
	case "postgres":
		dsn := config.GetDBDSN()
		if dsn == "" {
			return errors.New("XUI_DB_TYPE=postgres but XUI_DB_DSN is empty")
		}
		db, err = openPostgresWithRetry(dsn, c)
		if err != nil {
			return err
		}
	default:
		dir := path.Dir(dbPath)
		if err = os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err = cleanupSQLiteBackupDirs(filepath.Dir(dbPath)); err != nil {
			log.Printf("clean SQLite backup directories: %v", err)
		}

		sync := sqliteSynchronous()
		journal := sqliteJournalMode()
		dsn := dbPath + "?_journal_mode=" + journal + "&_busy_timeout=10000&_synchronous=" + sync + "&_txlock=immediate"
		db, err = gorm.Open(sqlite.Open(dsn), c)
		if err != nil {
			return err
		}
		if err := restrictSQLiteFilePerms(dbPath); err != nil {
			log.Printf("restrict SQLite file permissions: %v", err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}

		// Keep the baseline footprint bounded even on medium VDS hosts. Low-resource
		// mode tightens these further and uses file-backed temporary tables.
		cacheMB := envInt("XUI_DB_CACHE_MB", 16)
		mmapMB := envInt("XUI_DB_MMAP_MB", 64)
		tempStore := "MEMORY"
		if sys.IsLowResourceMode() {
			// SQLite cache and mmap settings are per connection. Keeping them small
			// prevents several concurrent panel requests from multiplying RSS.
			cacheMB = envInt("XUI_DB_CACHE_MB", 8)
			mmapMB = envInt("XUI_DB_MMAP_MB", 32)
			tempStore = "FILE"
		}
		pragmas := []string{
			"PRAGMA journal_mode=" + journal,
			"PRAGMA busy_timeout=10000",
			"PRAGMA synchronous=" + sync,
			fmt.Sprintf("PRAGMA cache_size=-%d", cacheMB*1024),
			fmt.Sprintf("PRAGMA mmap_size=%d", int64(mmapMB)*1024*1024),
			"PRAGMA temp_store=" + tempStore,
		}
		for _, p := range pragmas {
			if _, err := sqlDB.ExecContext(context.Background(), p); err != nil {
				return err
			}
		}
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	var maxOpen, maxIdle int
	switch config.GetDBKind() {
	case "postgres":
		maxOpen = envInt("XUI_DB_MAX_OPEN_CONNS", 25)
		maxIdle = envInt("XUI_DB_MAX_IDLE_CONNS", 25)
	default:
		maxOpen = envInt("XUI_DB_MAX_OPEN_CONNS", 4)
		maxIdle = envInt("XUI_DB_MAX_IDLE_CONNS", 2)
		if sys.IsLowResourceMode() {
			maxOpen = envInt("XUI_DB_MAX_OPEN_CONNS", 2)
			maxIdle = envInt("XUI_DB_MAX_IDLE_CONNS", 1)
		}
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if sys.IsLowResourceMode() {
		sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	} else {
		sqlDB.SetConnMaxIdleTime(30 * time.Minute)
	}

	if err := initModels(); err != nil {
		return err
	}

	isUsersEmpty, err := isTableEmpty("users")
	if err != nil {
		return err
	}
	if isUsersEmpty {
		if err := seedRandomSubscriptionPaths(); err != nil {
			return err
		}
	}
	if username, _, err := EnsureFirstUser(); err != nil {
		return err
	} else if username != "" {
		log.Print("Initial panel user created. Set login credentials with: x-ui setting -username <username> -password <password>")
	}
	return runSeeders(isUsersEmpty)
}

func normalizeApiTokenCreatedAtSeconds() error {
	return db.Model(&model.ApiToken{}).
		Where("created_at >= ?", model.ApiTokenUnixMillisecondsThreshold).
		UpdateColumn("created_at", gorm.Expr("created_at / ?", 1000)).Error
}

// openPostgresWithRetry retries the initial PostgreSQL connection with
// backoff so a database that starts slower than the panel (or drops out
// briefly) does not immediately kill the process and trip systemd's
// restart loop. Every failed attempt logs the real driver error, which
// used to be buried behind a generic startup failure.
func openPostgresWithRetry(dsn string, c *gorm.Config) (*gorm.DB, error) {
	delays := []time.Duration{0, 2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second}
	var lastErr error
	for i, delay := range delays {
		if delay > 0 {
			time.Sleep(delay)
		}
		conn, err := gorm.Open(postgres.Open(dsn), c)
		if err == nil {
			if i > 0 {
				log.Printf("postgres connection established on attempt %d/%d", i+1, len(delays))
			}
			return conn, nil
		}
		lastErr = err
		log.Printf("postgres connection attempt %d/%d failed: %v", i+1, len(delays), err)
	}
	return nil, fmt.Errorf("postgres unreachable after %d attempts: %w", len(delays), lastErr)
}

// The store holds client secrets, so it and its WAL/SHM side files stay
// owner-only. Best effort: a store the panel cannot chmod still opens.
func restrictSQLiteFilePerms(dbPath string) error {
	for _, name := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Chmod(name, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func sqliteJournalMode() string {
	switch strings.ToUpper(strings.TrimSpace(os.Getenv("XUI_DB_JOURNAL_MODE"))) {
	case "DELETE":
		return "DELETE"
	default:
		return "WAL"
	}
}

func backupSQLiteStepPages() int {
	if sqliteJournalMode() == "DELETE" {
		return 128
	}
	return -1
}

func cleanupSQLiteBackupDirs(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), sqliteBackupDirPrefix) {
			if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func sqliteSynchronous() string {
	switch strings.ToUpper(strings.TrimSpace(os.Getenv("XUI_DB_SYNCHRONOUS"))) {
	case "OFF":
		return "OFF"
	case "NORMAL":
		return "NORMAL"
	case "EXTRA":
		return "EXTRA"
	default:
		return "FULL"
	}
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func CloseDB() error {
	if db != nil {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

func GetDB() *gorm.DB {
	return db
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func IsSQLiteDB(file io.ReaderAt) (bool, error) {
	signature := []byte("SQLite format 3\x00")
	buf := make([]byte, len(signature))
	_, err := file.ReadAt(buf, 0)
	if err != nil {
		return false, err
	}
	return bytes.Equal(buf, signature), nil
}

func BackupSQLite(dstPath string) (err error) {
	if IsPostgres() {
		return errors.New("sqlite backup is unavailable for PostgreSQL")
	}
	if db == nil {
		return errors.New("database is not initialized")
	}
	if _, err := os.Lstat(dstPath); err == nil {
		return fmt.Errorf("sqlite backup destination already exists: %s", dstPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(dstPath)
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(dstPath)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), backupSQLiteTimeout)
	defer cancel()

	sourceDB, err := db.DB()
	if err != nil {
		return err
	}
	sourceConn, err := sourceDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer sourceConn.Close()

	destinationDB, err := sql.Open("sqlite3", dstPath)
	if err != nil {
		return err
	}
	defer destinationDB.Close()
	destinationConn, err := destinationDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer destinationConn.Close()

	return sourceConn.Raw(func(sourceDriver any) error {
		source, ok := sourceDriver.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("unexpected SQLite source connection type %T", sourceDriver)
		}
		return destinationConn.Raw(func(destinationDriver any) error {
			destination, ok := destinationDriver.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("unexpected SQLite destination connection type %T", destinationDriver)
			}
			backup, err := destination.Backup("main", source, "main")
			if err != nil {
				return err
			}
			finished := false
			defer func() {
				if !finished {
					_ = backup.Finish()
				}
			}()
			for {
				done, err := backup.Step(backupSQLiteStepPages())
				if err != nil {
					return err
				}
				if done {
					finished = true
					return backup.Finish()
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(10 * time.Millisecond):
				}
			}
		})
	})
}

func ValidateSQLiteDB(dbPath string) error {
	if _, err := os.Stat(dbPath); err != nil {
		return err
	}
	file, err := os.Open(dbPath)
	if err != nil {
		return err
	}
	valid, err := IsSQLiteDB(file)
	_ = file.Close()
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("not a SQLite database")
	}

	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	var res string
	if err := gdb.Raw("PRAGMA integrity_check;").Scan(&res).Error; err != nil {
		return err
	}
	if res != "ok" {
		return errors.New("sqlite integrity check failed: " + res)
	}
	return nil
}

const sqliteBackupDirPrefix = ".x-ui-backup-"

// mihomo_x25519 was saved but ignored until it began dropping ML-KEM, so rows set
// back then are cleared once rather than silently changing their subscriptions.
func resetPreexistingMihomoX25519() error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Host{}).Where("mihomo_x25519 = ?", true).Update("mihomo_x25519", false).Error; err != nil {
			return err
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: "HostMihomoX25519Reset"}).Error
	})
}

// SyncHostsFromExternalProxy carries the entries an inbound update changed onto the hosts
// still exactly as imported; a stream without the externalProxy key leaves hosts alone.
func SyncHostsFromExternalProxy(tx *gorm.DB, inboundId int, oldStreamSettings, newStreamSettings string) error {
	newImports, listed := externalProxyHosts(inboundId, newStreamSettings)
	if !listed {
		return nil
	}
	oldImports, _ := externalProxyHosts(inboundId, oldStreamSettings)
	gone, added := changedImports(oldImports, newImports)
	if len(gone) == 0 && len(added) == 0 {
		return nil
	}
	untouched, err := untouchedImportedHosts(tx, inboundId, oldImports)
	if err != nil || untouched == nil {
		return err
	}
	for i, old := range gone {
		host := untouched[old]
		if host == nil {
			continue
		}
		if i >= len(added) {
			if _, err := DeleteHosts(tx, "id = ?", host.Id); err != nil {
				return err
			}
			continue
		}
		added[i].Id, added[i].GroupId, added[i].SortOrder, added[i].CreatedAt = host.Id, host.GroupId, host.SortOrder, host.CreatedAt
		if err := tx.Save(added[i]).Error; err != nil {
			return err
		}
	}
	for _, host := range added[min(len(gone), len(added)):] {
		if err := tx.Create(host).Error; err != nil {
			return err
		}
	}
	return nil
}

// changedImports drops the entries both lists share and returns, in list order, the old
// positions that changed or went away and the new entries that changed or appeared.
func changedImports(oldImports, newImports []*model.Host) ([]int, []*model.Host) {
	kept := make([]bool, len(oldImports))
	var added []*model.Host
	for _, next := range newImports {
		match := -1
		for i, prev := range oldImports {
			if !kept[i] && sameImportedHost(prev, next) {
				match = i
				break
			}
		}
		if match < 0 {
			added = append(added, next)
			continue
		}
		kept[match] = true
	}
	var gone []int
	for i := range oldImports {
		if !kept[i] {
			gone = append(gone, i)
		}
	}
	return gone, added
}

// untouchedImportedHosts pairs each old import with a row still equal to it and alone in its
// group (nil where the admin edited, shared or deleted it); nil when no row qualifies.
func untouchedImportedHosts(tx *gorm.DB, inboundId int, imports []*model.Host) ([]*model.Host, error) {
	var stored []*model.Host
	if err := tx.Where("inbound_id = ?", inboundId).Order("sort_order asc, id asc").Find(&stored).Error; err != nil {
		return nil, err
	}
	paired := make([]*model.Host, len(imports))
	found := false
	for i, want := range imports {
		for _, host := range stored {
			if slices.Contains(paired, host) || !sameImportedHost(host, want) {
				continue
			}
			var groupRows int64
			if err := tx.Model(&model.Host{}).Where("group_id = ?", host.GroupId).Count(&groupRows).Error; err != nil {
				return nil, err
			}
			if groupRows == 1 {
				paired[i], found = host, true
				break
			}
		}
	}
	if !found {
		return nil, nil
	}
	return paired, nil
}

// externalProxyHosts maps each entry to the host an import creates; the bool reports whether
// the stream carries an externalProxy list at all, an empty one included.
func externalProxyHosts(inboundId int, streamSettings string) ([]*model.Host, bool) {
	return externalProxyHostsWithMapper(inboundId, streamSettings, externalProxyEntryToHost)
}

func externalProxyHostsWithMapper(inboundId int, streamSettings string, mapper func(int, int, map[string]any) *model.Host) ([]*model.Host, bool) {
	if strings.TrimSpace(streamSettings) == "" {
		return nil, false
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(streamSettings), &stream); err != nil {
		return nil, false
	}
	eps, listed := stream["externalProxy"].([]any)
	var hosts []*model.Host
	for i, raw := range eps {
		if ep, ok := raw.(map[string]any); ok {
			hosts = append(hosts, mapper(inboundId, i, ep))
		}
	}
	return hosts, listed
}

// sameImportedHost compares every field but row identity and placement, so a host the
// admin changed in any way no longer counts as the import; empty lists match nil.
func sameImportedHost(a, b *model.Host) bool {
	return reflect.DeepEqual(importedHostFields(*a), importedHostFields(*b))
}

// DeleteHosts deletes the hosts matching query and keeps each one's inbound in host_tombstones, in
// the caller's transaction, so a client still bound to one fails closed on that inbound alone.
func DeleteHosts(tx *gorm.DB, query any, args ...any) (int64, error) {
	var tombstones []model.HostTombstone
	if err := tx.Model(&model.Host{}).Select("id AS host_id, inbound_id").Where(query, args...).Scan(&tombstones).Error; err != nil {
		return 0, err
	}
	if len(tombstones) == 0 {
		return 0, nil
	}
	// migrate-db resyncs the hosts sequence to MAX(id), so a deleted id can return; keep its latest inbound.
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "host_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"inbound_id"}),
	}).CreateInBatches(&tombstones, 500).Error; err != nil {
		return 0, err
	}
	res := tx.Where(query, args...).Delete(&model.Host{})
	return res.RowsAffected, res.Error
}

// EnsureFirstUser creates the panel's first user with random credentials and returns
// them, or returns "" when one exists. Callers must not log the password.
func EnsureFirstUser() (username, password string, err error) {
	empty, err := isTableEmpty("users")
	if err != nil || !empty {
		return "", "", err
	}
	username, password = random.Seq(10), random.Seq(16)
	hashedPassword, err := crypto.HashPasswordAsBcrypt(password)
	if err != nil {
		return "", "", err
	}
	if err := db.Create(&model.User{Username: username, Password: hashedPassword}).Error; err != nil {
		return "", "", err
	}
	return username, password, nil
}

func importedHostFields(h model.Host) model.Host {
	h.Id, h.GroupId, h.SortOrder, h.CreatedAt, h.UpdatedAt = 0, "", 0, 0, 0
	for _, list := range []*[]string{&h.Tags, &h.Snis, &h.Alpn, &h.PinnedPeerCertSha256, &h.ExcludeFromSubTypes, &h.NodeGuids} {
		if len(*list) == 0 {
			*list = nil
		}
	}
	return h
}
