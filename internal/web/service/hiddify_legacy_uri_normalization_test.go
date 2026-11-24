package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestGetHiddifySubscriptionURIsNormalizesStoredListenerPort(t *testing.T) {
	setupConflictDB(t)
	const id = "768e8bdd-bee3-4442-9006-b26464148aaa"
	if err := database.GetDB().Create(&model.ClientRecord{
		Email:         "hiddify_Alice_768e8bdd",
		UUID:          id,
		SubID:         id,
		HiddifySubURI: "https://cdn.example.com:2096/BackupPath123/",
	}).Error; err != nil {
		t.Fatal(err)
	}

	urls, err := (&SettingService{}).GetHiddifySubscriptionURIs()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := urls[id], "https://cdn.example.com/BackupPath123/"; got != want {
		t.Fatalf("Hiddify subscription URL = %q, want %q", got, want)
	}
}

func TestHiddifySubscriptionPathDoesNotAcceptRootOrMalformedBase(t *testing.T) {
	setupConflictDB(t)
	const id = "768e8bdd-bee3-4442-9006-b26464148aaa"
	if err := database.GetDB().Create(&model.ClientRecord{
		Email:         "hiddify_Alice_768e8bdd",
		UUID:          id,
		SubID:         id,
		HiddifySubURI: "https://cdn.example.com/",
	}).Error; err != nil {
		t.Fatal(err)
	}
	s := &SettingService{}
	if s.IsHiddifySubscriptionPath(id, "/unrelated/"+id) {
		t.Fatal("root URI authorized an unrelated path")
	}
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("sub_id = ?", id).
		Update("hiddify_sub_uri", "https://cdn.example.com/BackupPath123/").Error; err != nil {
		t.Fatal(err)
	}
	if !s.IsHiddifySubscriptionPath(id, "/BackupPath123/"+id) ||
		s.IsHiddifySubscriptionPath(id, "/BackupPath123-other/"+id) {
		t.Fatal("legacy path boundary was not respected")
	}
}
