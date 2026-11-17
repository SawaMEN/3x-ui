package service

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// CommitCoreTraffic persists inbound, client and outbound traffic in one DB
// transaction. Counter acquisition and acknowledgement belong to the adapter.
func (s *InboundService) CommitCoreTraffic(inboundTraffics []*coremodel.Traffic, clientTraffics []*coremodel.ClientTraffic) error {
	return submitTrafficWrite(func() error {
		return database.GetDB().Transaction(func(tx *gorm.DB) error {
			if err := s.addInboundTraffic(tx, inboundTraffics); err != nil {
				return fmt.Errorf("persist inbound traffic: %w", err)
			}
			if err := s.addClientTrafficStrict(tx, clientTraffics); err != nil {
				return fmt.Errorf("persist client traffic: %w", err)
			}
			if err := addOutboundTrafficStrict(tx, inboundTraffics); err != nil {
				return fmt.Errorf("persist outbound traffic: %w", err)
			}
			return nil
		})
	})
}

// addClientTrafficStrict mirrors the accounting path but never swallows an
// UPDATE error. Otherwise a failed client update looks like a successful
// transaction to the Xray delta reader and those bytes are lost.
func (s *InboundService) addClientTrafficStrict(tx *gorm.DB, traffics []*coremodel.ClientTraffic) error {
	if len(traffics) == 0 {
		return nil
	}

	emails, trafficByEmail := aggregateClientTraffic(traffics)
	if len(emails) == 0 {
		return nil
	}
	dbClientTraffics, err := loadClientTrafficRows(tx, emails)
	if err != nil {
		return err
	}
	if len(dbClientTraffics) == 0 {
		return nil
	}
	var convertedExpiryByEmail map[string]int64
	dbClientTraffics, convertedExpiryByEmail, err = s.adjustTraffics(tx, dbClientTraffics)
	if err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	for _, ct := range dbClientTraffics {
		if ct == nil {
			continue
		}
		traffic, ok := trafficByEmail[ct.Email]
		if !ok || (traffic.Up == 0 && traffic.Down == 0) {
			continue
		}

		// The SQL clamp cannot protect PostgreSQL from overflowing while it
		// evaluates "stored + delta". Limit each delta against the durable row
		// first, just like the non-strict AddTraffic path does. Otherwise a
		// near-saturated client makes the transaction fail forever and the Xray
		// baseline keeps rolling back to the same unacknowledged delta.
		up := clampTrafficDeltaToStored(ct.Up, traffic.Up)
		down := clampTrafficDeltaToStored(ct.Down, traffic.Down)
		if err := tx.Exec(
			fmt.Sprintf(
				`UPDATE client_traffics SET up = %s, down = %s, last_online = %s WHERE email = ?`,
				database.ClampedAddExpr("up"),
				database.ClampedAddExpr("down"),
				database.GreatestExpr("last_online", "?"),
			),
			up, down, now, ct.Email,
		).Error; err != nil {
			return err
		}
	}

	for _, email := range slices.Sorted(maps.Keys(convertedExpiryByEmail)) {
		if err := tx.Exec(
			`UPDATE client_traffics SET expiry_time = ? WHERE email = ? AND expiry_time < 0`,
			convertedExpiryByEmail[email], email,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// Aggregate before querying so duplicate sources use one SQL variable and zero
// snapshots cannot start a delayed subscription. Never mutate adapter snapshots.
func aggregateClientTraffic(traffics []*coremodel.ClientTraffic) ([]string, map[string]*coremodel.ClientTraffic) {
	emails := make([]string, 0, len(traffics))
	byEmail := make(map[string]*coremodel.ClientTraffic, len(traffics))
	for _, traffic := range traffics {
		if traffic == nil || traffic.Email == "" || (traffic.Up <= 0 && traffic.Down <= 0) {
			continue
		}
		item := byEmail[traffic.Email]
		if item == nil {
			item = &coremodel.ClientTraffic{Email: traffic.Email}
			byEmail[traffic.Email] = item
			emails = append(emails, traffic.Email)
		}
		item.Up = sumTrafficDelta(item.Up, traffic.Up)
		item.Down = sumTrafficDelta(item.Down, traffic.Down)
	}
	return emails, byEmail
}

func loadClientTrafficRows(tx *gorm.DB, emails []string) ([]*coremodel.ClientTraffic, error) {
	rows := make([]*coremodel.ClientTraffic, 0, len(emails))
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var page []*coremodel.ClientTraffic
		if err := tx.Where("email IN ?", batch).Find(&page).Error; err != nil {
			return nil, err
		}
		rows = append(rows, page...)
	}
	// Consistent lock order across batches and callers prevents deadlocks.
	slices.SortFunc(rows, func(a, b *coremodel.ClientTraffic) int { return cmp.Compare(a.Id, b.Id) })
	return rows, nil
}

// clampTrafficDeltaToStored returns the part of delta that can still be added
// to an already stored counter without making the intermediate SQL addition
// exceed TrafficMax. Invalid/negative stored values are treated as zero usage;
// negative deltas are never applied.
func clampTrafficDeltaToStored(stored, delta int64) int64 {
	if delta <= 0 || stored >= database.TrafficMax {
		return 0
	}
	if stored < 0 {
		stored = 0
	}
	remaining := database.TrafficMax - stored
	if delta > remaining {
		return remaining
	}
	return delta
}

func addOutboundTrafficStrict(tx *gorm.DB, traffics []*coremodel.Traffic) error {
	for _, traffic := range traffics {
		if traffic == nil || !traffic.IsOutbound || traffic.Tag == "" {
			continue
		}

		var stored model.OutboundTraffics
		if err := tx.Model(&model.OutboundTraffics{}).
			Where("tag = ?", traffic.Tag).
			FirstOrCreate(&stored, model.OutboundTraffics{Tag: traffic.Tag}).Error; err != nil {
			return err
		}

		stored.Tag = traffic.Tag
		stored.Up = sumTrafficDelta(stored.Up, traffic.Up)
		stored.Down = sumTrafficDelta(stored.Down, traffic.Down)
		stored.Total = sumTrafficDelta(stored.Up, stored.Down)
		if err := tx.Save(&stored).Error; err != nil {
			return err
		}
	}
	return nil
}
