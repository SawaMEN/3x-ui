package database

import (
	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// Repair only rows still exactly equal to the old importer output and alone in
// their group. An administrator's edits, shared groups and deleted rows win.
func repairImportedHostTLS() error {
	const name = "ImportedHostTLSOverrides"
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.HistoryOfSeeders{}).Where("seeder_name = ?", name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		var inbounds []model.Inbound
		if err := tx.Select("id", "stream_settings").Find(&inbounds).Error; err != nil {
			return err
		}
		for _, inbound := range inbounds {
			legacy, listed := externalProxyHostsWithMapper(inbound.Id, inbound.StreamSettings, legacyExternalProxyEntryToHost)
			if !listed || len(legacy) == 0 {
				continue
			}
			desired, _ := externalProxyHosts(inbound.Id, inbound.StreamSettings)
			paired, err := untouchedImportedHosts(tx, inbound.Id, legacy)
			if err != nil {
				return err
			}
			for i, old := range paired {
				if old == nil || sameImportedHost(old, desired[i]) {
					continue
				}
				next := desired[i]
				next.Id, next.GroupId, next.SortOrder, next.CreatedAt = old.Id, old.GroupId, old.SortOrder, old.CreatedAt
				if err := tx.Model(&model.Host{}).Where("id = ?", old.Id).Select("*").Updates(next).Error; err != nil {
					return err
				}
			}
		}
		return tx.Create(&model.HistoryOfSeeders{SeederName: name}).Error
	})
}
