package sub

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestGetSubs_MixedNormalizedProtocols(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	const subID = "sub-mixed"
	db := database.GetDB()

	vless := &model.Inbound{
		UserId: 1, Tag: "multi-vless", Enable: true, Port: 42101, Protocol: model.VLESS,
		Settings: `{"clients":[{"id":"11111111-2222-4333-8444-555555555555","email":"vless@example.com","subId":"sub-mixed","enable":true}]}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	trojan := &model.Inbound{
		UserId: 1, Tag: "multi-trojan", Enable: true, Port: 42102, Protocol: model.Trojan,
		Settings: `{"clients":[{"id":"66666666-7777-4888-8999-000000000000","email":"trojan@example.com","password":"secret","subId":"sub-mixed","enable":true}]}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	if err := db.Create(vless).Error; err != nil {
		t.Fatalf("seed vless: %v", err)
	}
	if err := db.Create(trojan).Error; err != nil {
		t.Fatalf("seed trojan: %v", err)
	}

	vlessClient := &model.ClientRecord{Email: "vless@example.com", SubID: subID, UUID: "11111111-2222-4333-8444-555555555555", Enable: true}
	trojanClient := &model.ClientRecord{Email: "trojan@example.com", SubID: subID, UUID: "66666666-7777-4888-8999-000000000000", Password: "secret", Enable: true}
	if err := db.Create(vlessClient).Error; err != nil {
		t.Fatalf("seed vless client: %v", err)
	}
	if err := db.Create(trojanClient).Error; err != nil {
		t.Fatalf("seed trojan client: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: vlessClient.Id, InboundId: vless.Id}).Error; err != nil {
		t.Fatalf("attach vless: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: trojanClient.Id, InboundId: trojan.Id}).Error; err != nil {
		t.Fatalf("attach trojan: %v", err)
	}

	links, emails, _, _, err := NewSubService("").GetSubs(subID, "sub.example.com")
	if err != nil {
		t.Fatalf("GetSubs: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("links = %d, want 2: %v", len(links), links)
	}
	joined := strings.Join(links, "\n")
	if !strings.Contains(joined, "vless://") || !strings.Contains(joined, "trojan://") {
		t.Fatalf("subscription must contain VLESS and Trojan: %v", links)
	}
	if len(emails) != 2 || emails[0] != "vless@example.com" || emails[1] != "trojan@example.com" {
		t.Fatalf("emails = %v, want both normalized clients in inbound order", emails)
	}
}

func TestGetSubs_MultipleConnectionsSameProtocol(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	const subID = "sub-vless-multi"
	const uuid = "22222222-3333-4444-8555-666666666666"
	db := database.GetDB()
	client := &model.ClientRecord{Email: "multi-vless@example.com", SubID: subID, UUID: uuid, Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}

	for _, seed := range []struct {
		tag  string
		port int
	}{
		{tag: "vless-one", port: 42111},
		{tag: "vless-two", port: 42112},
	} {
		ib := &model.Inbound{
			UserId: 1, Tag: seed.tag, Enable: true, Port: seed.port, Protocol: model.VLESS,
			Settings: fmt.Sprintf(`{"clients":[{"id":%q,"email":%q,"subId":%q,"enable":true}]}`, uuid, client.Email, subID),
			StreamSettings: `{"network":"tcp","security":"none"}`,
		}
		if err := db.Create(ib).Error; err != nil {
			t.Fatalf("seed inbound %s: %v", seed.tag, err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
			t.Fatalf("attach %s: %v", seed.tag, err)
		}
	}

	links, emails, _, _, err := NewSubService("").GetSubs(subID, "sub.example.com")
	if err != nil {
		t.Fatalf("GetSubs: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("links = %d, want 2: %v", len(links), links)
	}
	for _, link := range links {
		if !strings.HasPrefix(link, "vless://") {
			t.Fatalf("unexpected subscription link: %s", link)
		}
	}
	if len(emails) != 2 || emails[0] != client.Email || emails[1] != client.Email {
		t.Fatalf("emails = %v, want the same client once per attached inbound", emails)
	}
	if links[0] == links[1] {
		t.Fatalf("two attached VLESS connections collapsed to the same link: %v", links)
	}
}

func TestGetSubs_DoesNotIncludeUnattachedSettingsOnlyInbound(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	const subID = "sub-no-legacy-leak"
	const uuid = "33333333-4444-4555-8666-777777777777"
	db := database.GetDB()

	working := &model.Inbound{
		UserId: 1, Tag: "working-vless", Enable: true, Port: 42121, Protocol: model.VLESS,
		Settings: fmt.Sprintf(`{"clients":[{"id":%q,"email":"user@example.com","subId":%q,"enable":true}]}`, uuid, subID),
		StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	stale := &model.Inbound{
		UserId: 1, Tag: "stale-trojan", Enable: true, Port: 42122, Protocol: model.Trojan,
		Settings: fmt.Sprintf(`{"clients":[{"id":%q,"email":"user@example.com","enable":true,"password":"stale-secret"}]}`, uuid),
		StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	if err := db.Create(working).Error; err != nil {
		t.Fatalf("seed working inbound: %v", err)
	}
	if err := db.Create(stale).Error; err != nil {
		t.Fatalf("seed stale inbound: %v", err)
	}

	client := &model.ClientRecord{Email: "user@example.com", SubID: subID, UUID: uuid, Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: working.Id}).Error; err != nil {
		t.Fatalf("attach working inbound: %v", err)
	}

	links, _, _, _, err := NewSubService("").GetSubs(subID, "sub.example.com")
	if err != nil {
		t.Fatalf("GetSubs: %v", err)
	}
	if len(links) != 1 || !strings.HasPrefix(links[0], "vless://") {
		t.Fatalf("links = %v, want only the attached VLESS connection", links)
	}
	if strings.Contains(strings.Join(links, "\n"), "trojan://") {
		t.Fatal("subscription included an unattached legacy Trojan connection")
	}

	var attachments int64
	if err := db.Model(&model.ClientInbound{}).
		Where("client_id = ? AND inbound_id = ?", client.Id, stale.Id).
		Count(&attachments).Error; err != nil {
		t.Fatalf("count stale attachment: %v", err)
	}
	if attachments != 0 {
		t.Fatalf("subscription fetch unexpectedly created a client_inbound row: %d", attachments)
	}
}
