package service

import (
	"context"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/common"
)

func (s *SettingService) getSettingContext(ctx context.Context, key string) (*model.Setting, error) {
	db := database.GetDB().WithContext(ctx)
	setting := &model.Setting{}
	err := db.Model(model.Setting{}).Where("key = ?", key).First(setting).Error
	if err != nil {
		return nil, err
	}
	return setting, nil
}

func (s *SettingService) getStringContext(ctx context.Context, key string) (string, error) {
	setting, err := s.getSettingContext(ctx, key)
	if database.IsNotFound(err) {
		value, ok := defaultValueMap[key]
		if !ok {
			return "", common.NewErrorf("key <%v> not in defaultValueMap", key)
		}
		return value, nil
	} else if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (s *SettingService) GetCoreTypeContext(ctx context.Context) (string, error) {
	coreType, err := s.getStringContext(ctx, "coreType")
	if err != nil {
		return "", err
	}
	coreType = canonicalCoreType(coreType)
	if coreType == CoreTypeXray {
		return coreType, nil
	} // Read-only legacy value for offline migration.
	if !isSupportedCoreType(coreType) {
		return CoreTypeSingBox, nil
	}
	return coreType, nil
}
