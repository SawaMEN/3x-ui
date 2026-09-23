package service

import (
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/psiphon"
)

func (s *InboundService) DesiredPsiphonInstances() ([]psiphon.Instance, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Model(model.Inbound{}).
		Where("protocol = ? AND enable = ? AND node_id IS NULL", model.Psiphon, true).
		Find(&inbounds).Error; err != nil {
		return nil, err
	}
	out := make([]psiphon.Instance, 0, len(inbounds))
	for _, ib := range inbounds {
		if inst, ok := psiphon.InstanceFromInbound(ib); ok {
			out = append(out, inst)
		}
	}
	return out, nil
}
