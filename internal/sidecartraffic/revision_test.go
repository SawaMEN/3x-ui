package sidecartraffic

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestDesiredRevisionRejectsStaleInputsAndEligibility(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	db := database.GetDB()
	ib := model.Inbound{Id: 10, Tag: "revision", Enable: true, Protocol: model.Mieru, Port: 34585, Settings: `{"clients":[{"email":"first","enable":true,"password":"old"}]}`}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	request := Desired{ID: ib.Id, Protocol: ib.Protocol, Revision: RevisionOf(&ib), Clients: []string{"first"}}
	if err := ValidateDesired(request); err != nil {
		t.Fatal(err)
	}
	unversioned := request
	unversioned.Revision = ""
	if err := ValidateDesired(unversioned); !errors.Is(err, ErrStaleDesired) {
		t.Fatalf("unversioned: %v", err)
	}
	if err := db.Model(&ib).Updates(map[string]any{"up": 123, "down": 456, "remark": "panel-only"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ValidateDesired(request); err != nil {
		t.Fatalf("counters/presentation invalidated snapshot: %v", err)
	}
	ib.Settings = `{"clients":[{"email":"first","enable":true,"password":"new"}]}`
	if err := db.Save(&ib).Error; err != nil {
		t.Fatal(err)
	}
	if err := ValidateDesired(request); !errors.Is(err, ErrStaleDesired) {
		t.Fatalf("old credentials accepted: %v", err)
	}
	request.Revision = RevisionOf(&ib)
	if err := ValidateDesired(request); err != nil {
		t.Fatal(err)
	}
	row := coremodel.ClientTraffic{InboundId: ib.Id, Email: "first", Enable: false}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := ValidateDesired(request); !errors.Is(err, ErrStaleDesired) {
		t.Fatalf("quota block not detected: %v", err)
	}
	request.Clients = nil
	request.ClientRevision = DisabledRevision(map[string]bool{"first": true})
	if err := ValidateDesired(request); err != nil {
		t.Fatalf("current filtered payload rejected: %v", err)
	}
	if err := db.Model(&row).Update("enable", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := ValidateDesired(request); !errors.Is(err, ErrStaleDesired) {
		t.Fatalf("old blocked set accepted after reset: %v", err)
	}
	request.ClientRevision = ""
	if err := ValidateDesired(request); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeProvenanceIsLocalAndDoesNotPersist(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	db := database.GetDB()
	ib := model.Inbound{Tag: "provenance", Enable: true, Protocol: model.Mieru, Settings: `{"clients":[]}`}
	original := RevisionOf(&ib)
	ib.RuntimeSourceRevision = original
	ib.RuntimeClientRevision = "local-eligibility"
	ib.Settings = `{"clients":[{"email":"filtered"}]}`
	if RevisionOf(&ib) != original {
		t.Fatal("local filtering changed source revision")
	}
	data, err := json.Marshal(ib)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), original) || strings.Contains(string(data), "local-eligibility") {
		t.Fatal("runtime provenance escaped into API JSON")
	}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	var stored model.Inbound
	if err := db.First(&stored, ib.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.RuntimeSourceRevision != "" || stored.RuntimeClientRevision != "" {
		t.Fatal("runtime provenance persisted")
	}
	if RevisionOf(&stored) == original {
		t.Fatal("database row incorrectly inherited old source revision")
	}
	a := DisabledRevision(map[string]bool{"a": true, "b": true})
	b := DisabledRevision(map[string]struct{}{"b": {}, "a": {}})
	if a != b || a == "" || DisabledRevision(map[string]bool{}) != "" {
		t.Fatal("eligibility revision is not deterministic")
	}
}
