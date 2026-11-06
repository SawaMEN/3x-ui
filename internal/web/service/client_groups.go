package service

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

)

type GroupSummary struct {
	Name        string `json:"name"`
	ClientCount int    `json:"clientCount"`
	TrafficUsed int64  `json:"trafficUsed"`
	Up          int64  `json:"up"`
	Down        int64  `json:"down"`
}

func (s *ClientService) ListGroups() ([]GroupSummary, error) {
	db := database.GetDB()
	// email is unique in both clients and client_traffics, so the LEFT JOIN
	// never double-counts a client's traffic.
	derived, err := groupTrafficTotals(db, "c.group_name <> ''")
	if err != nil {
		return nil, err
	}
	var stored []model.ClientGroup
	if err := db.Find(&stored).Error; err != nil {
		return nil, err
	}
	type groupAgg struct {
		count int
		up    int64
		down  int64
	}
	baseUp := make(map[string]int64, len(stored))
	baseDown := make(map[string]int64, len(stored))
	merged := make(map[string]groupAgg, len(derived)+len(stored))
	for _, g := range stored {
		merged[g.Name] = groupAgg{}
		baseUp[g.Name] = g.ResetUp
		baseDown[g.Name] = g.ResetDown
	}
	for _, g := range derived {
		merged[g.Name] = groupAgg{count: g.ClientCount, up: g.Up, down: g.Down}
	}
	out := make([]GroupSummary, 0, len(merged))
	for name, agg := range merged {
		up := groupUsageSince(agg.up, baseUp[name])
		down := groupUsageSince(agg.down, baseDown[name])
		out = append(out, GroupSummary{Name: name, ClientCount: agg.count, TrafficUsed: common.TrafficUsage(up, down), Up: up, Down: down})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// adjustGroupBaselinesForRemovedTraffic shifts group baselines down by the clients'
// current counters so ListGroups totals survive a traffic reset or client delete (#5675).
func adjustGroupBaselinesForRemovedTraffic(tx *gorm.DB, emails []string) error {
	if len(emails) == 0 {
		return nil
	}
	type groupDelta struct {
		Name string
		Up   int64
		Down int64
	}
	totals := make(map[string]*groupDelta)
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		part, err := groupTrafficTotals(tx, "c.group_name <> '' AND c.email IN ?", batch)
		if err != nil {
			return err
		}
		for i := range part {
			if agg, ok := totals[part[i].Name]; ok {
				agg.Up = common.TrafficUsage(agg.Up, part[i].Up)
				agg.Down = common.TrafficUsage(agg.Down, part[i].Down)
			} else {
				totals[part[i].Name] = &groupDelta{Name: part[i].Name, Up: part[i].Up, Down: part[i].Down}
			}
		}
	}
	for name, d := range totals {
		if d.Up == 0 && d.Down == 0 {
			continue
		}
		res := tx.Model(&model.ClientGroup{}).Where("name = ?", name).Updates(map[string]any{
			"reset_up":   shiftGroupCounter("reset_up", d.Up, true),
			"reset_down": shiftGroupCounter("reset_down", d.Down, true),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			if err := tx.Create(&model.ClientGroup{Name: name, ResetUp: -d.Up, ResetDown: -d.Down}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ClientService) EmailsByGroup(name string) ([]string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return []string{}, nil
	}
	db := database.GetDB()
	var emails []string
	if err := db.Model(&model.ClientRecord{}).
		Where("group_name = ?", name).
		Order("email ASC").
		Pluck("email", &emails).Error; err != nil {
		return nil, err
	}
	if emails == nil {
		emails = []string{}
	}
	return emails, nil
}

func (s *ClientService) ResetGroupTraffic(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return common.NewError("group name is required")
	}
	return runSerializedTx(func(tx *gorm.DB) error {
		totals, err := groupTrafficTotals(tx, "c.group_name = ?", name)
		if err != nil {
			return err
		}
		var agg GroupSummary
		if len(totals) > 0 {
			agg = totals[0]
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"reset_up", "reset_down"}),
		}).Create(&model.ClientGroup{Name: name, ResetUp: agg.Up, ResetDown: agg.Down}).Error
	})
}

func groupExistsTx(tx *gorm.DB, name string) (bool, error) {
	var count int64
	if err := tx.Model(&model.ClientGroup{}).Where("name = ?", name).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := tx.Model(&model.ClientRecord{}).Where("group_name = ?", name).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *ClientService) CreateGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return common.NewError("group name is required")
	}
	return runSerializedTx(func(tx *gorm.DB) error {
		exists, err := groupExistsTx(tx, name)
		if err != nil {
			return err
		}
		if exists {
			return common.NewError("group already exists")
		}
		return tx.Create(&model.ClientGroup{Name: name}).Error
	})
}

func (s *ClientService) RenameGroup(oldName, newName string) (int, error) {
	oldName, newName = strings.TrimSpace(oldName), strings.TrimSpace(newName)
	if oldName == "" {
		return 0, common.NewError("old group name is required")
	}
	if newName == "" {
		return 0, common.NewError("new group name is required")
	}
	if oldName == newName {
		return 0, nil
	}
	return s.replaceGroupValue(oldName, newName)
}

func (s *ClientService) DeleteGroup(name string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, common.NewError("group name is required")
	}
	return s.replaceGroupValue(name, "")
}

func (s *ClientService) RemoveFromGroup(emails []string) (int, error) {
	return s.AddToGroup(emails, "")
}

func (s *ClientService) AddToGroup(emails []string, group string) (int, error) {
	group = strings.TrimSpace(group)
	emails = trimmedUniqueEmails(emails)
	if len(emails) == 0 {
		return 0, nil
	}
	var affected int
	err := runSerializedTx(func(tx *gorm.DB) error {
		var records []model.ClientRecord
		for _, batch := range chunkStrings(emails, sqlInChunk) {
			var rows []model.ClientRecord
			if err := tx.Where("email IN ?", batch).
				Where("group_name IS NULL OR group_name <> ?", group).Find(&rows).Error; err != nil {
				return err
			}
			records = append(records, rows...)
		}
		if len(records) == 0 {
			return nil
		}
		changed := make([]string, 0, len(records))
		for _, rec := range records {
			changed = append(changed, rec.Email)
		}
		// Historical usage stays with the old group. The destination starts
		// counting these clients from the moment they join, even after resets.
		if err := adjustGroupBaselinesForRemovedTraffic(tx, changed); err != nil {
			return err
		}
		if group != "" {
			var up, down int64
			for _, batch := range chunkStrings(changed, sqlInChunk) {
				totals, err := groupTrafficTotals(tx, "c.email IN ?", batch)
				if err != nil {
					return err
				}
				for _, row := range totals {
					up = common.TrafficUsage(up, row.Up)
					down = common.TrafficUsage(down, row.Down)
				}
			}
			if err := shiftGroupBaseline(tx, group, up, down); err != nil {
				return err
			}
		}
		if err := applyClientGroupTx(tx, records, group); err != nil {
			return err
		}
		affected = len(records)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func shiftGroupBaseline(tx *gorm.DB, name string, up, down int64) error {
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "name"}},
		DoUpdates: clause.Assignments(map[string]any{
			"reset_up":   shiftGroupCounter("client_groups.reset_up", up, false),
			"reset_down": shiftGroupCounter("client_groups.reset_down", down, false),
		}),
	}).Create(&model.ClientGroup{Name: name, ResetUp: up, ResetDown: down}).Error
}

// Keep normalized records and the legacy inbound projection in one transaction.
// A malformed projection must fail rather than leave two different group names.
func applyClientGroupTx(tx *gorm.DB, records []model.ClientRecord, group string) error {
	emails := make([]string, 0, len(records))
	emailSet := make(map[string]struct{}, len(records))
	for _, rec := range records {
		emails = append(emails, rec.Email)
		emailSet[rec.Email] = struct{}{}
	}
	ids := make(map[int]struct{})
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		if err := tx.Model(&model.ClientRecord{}).Where("email IN ?", batch).
			Updates(map[string]any{"group_name": group, "updated_at": time.Now().UnixMilli()}).Error; err != nil {
			return err
		}
		var part []int
		if err := tx.Table("client_inbounds").
			Joins("JOIN clients ON clients.id = client_inbounds.client_id").
			Where("clients.email IN ?", batch).Distinct("client_inbounds.inbound_id").
			Pluck("inbound_id", &part).Error; err != nil {
			return err
		}
		for _, id := range part {
			ids[id] = struct{}{}
		}
	}
	ordered := make([]int, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Ints(ordered)
	for _, id := range ordered {
		var ib model.Inbound
		if err := tx.First(&ib, id).Error; err != nil {
			return err
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
			return fmt.Errorf("inbound %d settings: %w", id, err)
		}
		clients, _ := settings["clients"].([]any)
		modified := false
		for _, item := range clients {
			cm, ok := item.(map[string]any)
			if !ok {
				continue
			}
			email, _ := cm["email"].(string)
			if _, hit := emailSet[email]; !hit {
				continue
			}
			if group == "" {
				delete(cm, "group")
			} else {
				cm["group"] = group
			}
			modified = true
		}
		if modified {
			raw, err := json.Marshal(settings)
			if err != nil {
				return err
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", id).UpdateColumn("settings", string(raw)).Error; err != nil {
				return err
			}
		}
		if ib.NodeID != nil {
			if err := (&NodeService{}).MarkNodeDirtyTx(tx, *ib.NodeID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ClientService) replaceGroupValue(oldName, newName string) (int, error) {
	var affected int
	err := runSerializedTx(func(tx *gorm.DB) error {
		exists, err := groupExistsTx(tx, oldName)
		if err != nil {
			return err
		}
		if !exists {
			return common.NewError("group not found")
		}
		if newName != "" {
			exists, err := groupExistsTx(tx, newName)
			if err != nil {
				return err
			}
			if exists {
				return common.NewError("group already exists")
			}
		}
		var records []model.ClientRecord
		if err := tx.Where("group_name = ?", oldName).Find(&records).Error; err != nil {
			return err
		}
		if err := applyClientGroupTx(tx, records, newName); err != nil {
			return err
		}
		if newName == "" {
			if err := tx.Where("name = ?", oldName).Delete(&model.ClientGroup{}).Error; err != nil {
				return err
			}
		} else {
			res := tx.Model(&model.ClientGroup{}).Where("name = ?", oldName).Update("name", newName)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				if err := tx.Create(&model.ClientGroup{Name: newName}).Error; err != nil {
					return err
				}
			}
		}
		affected = len(records)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// Negative baselines retain usage of removed/reset clients. Their difference
// can exceed int64 even though both stored columns are individually valid.
func groupUsageSince(counter, baseline int64) int64 {
	counter = max(counter, 0)
	if baseline < 0 && counter > math.MaxInt64+baseline {
		return math.MaxInt64
	}
	return max(counter-baseline, 0)
}

// Keep the normal path in SQL. PostgreSQL SUM(bigint) is exact NUMERIC; clamp
// only the display aggregate. SQLite SUM may overflow even with valid rows,
// so its exceptional path streams exact integers, retaining O(groups) memory.
func groupTrafficTotals(tx *gorm.DB, condition string, args ...any) ([]GroupSummary, error) {
	query := tx.Table("clients AS c").Joins("LEFT JOIN client_traffics ct ON ct.email = c.email").Where(condition, args...)
	up, down := "COALESCE(SUM(ct.up), 0)", "COALESCE(SUM(ct.down), 0)"
	if tx.Name() == "postgres" {
		up = fmt.Sprintf("LEAST(%s, %d)", up, int64(math.MaxInt64))
		down = fmt.Sprintf("LEAST(%s, %d)", down, int64(math.MaxInt64))
	}
	var totals []GroupSummary
	err := query.Session(&gorm.Session{}).Select("COALESCE(c.group_name, '') AS name, COUNT(*) AS client_count, " + up + " AS up, " + down + " AS down").Group("c.group_name").Scan(&totals).Error
	if err == nil {
		return totals, nil
	}
	if tx.Name() != "sqlite" || !strings.Contains(err.Error(), "integer overflow") {
		return nil, err
	}
	rows, err := query.Session(&gorm.Session{}).Select("COALESCE(c.group_name, ''), COALESCE(ct.up, 0), COALESCE(ct.down, 0)").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grouped := map[string]*GroupSummary{}
	for rows.Next() {
		var name string
		var up, down int64
		if err := rows.Scan(&name, &up, &down); err != nil {
			return nil, err
		}
		row := grouped[name]
		if row == nil {
			row = &GroupSummary{Name: name}
			grouped[name] = row
		}
		row.ClientCount++
		row.Up = common.TrafficUsage(row.Up, up)
		row.Down = common.TrafficUsage(row.Down, down)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	totals = make([]GroupSummary, 0, len(grouped))
	for _, row := range grouped {
		totals = append(totals, *row)
	}
	return totals, nil
}

func shiftGroupCounter(column string, amount int64, subtract bool) clause.Expr {
	if subtract {
		return gorm.Expr("CASE WHEN "+column+" < ? THEN ? ELSE "+column+" - ? END", int64(math.MinInt64)+amount, int64(math.MinInt64), amount)
	}
	return gorm.Expr("CASE WHEN "+column+" > ? THEN ? ELSE "+column+" + ? END", int64(math.MaxInt64)-amount, int64(math.MaxInt64), amount)
}
