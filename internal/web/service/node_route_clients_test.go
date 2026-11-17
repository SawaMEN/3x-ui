package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestManagedNodeRouteClientsAreSeparateFromPublicLists(t *testing.T) {
	svc, inbounds, settings := setupPagingServices(t)
	db := database.GetDB()
	ordinary := model.ClientRecord{Email: "alice@example.com", SubID: "alice-sub", Enable: true}
	managed := model.ClientRecord{Email: nodeRouteClientEmail(42), SubID: "route-sub", UUID: "preserved-route-uuid", Enable: true, Group: "_system"}
	for _, row := range []*model.ClientRecord{&ordinary, &managed} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.ClientGroup{Name: "_system"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{ordinary.Email, managed.Email} {
		if err := db.Create(&coremodel.ClientTraffic{Email: email, Enable: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	rows, err := svc.List()
	if err != nil || len(rows) != 1 || rows[0].Email != ordinary.Email {
		t.Fatalf("ordinary list includes service identity: %v %v", rows, err)
	}
	page, err := svc.ListPaged(inbounds, settings, ClientPageParams{PageSize: 50})
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("paging counts include service identity: %+v %v", page, err)
	}
	found, count, err := newClientQuery(db, time.Now().UnixMilli(), 0, 0).onlineEmails([]string{ordinary.Email, managed.Email})
	if err != nil || count != 1 || len(found) != 1 || found[0] != ordinary.Email {
		t.Fatalf("online count includes route transport: %v %d %v", found, count, err)
	}
	exported, err := svc.ExportAll()
	if err != nil || len(exported) != 1 || exported[0].Client.Email != ordinary.Email {
		t.Fatalf("client export contains route credentials: %v %v", exported, err)
	}
	groups, err := svc.ListGroups()
	if err != nil || len(groups) != 0 {
		t.Fatalf("system-only group leaked: %v %v", groups, err)
	}
	stored, err := svc.GetRecordByEmail(nil, managed.Email)
	if err != nil || stored.UUID != managed.UUID {
		t.Fatalf("hiding clients changed live routing credentials: %v %v", stored, err)
	}
}

func TestManagedNodeRouteClientRejectsOrdinaryMutations(t *testing.T) {
	setupBulkDB(t)
	svc, inboundSvc := &ClientService{}, &InboundService{}
	row := &model.ClientRecord{Email: nodeRouteClientEmail(42), SubID: "route-sub", Enable: true}
	if err := database.GetDB().Create(row).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{Client: model.Client{Email: row.Email, Enable: true}, InboundIds: []int{42}}); !errors.Is(err, ErrManagedNodeRouteClient) {
		t.Fatalf("public create accepted service identity: %v", err)
	}
	if _, err := svc.Attach(inboundSvc, row.Id, []int{99}); !errors.Is(err, ErrManagedNodeRouteClient) {
		t.Fatalf("ordinary attach accepted service identity: %v", err)
	}
	if _, err := svc.Update(inboundSvc, row.Id, model.Client{Email: "renamed"}, 0); !errors.Is(err, ErrManagedNodeRouteClient) {
		t.Fatalf("ordinary update accepted service identity: %v", err)
	}
	if _, _, err := svc.BulkAttach(inboundSvc, []string{row.Email}, []int{99}); !errors.Is(err, ErrManagedNodeRouteClient) {
		t.Fatalf("bulk attach accepted service identity: %v", err)
	}
	if _, _, err := svc.BulkCreate(inboundSvc, []ClientCreatePayload{{Client: model.Client{Email: row.Email}}}); !errors.Is(err, ErrManagedNodeRouteClient) {
		t.Fatalf("bulk create accepted service identity: %v", err)
	}
	if _, err := svc.AddToGroup([]string{row.Email}, "ordinary"); !errors.Is(err, ErrManagedNodeRouteClient) {
		t.Fatalf("group mutation accepted service identity: %v", err)
	}
	happ := NewHappService(svc, &SettingService{})
	if _, _, reason := happ.currentSource(row.Id, "panel.example.com"); reason != "client_unavailable" {
		t.Fatalf("managed credential exposed as Happ source: %s", reason)
	}
}

func TestManagedNodeRouteDeletionProtectsSavedReferences(t *testing.T) {
	setupBulkDB(t)
	db := database.GetDB()
	tag := nodeRouteTag(42)
	for _, raw := range []string{
		"{\"route\":{\"final\":\"node-route-42\"}}",
		"{\"route\":{\"rules\":[{\"enabled\":false,\"outbound\":\"node-route-42\"}]}}",
		"{\"route\":{\"rules\":[{\"rules\":[{\"outbound\":\"node-route-42\"}]}]}}",
	} {
		if err := (&SettingService{}).SetSingBoxConfigTemplate(raw); err != nil {
			t.Fatal(err)
		}
		used, err := nodeRouteReferenced(db, tag)
		if err != nil || !used {
			t.Fatalf("saved reference not protected: %s %v", raw, err)
		}
	}
	if err := (&SettingService{}).SetSingBoxConfigTemplate("{}"); err != nil {
		t.Fatal(err)
	}
	used, err := nodeRouteReferenced(db, tag)
	if err != nil || used {
		t.Fatalf("unused connection cannot be deleted: %v %v", used, err)
	}
}

func TestDeleteManagedNodeRoutePreservesNodeInboundAndOrdinaryClients(t *testing.T) {
	setupBulkDB(t)
	nodeID, _ := setupNodeRuntime(t)
	ib := nodeInbound(t, nodeID, 32040, nil)
	clients := []model.Client{
		{ID: "11111111-1111-4111-8111-111111111111", Email: nodeRouteClientEmail(ib.Id), Enable: true},
		{ID: "22222222-2222-4222-8222-222222222222", Email: "ordinary@example.com", Enable: true},
	}
	db := database.GetDB()
	ib.Settings = clientsSettings(t, clients)
	if err := db.Save(ib).Error; err != nil {
		t.Fatal(err)
	}
	if err := (&ClientService{}).SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatal(err)
	}
	if _, _, err := DeleteNodeRoute(ib.UserId+1, ib.Id); err == nil {
		t.Fatal("another user deleted connection")
	}
	if err := saveSettingIn(db, "panelOutbound", nodeRouteTag(ib.Id)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := DeleteNodeRoute(ib.UserId, ib.Id); err == nil {
		t.Fatal("referenced connection was deleted")
	}
	if err := saveSettingIn(db, "panelOutbound", "direct"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Node{}).Where("id = ?", nodeID).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	pending, _, err := DeleteNodeRoute(ib.UserId, ib.Id)
	if err != nil || !pending {
		t.Fatalf("offline removal pending=%v err=%v", pending, err)
	}
	var stored model.Inbound
	if err := db.First(&stored, ib.Id).Error; err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Clients []model.Client `json:"clients"`
	}
	if err := json.Unmarshal([]byte(stored.Settings), &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Clients) != 1 || settings.Clients[0].Email != clients[1].Email {
		t.Fatalf("ordinary client changed: %+v", settings.Clients)
	}
	var node model.Node
	if err := db.First(&node, nodeID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := (&ClientService{}).GetRecordByEmail(nil, clients[0].Email); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("routing credential still exists: %v", err)
	}
	if _, _, err := DeleteNodeRoute(ib.UserId, ib.Id); err != nil {
		t.Fatalf("repeated delete: %v", err)
	}
}
