package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

type vkTurnProxyTrafficBinding struct {
	inboundID int
	client    VKTurnProxyClient
}

func vkTurnProxyClientToModelClient(client *VKTurnProxyClient) model.Client {
	return model.Client{ID: client.ID, Email: client.Email, LimitIP: client.LimitIP, TotalGB: client.TotalGB, ExpiryTime: client.ExpiryTime, Enable: client.Enable, TgID: client.TgID, SubID: client.SubID, Comment: client.Comment, Reset: client.Reset, CreatedAt: client.CreatedAt, UpdatedAt: client.UpdatedAt}
}

func (s *InboundService) addVKTurnProxyClientTraffic(tx *gorm.DB, inboundID int, client *VKTurnProxyClient) error {
	c := vkTurnProxyClientToModelClient(client)
	return s.AddClientStat(tx, inboundID, &c)
}

func (s *InboundService) updateVKTurnProxyClientTraffic(tx *gorm.DB, oldEmail string, client *VKTurnProxyClient) error {
	c := vkTurnProxyClientToModelClient(client)
	if err := s.UpdateClientStat(tx, oldEmail, &c); err != nil {
		return err
	}
	if oldEmail != client.Email {
		return s.UpdateClientIPs(tx, oldEmail, client.Email)
	}
	return nil
}

func (s *InboundService) delVKTurnProxyClientTraffic(tx *gorm.DB, email string) error {
	if err := s.DelClientStat(tx, email); err != nil {
		return err
	}
	return s.DelClientIPs(tx, email)
}

func (s *InboundService) syncVKTurnProxyClientTrafficRows(tx *gorm.DB, inbounds []*model.Inbound) (bool, error) {
	bindings := make(map[string]vkTurnProxyTrafficBinding)
	for _, inbound := range inbounds {
		if inbound == nil || inbound.Protocol != model.VKTurnProxy {
			continue
		}
		settings, err := s.getVKTurnProxySettings(inbound.Settings)
		if err != nil {
			logger.Warningf("skip vk-turn-proxy client traffic sync for inbound %d: %v", inbound.Id, err)
			continue
		}
		for _, client := range settings.Clients {
			email := strings.TrimSpace(client.Email)
			if email != "" {
				bindings[strings.ToLower(email)] = vkTurnProxyTrafficBinding{inboundID: inbound.Id, client: client}
			}
		}
	}
	if len(bindings) == 0 {
		return false, nil
	}
	emails := make([]string, 0, len(bindings))
	for _, b := range bindings {
		emails = append(emails, b.client.Email)
	}
	var rows []xray.ClientTraffic
	if err := tx.Where("email IN ?", emails).Find(&rows).Error; err != nil {
		return false, err
	}
	existing := make(map[string]xray.ClientTraffic, len(rows))
	for _, row := range rows {
		existing[strings.ToLower(row.Email)] = row
	}
	changed := false
	for key, b := range bindings {
		if row, ok := existing[key]; ok {
			updates := map[string]any{}
			if row.InboundId != b.inboundID {
				updates["inbound_id"] = b.inboundID
			}
			if row.Enable != b.client.Enable {
				updates["enable"] = b.client.Enable
			}
			if row.Total != b.client.TotalGB {
				updates["total"] = b.client.TotalGB
			}
			if row.ExpiryTime != b.client.ExpiryTime {
				updates["expiry_time"] = b.client.ExpiryTime
			}
			if row.Reset != b.client.Reset {
				updates["reset"] = b.client.Reset
			}
			if len(updates) > 0 {
				if err := tx.Model(&xray.ClientTraffic{}).Where("email = ?", b.client.Email).Updates(updates).Error; err != nil {
					return changed, err
				}
				changed = true
			}
			continue
		}
		if err := s.addVKTurnProxyClientTraffic(tx, b.inboundID, &b.client); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

// BuildVKTurnProxyInboundTraffics keeps VK TURN inbound totals visible in the existing traffic UI.
// It deliberately does not consume or modify any WireGuard-specific statistics API.
func (s *InboundService) BuildVKTurnProxyInboundTraffics(clientTraffics []*xray.ClientTraffic) ([]*xray.Traffic, error) {
	if len(clientTraffics) == 0 {
		return nil, nil
	}
	up, down := map[int]int64{}, map[int]int64{}
	ids := make([]int, 0)
	seen := map[int]struct{}{}
	for _, ct := range clientTraffics {
		if ct == nil {
			continue
		}
		up[ct.InboundId] += ct.Up
		down[ct.InboundId] += ct.Down
		if _, ok := seen[ct.InboundId]; !ok {
			seen[ct.InboundId] = struct{}{}
			ids = append(ids, ct.InboundId)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var inbounds []*model.Inbound
	if err := database.GetDB().Select("id", "tag").Where("id IN ? AND protocol = ?", ids, model.VKTurnProxy).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	out := make([]*xray.Traffic, 0, len(inbounds))
	for _, ib := range inbounds {
		if tag := strings.TrimSpace(ib.Tag); tag != "" {
			out = append(out, &xray.Traffic{IsInbound: true, Tag: tag, Up: up[ib.Id], Down: down[ib.Id]})
		}
	}
	return out, nil
}

func (s *InboundService) MigrationBackfillVKTurnProxyClientTraffics() {
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Where("protocol = ?", model.VKTurnProxy).Find(&inbounds).Error; err != nil {
		logger.Warningf("vk-turn-proxy traffic backfill query failed: %v", err)
		return
	}
	for _, ib := range inbounds {
		settings, err := s.getVKTurnProxySettings(ib.Settings)
		if err != nil {
			logger.Warningf("skip vk-turn-proxy traffic backfill for inbound %d: %v", ib.Id, err)
			continue
		}
		for _, client := range settings.Clients {
			if strings.TrimSpace(client.Email) == "" {
				continue
			}
			mc := vkTurnProxyClientToModelClient(&client)
			var traffic xray.ClientTraffic
			err = db.Where("email = ?", mc.Email).First(&traffic).Error
			switch {
			case err == nil:
				err = db.Model(&traffic).Updates(map[string]any{"inbound_id": ib.Id, "enable": mc.Enable, "total": mc.TotalGB, "expiry_time": mc.ExpiryTime, "reset": mc.Reset}).Error
			case errors.Is(err, gorm.ErrRecordNotFound):
				err = db.Create(&xray.ClientTraffic{InboundId: ib.Id, Email: mc.Email, Enable: mc.Enable, ExpiryTime: mc.ExpiryTime, Total: mc.TotalGB, Reset: mc.Reset}).Error
			}
			if err != nil {
				logger.Warningf("vk-turn-proxy traffic backfill failed for client %s: %v", mc.Email, err)
			}
		}
	}
}
