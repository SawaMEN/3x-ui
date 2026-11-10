package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestManagerSNIBackupPreservesIdentityAndCommitsOnlyWorkingCandidate(t *testing.T) {
	original := &model.Inbound{
		Settings:       `{"clients":[{"email":"preserve"}]}`,
		StreamSettings: `{"security":"reality","realitySettings":{"privateKey":"persistent-key","serverNames":["primary.example.org"]}}`,
	}
	entry := model.ManagerEntry{
		SNI: "primary.example.org",
		SNIBackups: []string{"backup.example.org", "another.example.org"},
	}
	attempts := []string{}
	name, err := managerChooseEntrySNI(original, entry,
		func(ib *model.Inbound, e model.ManagerEntry) error {
			attempts = append(attempts, e.SNI)
			ib.StreamSettings = strings.ReplaceAll(ib.StreamSettings, "primary.example.org", e.SNI)
			return nil
		},
		func(ib *model.Inbound) error {
			if strings.Contains(ib.StreamSettings, "backup.example.org") {
				return nil
			}
			return errors.New("TLS validation failed")
		})
	if err != nil || name != "backup.example.org" {
		t.Fatalf("backup not selected: %s, %v", name, err)
	}
	if len(attempts) != 2 || attempts[0] != entry.SNI || attempts[1] != entry.SNIBackups[0] {
		t.Fatalf("unexpected SNI order: %+v", attempts)
	}
	if !strings.Contains(original.StreamSettings, "persistent-key") ||
		!strings.Contains(original.StreamSettings, "backup.example.org") ||
		original.Settings != `{"clients":[{"email":"preserve"}]}` {
		t.Fatal("backup selection changed credentials or damaged the inbound")
	}
}

func TestManagerSNIBackupFailureDoesNotModifyInbound(t *testing.T) {
	ib := &model.Inbound{Settings: "preserve", StreamSettings: "before"}
	entry := model.ManagerEntry{SNI: "first.example.org", SNIBackups: []string{"second.example.org"}}
	_, err := managerChooseEntrySNI(ib, entry,
		func(ib *model.Inbound, e model.ManagerEntry) error {
			ib.Settings = e.SNI
			ib.StreamSettings = e.SNI
			return nil
		},
		func(*model.Inbound) error { return errors.New("offline") })
	if err == nil || ib.Settings != "preserve" || ib.StreamSettings != "before" {
		t.Fatalf("failed alternatives modified the original: %+v, %v", ib, err)
	}
}

func TestManagerValidateSNIBackupOnlyForReality(t *testing.T) {
	svc, ib, sub := managerFixture(t)
	sub.Entries[0].SNIBackups = []string{"backup.example.org"}
	if _, err := svc.Save(1, *sub); err == nil {
		t.Fatal("backup allowed on an unencrypted connection")
	}
	sub.Entries[0].SNIBackups = []string{"backup.example.org", "backup.example.org"}
	ib.StreamSettings = `{"network":"tcp","security":"reality","realitySettings":{"target":"example.org:443","serverNames":["example.org"]}}`
	if err := database.GetDB().Save(ib).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(1, *sub); err == nil {
		t.Fatal("duplicate backups were accepted")
	}
}
