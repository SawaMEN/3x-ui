package database

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// Start with the deployed SQL schema rather than an alias of the new Go type.
func TestCoreModelUpgradePreservesTrafficAndClientIdentity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	path := filepath.Join(dir, "x-ui.db")
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE client_traffics (
		id integer PRIMARY KEY AUTOINCREMENT, inbound_id integer, enable numeric,
		email text UNIQUE, up integer, down integer, expiry_time integer, total integer,
		reset integer DEFAULT 0, reset_day integer DEFAULT 0, reset_max integer DEFAULT 0,
		reset_count integer DEFAULT 0, last_online integer DEFAULT 0, last_sub_fetch integer DEFAULT 0);
		CREATE INDEX idx_client_traffics_inbound ON client_traffics(inbound_id);
		CREATE INDEX idx_client_traffics_renew ON client_traffics(expiry_time, reset);
		CREATE INDEX idx_client_traffics_last_online ON client_traffics(last_online);
		INSERT INTO client_traffics VALUES (14825,37,1,'stage3@example.com',1048576,2097152,4102444800000,10737418240,30,17,12,3,1735680000000,1735680000123);`)
	closeErr := legacy.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	want := coremodel.ClientTraffic{Id: 14825, InboundId: 37, Enable: true, Email: "stage3@example.com", Up: 1048576, Down: 2097152, ExpiryTime: 4102444800000, Total: 10737418240, Reset: 30, ResetDay: 17, ResetMax: 12, ResetCount: 3, LastOnline: 1735680000000, LastSubFetch: 1735680000123}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	inbound := model.Inbound{Id: 37, Port: 19437, Protocol: model.VLESS, Tag: "stage3", Settings: `{"clients":[]}`, StreamSettings: `{}`, Sniffing: `{}`}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	client := model.ClientRecord{Id: 91, Email: want.Email, UUID: "ce8d33df-3a64-4f10-8f9b-91c3a8e0c001", SubID: "stage3-sub", Enable: true}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	link := model.ClientInbound{ClientId: 91, InboundId: 37}
	if err := db.Create(&link).Error; err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := CloseDB(); err != nil {
			t.Fatal(err)
		}
		if err := InitDB(path); err != nil {
			t.Fatal(err)
		}
		var got coremodel.ClientTraffic
		if err := db.First(&got, want.Id).Error; err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("traffic changed after upgrade: got %+v, want %+v", got, want)
		}
		var stored model.ClientRecord
		if err := db.First(&stored, client.Id).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Email != client.Email || stored.UUID != client.UUID || stored.SubID != client.SubID {
			t.Fatalf("client identity changed: %+v", stored)
		}
		var parent model.Inbound
		if err := db.Preload("ClientStats").First(&parent, 37).Error; err != nil {
			t.Fatal(err)
		}
		if len(parent.ClientStats) != 1 || !reflect.DeepEqual(parent.ClientStats[0], want) {
			t.Fatalf("traffic association changed: %+v", parent.ClientStats)
		}
		var count int64
		if err := db.Model(&model.ClientInbound{}).Where("client_id = ? AND inbound_id = ?", 91, 37).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("client attachment changed: count=%d, err=%v", count, err)
		}
		for _, index := range []string{"idx_client_traffics_inbound", "idx_client_traffics_renew", "idx_client_traffics_last_online"} {
			if !db.Migrator().HasIndex(&coremodel.ClientTraffic{}, index) {
				t.Fatalf("missing index %s", index)
			}
		}
	}
	duplicate := coremodel.ClientTraffic{Email: want.Email}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("email uniqueness lost")
	}
	next := coremodel.ClientTraffic{Email: "next@example.com"}
	if err := db.Create(&next).Error; err != nil {
		t.Fatal(err)
	}
	if next.Id <= want.Id {
		t.Fatalf("autoincrement sequence lost: %d", next.Id)
	}
	for _, table := range []string{"coremodel_client_traffics", "xray_client_traffics"} {
		if db.Migrator().HasTable(table) {
			t.Fatalf("unexpected replacement table %s", table)
		}
	}
}
