package service

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func auditDelayedClient(t *testing.T, expiry int64) (*InboundService, *model.Inbound) {
	t.Helper()
	setupBulkDB(t)
	svc := &InboundService{}
	clients := []model.Client{{Email: "audit", ID: "11111111-1111-4111-8111-111111111111", Enable: true, ExpiryTime: expiry}}
	ib := mkInbound(t, 30321, model.VLESS, clientsSettings(t, clients))
	if err := svc.clientService.SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&coremodel.ClientTraffic{Email: "audit", InboundId: ib.Id, Enable: true, ExpiryTime: expiry}).Error; err != nil {
		t.Fatal(err)
	}
	return svc, ib
}

func TestAuditZeroTrafficDoesNotStartDelayedSubscription(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "strict"}[strict], func(t *testing.T) {
			svc, _ := auditDelayedClient(t, -86400000)
			data := []*coremodel.ClientTraffic{{Email: "audit"}}
			var err error
			if strict {
				err = svc.addClientTrafficStrict(database.GetDB(), data)
			} else {
				err = svc.addClientTraffic(database.GetDB(), data)
			}
			if err != nil {
				t.Fatal(err)
			}
			var got coremodel.ClientTraffic
			if err = database.GetDB().Where("email = ?", "audit").First(&got).Error; err != nil {
				t.Fatal(err)
			}
			if got.ExpiryTime != -86400000 {
				t.Fatalf("zero traffic activated subscription: expiry=%d", got.ExpiryTime)
			}
		})
	}
}

func TestAuditNegativeFirstDeltaCannotCancelPositiveTraffic(t *testing.T) {
	svc, _ := auditDelayedClient(t, 0)
	deltas := []*coremodel.ClientTraffic{{Email: "audit", Up: -100, Down: math.MinInt64}, {Email: "audit", Up: 20, Down: 30}}
	if err := svc.addClientTrafficStrict(database.GetDB(), deltas); err != nil {
		t.Fatal(err)
	}
	var got coremodel.ClientTraffic
	if err := database.GetDB().Where("email = ?", "audit").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Up != 20 || got.Down != 30 {
		t.Fatalf("positive deltas lost: %+v", got)
	}
	if deltas[0].Up != -100 {
		t.Fatal("caller snapshot mutated")
	}
}

func TestAuditExtremeDelayedExpiryRemainsPositive(t *testing.T) {
	svc, _ := auditDelayedClient(t, math.MinInt64)
	if err := svc.addClientTraffic(database.GetDB(), []*coremodel.ClientTraffic{{Email: "audit", Up: 1}}); err != nil {
		t.Fatal(err)
	}
	var got coremodel.ClientTraffic
	if err := database.GetDB().Where("email = ?", "audit").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.ExpiryTime <= time.Now().UnixMilli() {
		t.Fatalf("delayed deadline wrapped: %d", got.ExpiryTime)
	}
}

func TestAuditNullClientsDoNotPanicDuringActivation(t *testing.T) {
	svc, ib := auditDelayedClient(t, -86400000)
	settings := `{"clients":[null,{"email":"audit","id":"11111111-1111-4111-8111-111111111111","enable":true,"expiryTime":-86400000}]}`
	if err := database.GetDB().Model(ib).Update("settings", settings).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.addClientTraffic(database.GetDB(), []*coremodel.ClientTraffic{{Email: "audit", Up: 1}}); err != nil {
		t.Fatal(err)
	}
}

func TestAuditSaturatedTrafficStillUpdatesOnline(t *testing.T) {
	svc, _ := auditDelayedClient(t, 0)
	db := database.GetDB()
	if err := db.Model(&coremodel.ClientTraffic{}).Where("email = ?", "audit").Updates(map[string]any{"up": database.TrafficMax, "down": database.TrafficMax}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.addClientTrafficStrict(db, []*coremodel.ClientTraffic{{Email: "audit", Up: 1}}); err != nil {
		t.Fatal(err)
	}
	var got coremodel.ClientTraffic
	if err := db.Where("email = ?", "audit").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.LastOnline == 0 {
		t.Fatal("saturated client remained offline despite new traffic")
	}
}

func TestAuditDelayedActivationDatabaseFailureRollsBackTraffic(t *testing.T) {
	svc, _ := auditDelayedClient(t, -86400000)
	db := database.GetDB()
	name := "audit:fail-inbound-save"
	db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "inbounds" {
			tx.AddError(errors.New("injected inbound save failure"))
		}
	})
	defer db.Callback().Create().Remove(name)
	err := svc.CommitCoreTraffic(nil, []*coremodel.ClientTraffic{{Email: "audit", Up: 7}})
	if err == nil {
		t.Fatal("activation failure was acknowledged as success")
	}
	var got coremodel.ClientTraffic
	if err = db.Where("email = ?", "audit").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Up != 0 || got.ExpiryTime != -86400000 {
		t.Fatalf("partial activation persisted: %+v", got)
	}
}

func TestAuditNullRenewalAndSaturatedRenewalCount(t *testing.T) {
	svc, ib := auditDelayedClient(t, 1)
	db := database.GetDB()
	if err := db.Model(&coremodel.ClientTraffic{}).Where("email = ?", "audit").Updates(map[string]any{"reset": 1, "reset_count": math.MaxInt}).Error; err != nil {
		t.Fatal(err)
	}
	settings := `{"clients":[null,{"email":"audit","id":"11111111-1111-4111-8111-111111111111","enable":true,"expiryTime":1,"reset":1}]}`
	if err := db.Model(ib).Update("settings", settings).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.autoRenewClients(db, newTrafficMutationBatch()); err != nil {
		t.Fatal(err)
	}
	var got coremodel.ClientTraffic
	if err := db.Where("email = ?", "audit").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.ResetCount != math.MaxInt || got.ExpiryTime < time.Now().UnixMilli() {
		t.Fatalf("renewal state wrapped: %+v", got)
	}
}

func TestAuditInvalidAddPayloadReturnsError(t *testing.T) {
	setupBulkDB(t)
	ib := mkInbound(t, 30322, model.VLESS, `{"clients":[]}`)
	for _, settings := range []string{`{}`, `{"clients":null}`, `{"clients":{}}`} {
		if _, err := (&ClientService{}).AddInboundClient(&InboundService{}, &model.Inbound{Id: ib.Id, Settings: settings}); err == nil {
			t.Fatalf("accepted %s", settings)
		}
	}
}

func TestAuditNegativeStoredCountersDoNotLoseNewUsage(t *testing.T) {
	svc, _ := auditDelayedClient(t, 0)
	db := database.GetDB()
	if err := db.Model(&coremodel.ClientTraffic{}).Where("email = ?", "audit").Updates(map[string]any{"up": int64(math.MinInt64), "down": int64(-100)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.addClientTraffic(db, []*coremodel.ClientTraffic{{Email: "audit", Up: 10, Down: 20}}); err != nil {
		t.Fatal(err)
	}
	var got coremodel.ClientTraffic
	if err := db.Where("email = ?", "audit").First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Up != 10 || got.Down != 20 {
		t.Fatalf("positive deltas lost while repairing stored counters: %+v", got)
	}
}

func TestAuditTrafficQueriesRespectSQLiteVariableBudget(t *testing.T) {
	setupBulkDB(t)
	db := database.GetDB()
	rows := make([]coremodel.ClientTraffic, 1005)
	deltas := make([]*coremodel.ClientTraffic, len(rows))
	for i := range rows {
		email := fmt.Sprintf("audit-%04d", i)
		rows[i] = coremodel.ClientTraffic{Email: email, Enable: true}
		deltas[i] = &coremodel.ClientTraffic{Email: email, Up: 1}
	}
	if err := db.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	name := "audit:sqlite-variable-budget"
	if err := db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "client_traffics" && len(tx.Statement.Vars) > 999 {
			tx.AddError(errors.New("too many SQL variables"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(name)
	if err := (&InboundService{}).addClientTrafficStrict(db, deltas); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&coremodel.ClientTraffic{}).Where("up = ?", 1).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != int64(len(rows)) {
		t.Fatalf("updated %d of %d clients", count, len(rows))
	}
}
