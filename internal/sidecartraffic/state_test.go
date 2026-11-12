package sidecartraffic

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestCurrentBatchesAndMatchesExactProtocol(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	db := database.GetDB()
	rows := make([]model.Inbound, 1802)
	desired := make([]Desired, len(rows))
	for i := range rows {
		rows[i] = model.Inbound{Id: i + 1, Enable: true, Tag: fmt.Sprintf("state-%d", i), Protocol: model.OpenFlux}
		desired[i] = Desired{ID: i + 1, Protocol: model.OpenFlux}
	}
	node := 1
	rows[0].Enable = false
	rows[900].NodeID = &node
	rows[1800].Protocol = model.TrustTunnel
	if err := db.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	desired = append(desired, Desired{ID: 9999, Protocol: model.OpenFlux}, Desired{ID: 1802, Protocol: model.TrustTunnel})
	allowed, err := Current(desired)
	if err != nil {
		t.Fatal(err)
	}
	for i, ok := range allowed {
		want := i != 0 && i != 900 && i != 1800 && i != 1802 && i != 1803
		if ok != want {
			t.Errorf("decision %d=%t, want %t", i, ok, want)
		}
	}
	if err := ValidateDesired(1, model.OpenFlux); !errors.Is(err, ErrRevoked) {
		t.Fatalf("disabled inbound: %v", err)
	}
	if err := ValidateDesired(1802, model.OpenFlux); err != nil {
		t.Fatalf("valid inbound: %v", err)
	}
}

func TestCurrentReadFailureIsNotRevocation(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	db := database.GetDB()
	if err := db.Callback().Query().Before("gorm:query").Register("audit:state-failure", func(tx *gorm.DB) { tx.AddError(errors.New("secret fixture failure")) }); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove("audit:state-failure")
	if allowed, err := Current([]Desired{{ID: 1, Protocol: model.Mieru}}); allowed != nil || err == nil || errors.Is(err, ErrRevoked) || err.Error() != "read current sidecar inbound state failed" {
		t.Fatalf("failure misclassified: %v %v", allowed, err)
	}
	if allowed, err := Current(nil); err != nil || len(allowed) != 0 {
		t.Fatalf("empty desired should not need DB: %v %v", allowed, err)
	}
}
