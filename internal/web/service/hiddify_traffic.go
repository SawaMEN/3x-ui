package service

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// commitHiddifySnapshot makes counter baselines and accumulated panel traffic
// one transaction. An interrupted response or panel restart cannot replay a
// snapshot, and a new core process starts a fresh baseline without clearing DB
// totals. The core owns in-memory counters; unobserved bytes on abrupt core
// death cannot be recovered by its current API.
func (s *SingBoxService) commitHiddifySnapshot(epoch string, stats []singbox.V2RayStat) error {
	if epoch == "" {
		return fmt.Errorf("empty hiddify statistics epoch")
	}
	seen := make(map[string]bool, len(stats))
	filtered := make([]singbox.V2RayStat, 0, len(stats))
	for _, stat := range stats {
		kind, rest, ok := strings.Cut(stat.Name, ">>>")
		i := strings.LastIndex(rest, ">>>traffic>>>")
		if !ok || (kind != "user" && kind != "inbound") || i <= 0 {
			continue
		}
		direction := rest[i+len(">>>traffic>>>"):]
		if direction != "uplink" && direction != "downlink" {
			continue
		}
		if stat.Value < 0 || seen[stat.Name] {
			return fmt.Errorf("invalid or duplicate hiddify counter %q", stat.Name)
		}
		seen[stat.Name] = true
		filtered = append(filtered, stat)
	}
	var restart, disabled bool
	var nodes []int
	var plans []trafficInboundUpdatePlan
	err := submitTrafficWrite(func() error {
		svc := &singBoxInboundService
		err := database.GetDB().Transaction(func(tx *gorm.DB) error {
			var stored []model.CoreTrafficCheckpoint
			if err := tx.Where("name LIKE ?", "hiddify:%").Find(&stored).Error; err != nil {
				return err
			}
			previous := make(map[string]model.CoreTrafficCheckpoint, len(stored))
			for _, row := range stored {
				previous[row.Name] = row
			}
			// Bind source counters to the stable traffic row so an email edit
			// while the old core is running cannot discard its remaining bytes.
			var owners []struct {
				Id    int
				Email string
			}
			if err := tx.Model(&coremodel.ClientTraffic{}).Select("id", "email").Find(&owners).Error; err != nil {
				return err
			}
			ownerIDs, ownerEmails := map[string]int{}, map[int]string{}
			for _, owner := range owners {
				ownerIDs[owner.Email] = owner.Id
				ownerEmails[owner.Id] = owner.Email
			}
			sourceOwners := map[string]int{}
			for _, row := range stored {
				if row.Epoch == epoch && row.TrafficID > 0 {
					i := strings.LastIndex(row.Name, ">>>traffic>>>")
					if i > 0 {
						sourceOwners[row.Name[:i]] = row.TrafficID
					}
				}
			}
			deltas := make([]singbox.V2RayStat, 0, len(filtered))
			checkpoints := make([]model.CoreTrafficCheckpoint, 0, len(filtered))
			for _, stat := range filtered {
				name := "hiddify:" + stat.Name
				row := previous[name]
				delta := stat.Value
				if row.Epoch == epoch && stat.Value >= row.Value {
					delta -= row.Value
				}
				destination := stat.Name
				trafficID := 0
				if strings.HasPrefix(stat.Name, "user>>>") {
					i := strings.LastIndex(stat.Name, ">>>traffic>>>")
					source := stat.Name[len("user>>>"):i]
					trafficID = sourceOwners["hiddify:"+stat.Name[:i]]
					if trafficID == 0 {
						trafficID = ownerIDs[source]
					}
					if trafficID > 0 {
						if email := ownerEmails[trafficID]; email != "" {
							destination = "user>>>" + email + stat.Name[i:]
						} else {
							destination = ""
						}
					}
				}
				if destination != "" {
					deltas = append(deltas, singbox.V2RayStat{Name: destination, Value: delta})
				}
				checkpoints = append(checkpoints, model.CoreTrafficCheckpoint{Name: name, Epoch: epoch, Value: stat.Value, TrafficID: trafficID})
			}
			if len(checkpoints) > 0 {
				if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoUpdates: clause.AssignmentColumns([]string{"epoch", "value", "traffic_id"})}).CreateInBatches(checkpoints, 200).Error; err != nil {
					return err
				}
			}
			// Retire counters from older processes without accumulating epoch history.
			if err := tx.Where("name LIKE ? AND epoch <> ?", "hiddify:%", epoch).Delete(&model.CoreTrafficCheckpoint{}).Error; err != nil {
				return err
			}

			inbounds, clients := splitHiddifyStats(deltas)
			if err := svc.addInboundTraffic(tx, inbounds); err != nil {
				return err
			}
			return svc.addClientTraffic(tx, clients)
		})
		if err != nil {
			return err
		}
		var maintenanceErr error
		restart, disabled, nodes, plans, maintenanceErr = svc.maintainTrafficLocked()
		return maintenanceErr
	})
	if err != nil {
		return err
	}
	restart = singBoxInboundService.applyTrafficRemotePlans(plans) || restart
	if len(nodes) > 0 {
		singBoxInboundService.restartRemoteNodesOnDisable(nodes)
	}
	if restart || disabled {
		s.SetToNeedRestart()
	}
	return nil
}

// The configuration apply is deferred; revoke already active sessions as soon
// as disabled DB state is durable. Retry on the next poll if the API fails.
// Network requests stay off the serial traffic writer.
func (s *SingBoxService) enforceHiddifySessions(ctx context.Context) error {
	var users []string
	var tags []string
	db := database.GetDB()
	if err := db.Model(&model.ClientRecord{}).Where("enable = ?", false).Pluck("email", &users).Error; err != nil {
		return err
	}
	var trafficUsers []string
	if err := db.Model(&coremodel.ClientTraffic{}).Where("enable = ?", false).Pluck("email", &trafficUsers).Error; err != nil {
		return err
	}
	if err := db.Model(&model.Inbound{}).Where("enable = ? AND node_id IS NULL", false).Pluck("tag", &tags).Error; err != nil {
		return err
	}
	disabledUsers, disabledTags := map[string]bool{}, map[string]bool{}
	for _, user := range append(users, trafficUsers...) {
		if user != "" {
			disabledUsers[user] = true
		}
	}
	for _, tag := range tags {
		if tag != "" {
			disabledTags[tag] = true
		}
	}
	if len(disabledUsers) == 0 && len(disabledTags) == 0 {
		return nil
	}
	_, err := s.disconnectHiddify(ctx, func(session singbox.ActiveSession) bool {
		return disabledUsers[session.User] || disabledTags[session.Inbound]
	})
	return err
}
