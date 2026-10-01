package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const proxyPresetMaxCount = 100

var (
	proxyPresetSchemaOnce sync.Once
	proxyPresetSchemaErr  error
)

type ProxyPresetInput struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Config      model.ProxyPresetConfig `json:"config"`
}

type ProxyPresetView struct {
	Id          int                     `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Config      model.ProxyPresetConfig `json:"config"`
	CreatedAt   int64                   `json:"createdAt"`
	UpdatedAt   int64                   `json:"updatedAt"`
}

type ProxyPresetAssignment struct {
	GroupId    string `json:"groupId"`
	PresetId   int    `json:"presetId"`
	PresetName string `json:"presetName"`
}

type ProxyPresetService struct{}

func ensureProxyPresetSchema() error {
	proxyPresetSchemaOnce.Do(func() {
		proxyPresetSchemaErr = database.GetDB().AutoMigrate(&model.ProxyPreset{}, &model.HostProxyPreset{})
		if proxyPresetSchemaErr == nil {
			model.InvalidateProxyPresetCache()
		}
	})
	return proxyPresetSchemaErr
}

func validateJSONPresetObject(name string, value *string) error {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		*value = ""
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil || obj == nil {
		return fmt.Errorf("%s must be a JSON object", name)
	}
	clean, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("%s could not be normalized: %w", name, err)
	}
	*value = string(clean)
	return nil
}

func validateProxyPresetInput(in *ProxyPresetInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 120 {
		return errors.New("name must be between 1 and 120 characters")
	}
	in.Description = strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(in.Description) > 1000 {
		return errors.New("description must not exceed 1000 characters")
	}
	cfg := &in.Config
	if cfg.Port != nil && (*cfg.Port < 0 || *cfg.Port > 65535) {
		return errors.New("port must be between 0 and 65535")
	}
	if cfg.ServerDescription != nil && utf8.RuneCountInString(*cfg.ServerDescription) > 64 {
		return errors.New("serverDescription must not exceed 64 characters")
	}
	if cfg.Security != nil {
		*cfg.Security = strings.TrimSpace(*cfg.Security)
		switch *cfg.Security {
		case "", "same", "tls", "none", "reality":
		default:
			return errors.New("security must be one of same, tls, none, reality")
		}
	}
	if cfg.MihomoIpVersion != nil {
		*cfg.MihomoIpVersion = strings.TrimSpace(*cfg.MihomoIpVersion)
		switch *cfg.MihomoIpVersion {
		case "", "dual", "ipv4", "ipv6", "ipv4-prefer", "ipv6-prefer":
		default:
			return errors.New("mihomoIpVersion has an unsupported value")
		}
	}
	if cfg.VlessRoute != nil {
		*cfg.VlessRoute = strings.TrimSpace(*cfg.VlessRoute)
		if *cfg.VlessRoute != "" {
			port, err := strconv.Atoi(*cfg.VlessRoute)
			if err != nil || port < 0 || port > 65535 {
				return errors.New("vlessRoute must be an integer between 0 and 65535")
			}
		}
	}
	if err := validateJSONPresetObject("muxParams", cfg.MuxParams); err != nil {
		return err
	}
	if err := validateJSONPresetObject("sockoptParams", cfg.SockoptParams); err != nil {
		return err
	}
	if err := validateJSONPresetObject("finalMask", cfg.FinalMask); err != nil {
		return err
	}
	return nil
}

func decodeProxyPreset(row *model.ProxyPreset) (*ProxyPresetView, error) {
	var cfg model.ProxyPresetConfig
	if err := json.Unmarshal([]byte(row.Config), &cfg); err != nil {
		return nil, err
	}
	return &ProxyPresetView{
		Id: row.Id, Name: row.Name, Description: row.Description, Config: cfg,
		CreatedAt: row.CreatedAt.Unix(), UpdatedAt: row.UpdatedAt.Unix(),
	}, nil
}

func encodeProxyPresetConfig(cfg model.ProxyPresetConfig) (string, error) {
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (s *ProxyPresetService) List(userID int) ([]ProxyPresetView, error) {
	if err := ensureProxyPresetSchema(); err != nil {
		return nil, err
	}
	var rows []model.ProxyPreset
	if err := database.GetDB().Where("user_id = ?", userID).Order("name ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ProxyPresetView, 0, len(rows))
	for i := range rows {
		view, err := decodeProxyPreset(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *view)
	}
	return out, nil
}

func (s *ProxyPresetService) Get(userID, id int) (*ProxyPresetView, error) {
	if err := ensureProxyPresetSchema(); err != nil {
		return nil, err
	}
	var row model.ProxyPreset
	if err := database.GetDB().Where("user_id = ? AND id = ?", userID, id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("proxy preset not found")
		}
		return nil, err
	}
	return decodeProxyPreset(&row)
}

func (s *ProxyPresetService) Save(userID int, in ProxyPresetInput) (*ProxyPresetView, error) {
	if err := ensureProxyPresetSchema(); err != nil {
		return nil, err
	}
	if err := validateProxyPresetInput(&in); err != nil {
		return nil, err
	}
	db := database.GetDB()
	var count int64
	if err := db.Model(&model.ProxyPreset{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= proxyPresetMaxCount {
		return nil, errors.New("proxy preset limit reached")
	}
	config, err := encodeProxyPresetConfig(in.Config)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	row := &model.ProxyPreset{UserId: userID, Name: in.Name, Description: in.Description, Config: config, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(row).Error; err != nil {
		return nil, err
	}
	model.InvalidateProxyPresetCache()
	return decodeProxyPreset(row)
}

func (s *ProxyPresetService) Update(userID, id int, in ProxyPresetInput) (*ProxyPresetView, error) {
	if err := ensureProxyPresetSchema(); err != nil {
		return nil, err
	}
	if err := validateProxyPresetInput(&in); err != nil {
		return nil, err
	}
	config, err := encodeProxyPresetConfig(in.Config)
	if err != nil {
		return nil, err
	}
	db := database.GetDB()
	var row model.ProxyPreset
	if err := db.Where("user_id = ? AND id = ?", userID, id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("proxy preset not found")
		}
		return nil, err
	}
	row.Name = in.Name
	row.Description = in.Description
	row.Config = config
	row.UpdatedAt = time.Now()
	if err := db.Save(&row).Error; err != nil {
		return nil, err
	}
	model.InvalidateProxyPresetCache()
	return decodeProxyPreset(&row)
}

func (s *ProxyPresetService) Delete(userID, id int) error {
	if err := ensureProxyPresetSchema(); err != nil {
		return err
	}
	db := database.GetDB()
	err := db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.ProxyPreset{}).Where("user_id = ? AND id = ?", userID, id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errors.New("proxy preset not found")
		}
		if err := tx.Where("user_id = ? AND preset_id = ?", userID, id).Delete(&model.HostProxyPreset{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ? AND id = ?", userID, id).Delete(&model.ProxyPreset{}).Error
	})
	if err == nil {
		model.InvalidateProxyPresetCache()
	}
	return err
}

func hostGroupBelongsToUser(tx *gorm.DB, userID int, groupID string) (bool, error) {
	var count int64
	err := tx.Table("hosts AS h").Joins("JOIN inbounds AS i ON i.id = h.inbound_id").
		Where("h.group_id = ? AND i.user_id = ?", groupID, userID).Count(&count).Error
	return count > 0, err
}

func (s *ProxyPresetService) Assign(userID int, groupID string, presetID int) (*ProxyPresetAssignment, error) {
	if err := ensureProxyPresetSchema(); err != nil {
		return nil, err
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, errors.New("groupId is required")
	}
	db := database.GetDB()
	var preset model.ProxyPreset
	if err := db.Where("user_id = ? AND id = ?", userID, presetID).First(&preset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("proxy preset not found")
		}
		return nil, err
	}
	belongs, err := hostGroupBelongsToUser(db, userID, groupID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errors.New("host group not found")
	}
	now := time.Now()
	binding := model.HostProxyPreset{GroupId: groupID, UserId: userID, PresetId: presetID, CreatedAt: now, UpdatedAt: now}
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "group_id"}},
		DoUpdates: clause.Assignments(map[string]any{"user_id": userID, "preset_id": presetID, "updated_at": now}),
	}).Create(&binding).Error; err != nil {
		return nil, err
	}
	model.InvalidateProxyPresetCache()
	return &ProxyPresetAssignment{GroupId: groupID, PresetId: presetID, PresetName: preset.Name}, nil
}

func (s *ProxyPresetService) Assignment(userID int, groupID string) (*ProxyPresetAssignment, error) {
	if err := ensureProxyPresetSchema(); err != nil {
		return nil, err
	}
	groupID = strings.TrimSpace(groupID)
	var binding model.HostProxyPreset
	if err := database.GetDB().Where("user_id = ? AND group_id = ?", userID, groupID).First(&binding).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var preset model.ProxyPreset
	if err := database.GetDB().Where("user_id = ? AND id = ?", userID, binding.PresetId).First(&preset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ProxyPresetAssignment{GroupId: groupID, PresetId: preset.Id, PresetName: preset.Name}, nil
}

func (s *ProxyPresetService) Unassign(userID int, groupID string) error {
	if err := ensureProxyPresetSchema(); err != nil {
		return err
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return errors.New("groupId is required")
	}
	if err := database.GetDB().Where("user_id = ? AND group_id = ?", userID, groupID).Delete(&model.HostProxyPreset{}).Error; err != nil {
		return err
	}
	model.InvalidateProxyPresetCache()
	return nil
}
