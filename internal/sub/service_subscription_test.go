package sub

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestGetSubs_MixedNormalizedAndSettingsOnlyInbounds(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	const subID = "sub-multi"
	const email = "multi@example.com"
	const uuid = "11111111-2222-4333-8444-555555555555"

	vlessSettings := fmt.Sprintf(`{"clients":[{"id":%q,"email":%q,"subId":%q,"enable":true}]}`, uuid, email, subID)
	trojanSettings := fmt.Sprintf(`{"clients":[{"id":%q,"email":%q,"password":"secret","subId":%q,"enable":true}]}`, uuid, email, subID)

	vless := &model.Inbound{
		UserId: 1, Tag: "multi-vless", Enable: true, Port: 42101, Protocol: model.VLESS,
		Settings: vlessSettings, StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	trojan := &model.Inbound{
		UserId: 1, Tag: "multi-trojan", Enable: true, Port: 42102, Protocol: model.Trojan,
		Settings: trojanSettings, StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	db := database.GetDB()
	if err := db.Create(vless).Error; err != nil {
		t.Fatalf("seed vless: %v", err)
	}
	if err := db.Create(trojan).Error; err != nil {
		t.Fatalf("seed trojan: %v", err)
	}

	client := &model.ClientRecord{Email: email, SubID: subID, UUID: uuid, Password: "secret", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: vless.Id}).Error; err != nil {
		t.Fatalf("seed vless client_inbound: %v", err)
	}

	s := NewSubService("")
	links, _, _, _, err := s.GetSubs(subID, "sub.example.com")
	if err != nil {
		t.Fatalf("GetSubs: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("links = %d, want 2: %v", len(links), links)
	}
	joined := strings.Join(links, "\n")
	if !strings.Contains(joined, "vless://") {
		t.Fatalf("subscription missing VLESS link: %v", links)
	}
	if !strings.Contains(joined, "trojan://") {
		t.Fatalf("subscription missing Trojan link: %v", links)
	}
}
