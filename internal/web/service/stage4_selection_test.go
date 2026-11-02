package service

import (
	"errors"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
	"testing"
)

func TestStage4AtomicCoreSelection(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new-marker", true: "existing-marker"}[existing], func(t *testing.T) {
			setupBulkDB(t)
			s := &SettingService{}
			db := database.GetDB()
			if err := s.setString("coreType", CoreTypeSingBox); err != nil {
				t.Fatal(err)
			}
			if err := db.Where("key = ?", "standaloneCoreSelection").Delete(&model.Setting{}).Error; err != nil {
				t.Fatal(err)
			}
			if existing {
				if err := s.setString("standaloneCoreSelection", "previous"); err != nil {
					t.Fatal(err)
				}
			}
			injected := errors.New("injected core selection persistence failure")
			name := "test:stage4_core_selection"
			if err := db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
				if row, ok := tx.Statement.Dest.(*model.Setting); ok && row.Key == "coreType" {
					tx.AddError(injected)
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Update().Remove(name) })
			if err := s.SetCoreType(CoreTypeXray); !errors.Is(err, injected) {
				t.Fatalf("write failure not propagated: %v", err)
			}
			if got, err := s.GetCoreType(); err != nil || got != CoreTypeSingBox {
				t.Fatal("failed selection changed active type", got, err)
			}
			marker, err := s.getSetting("standaloneCoreSelection")
			if existing {
				if err != nil || marker.Value != "previous" {
					t.Fatal("failed selection changed compatibility marker")
				}
			} else if !database.IsNotFound(err) {
				t.Fatal("failed selection left a compatibility marker", err)
			}
		})
	}
}
