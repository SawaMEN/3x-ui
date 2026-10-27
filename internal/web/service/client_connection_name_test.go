package service

import (
	"encoding/json"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestClientConnectionNamePersistence(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	ib := mkInbound(t, 30121, model.VLESS, `{"decryption":"none","clients":[]}`)
	name := "Единое имя"
	client := model.Client{Email: "named", ID: "11111111-1111-4111-8111-111111111111", Enable: true, ConnectionName: &name}
	if err := svc.SyncInbound(nil, ib.Id, []model.Client{client}); err != nil {
		t.Fatal(err)
	}
	client.ConnectionName = nil
	if err := svc.SyncInbound(nil, ib.Id, []model.Client{client}); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.ListForInbound(nil, ib.Id)
	if err != nil || len(rows) != 1 {
		t.Fatalf("reload: %v %+v", err, rows)
	}
	if rows[0].ConnectionName == nil || *rows[0].ConnectionName != name {
		t.Fatal("old request erased name")
	}
	exported, err := svc.ExportAll()
	if err != nil || len(exported) != 1 || exported[0].Client.ConnectionDisplayName() != name {
		t.Fatalf("export: %v %+v", err, exported)
	}
	slim := clientSlimFromRecordForNameTest(t, rows[0])
	if slim.ConnectionName == nil || *slim.ConnectionName != name {
		t.Fatal("paging dropped name")
	}
	empty := ""
	client.ConnectionName = &empty
	if err := svc.SyncInbound(nil, ib.Id, []model.Client{client}); err != nil {
		t.Fatal(err)
	}
	rows, err = svc.ListForInbound(nil, ib.Id)
	if err != nil || rows[0].ConnectionName == nil || *rows[0].ConnectionName != "" {
		t.Fatal("clear failed", err)
	}
}

func clientSlimFromRecordForNameTest(t *testing.T, row model.Client) ClientSlim {
	t.Helper()
	page, err := (&ClientService{}).ListPaged(&InboundService{}, &SettingService{}, ClientPageParams{PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.Email == row.Email {
			return item
		}
	}
	t.Fatal("missing client")
	return ClientSlim{}
}

func TestClientConnectionNameOrphanUpdateAndImport(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	name := "original"
	row := model.ClientRecord{Email: "orphan-name", Enable: true, ConnectionName: &name}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	next := "Renamed VPN"
	update := *row.ToClient()
	update.ConnectionName = &next
	if _, err := svc.Update(&InboundService{}, row.Id, update, 0); err != nil {
		t.Fatal(err)
	}
	var saved model.ClientRecord
	if err := database.GetDB().First(&saved, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if saved.ConnectionName == nil || *saved.ConnectionName != next {
		t.Fatalf("orphan update lost name: %+v", saved)
	}
	exported, err := svc.ExportAll()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	var imported []ClientCreatePayload
	if err = json.Unmarshal(data, &imported); err != nil {
		t.Fatal(err)
	}
	imported[0].Client.Email = "imported-name"
	imported[0].Client.SubID = "imported-name-sub"
	result, _, err := svc.ImportClients(&InboundService{}, imported)
	if err != nil || result.Created != 1 {
		t.Fatalf("import: %v %+v", err, result)
	}
	var restored model.ClientRecord
	if err = database.GetDB().Where("email = ?", "imported-name").First(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if restored.ConnectionName == nil || *restored.ConnectionName != next {
		t.Fatal("import lost name")
	}
}
