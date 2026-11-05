package tgbot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/common"
)

const telemtWeeklyCheckpointPath = "/etc/x-ui/telemt-weekly-report.json"

type telemtWeeklyCheckpoint struct {
	At    time.Time        `json:"at"`
	Bytes map[string]int64 `json:"bytes"`
}

type telemtWeeklyUsage struct {
	Email string
	Bytes int64
}

func weeklyUsageSince(previous map[string]int64, current map[string]int64) ([]telemtWeeklyUsage, int64) {
	rows := make([]telemtWeeklyUsage, 0, len(current))
	var total int64
	for email, now := range current {
		if now < 0 {
			continue
		}
		delta := now - previous[email]
		if delta < 0 {
			// Telemt can reset its own per-user counters on restart; never
			// interpret a reset as a negative week's consumption.
			delta = now
		}
		total += delta
		if delta > 0 {
			rows = append(rows, telemtWeeklyUsage{Email: email, Bytes: delta})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Bytes != rows[j].Bytes {
			return rows[i].Bytes > rows[j].Bytes
		}
		return rows[i].Email < rows[j].Email
	})
	return rows, total
}

func telemtWeeklyClientBytes() (map[string]int64, error) {
	db := database.GetDB()
	if db == nil {
		return nil, fmt.Errorf("MTProto weekly statistics: database is unavailable")
	}
	var rows []struct {
		Email string
		Up    int64
		Down  int64
	}
	err := db.Table("client_traffics AS ct").
		Select("DISTINCT ct.email, ct.up, ct.down").
		Joins("JOIN clients AS c ON c.email = ct.email").
		Joins("JOIN client_inbounds AS ci ON ci.client_id = c.id").
		Joins("JOIN inbounds AS ib ON ib.id = ci.inbound_id").
		Where("ib.protocol = ? AND ib.node_id IS NULL", model.MTProto).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, row := range rows {
		if row.Email == "" {
			continue
		}
		result[row.Email] = row.Up + row.Down
	}
	return result, nil
}

func saveTelemtWeeklyCheckpoint(snapshot telemtWeeklyCheckpoint) error {
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(telemtWeeklyCheckpointPath), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(telemtWeeklyCheckpointPath), ".telemt-weekly-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), telemtWeeklyCheckpointPath)
}

func (t *Tgbot) SendWeeklyTelemtReport() {
	if !t.IsRunning() {
		return
	}
	current, err := telemtWeeklyClientBytes()
	if err != nil {
		return
	}
	var previous telemtWeeklyCheckpoint
	if data, err := os.ReadFile(telemtWeeklyCheckpointPath); err == nil {
		if json.Unmarshal(data, &previous) != nil {
			// Refuse to clobber an invalid checkpoint; operators can back it
			// up and repair it instead of silently losing weekly history.
			return
		}
	} else if !os.IsNotExist(err) {
		return
	}
	now := time.Now().UTC()
	if err := saveTelemtWeeklyCheckpoint(telemtWeeklyCheckpoint{At: now, Bytes: current}); err != nil {
		return
	}
	var message strings.Builder
	message.WriteString("MTProto · недельная сводка\n")
	if previous.At.IsZero() {
		message.WriteString("Первый снимок счётчиков сохранён. Недельные дельты будут доступны со следующего отчёта.")
	} else {
		rows, total := weeklyUsageSince(previous.Bytes, current)
		fmt.Fprintf(&message, "Период: %s — %s UTC\n", previous.At.Format("02.01 15:04"), now.Format("02.01 15:04"))
		fmt.Fprintf(&message, "Трафик: %s\nАктивных клиентов: %d\n", common.FormatTraffic(total), len(rows))
		if len(rows) > 5 {
			rows = rows[:5]
		}
		for i, row := range rows {
			fmt.Fprintf(&message, "%d. %s — %s\n", i+1, row.Email, common.FormatTraffic(row.Bytes))
		}
	}
	for _, admin := range adminSnapshot() {
		t.sendNotice(admin, message.String())
	}
}
