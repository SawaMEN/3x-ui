package sub

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestSplitLinkLinesPrefersCompleteMieruProfile(t *testing.T) {
	native := "mieru://Q29tcGxldGVQcm9maWxl"
	simple := "mierus://user:password@example.com?port=5000&profile=default&protocol=TCP"
	got := splitLinkLines(native + "\n" + simple)
	if len(got) != 1 {
		t.Fatalf("got %d Mieru links, want 1: %v", len(got), got)
	}
	if got[0] != native {
		t.Fatalf("Mieru subscription kept %q, want complete profile %q", got[0], native)
	}

	got = splitLinkLines(simple)
	if len(got) != 1 || got[0] != simple {
		t.Fatalf("standalone simple Mieru link must remain a fallback, got %v", got)
	}
}

func TestBuildRawSubscriptionBodyDeduplicatesAcrossEntries(t *testing.T) {
	const link = "vless://11111111-2222-4333-8444-555555555555@example.com:443?security=none#same"
	body := buildRawSubscriptionBody([]string{link, link, "\n" + link + "\n"})
	if got := strings.Count(body, link); got != 1 {
		t.Fatalf("raw subscription contains duplicate link %d times:\n%s", got, body)
	}
	if body != link+"\n" {
		t.Fatalf("raw subscription body = %q, want one normalized line", body)
	}
}

func TestRepairSubscriptionBindingsRestoresOnlyExactSubscriptionMembership(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	const subID = "sub-repair"
	db := database.GetDB()

	wanted := &model.Inbound{
		UserId:   1,
		Tag:      "repair-wanted",
		Enable:   true,
		Port:     443,
		Protocol: model.VLESS,
		Settings: `{"clients":[{"id":"11111111-2222-4333-8444-555555555555","email":"repair@example.com","subId":"sub-repair","enable":true}]}`,
	}
	wrongSub := &model.Inbound{
		UserId:   1,
		Tag:      "repair-wrong-sub",
		Enable:   true,
		Port:     444,
		Protocol: model.VLESS,
		Settings: `{"clients":[{"id":"22222222-2222-4333-8444-555555555555","email":"repair@example.com","subId":"different-sub","enable":true}]}`,
	}
	unrelatedEmail := &model.Inbound{
		UserId:   1,
		Tag:      "repair-wrong-email",
		Enable:   true,
		Port:     445,
		Protocol: model.VLESS,
		Settings: `{"clients":[{"id":"33333333-2222-4333-8444-555555555555","email":"other@example.com","subId":"sub-repair","enable":true}]}`,
	}
	for _, inbound := range []*model.Inbound{wanted, wrongSub, unrelatedEmail} {
		if err := db.Create(inbound).Error; err != nil {
			t.Fatalf("create inbound %s: %v", inbound.Tag, err)
		}
	}

	client := &model.ClientRecord{
		Email:  "repair@example.com",
		SubID:  subID,
		UUID:   "11111111-2222-4333-8444-555555555555",
		Enable: true,
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}

	svc := &SubService{}
	repaired, err := svc.repairSubscriptionBindings(subID)
	if err != nil {
		t.Fatalf("repairSubscriptionBindings: %v", err)
	}
	if repaired != 1 {
		t.Fatalf("repaired = %d, want 1", repaired)
	}

	var bindings []model.ClientInbound
	if err := db.Where("client_id = ?", client.Id).Find(&bindings).Error; err != nil {
		t.Fatalf("load bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].InboundId != wanted.Id {
		t.Fatalf("bindings = %+v, want only inbound %d", bindings, wanted.Id)
	}

	repaired, err = svc.repairSubscriptionBindings(subID)
	if err != nil {
		t.Fatalf("second repairSubscriptionBindings: %v", err)
	}
	if repaired != 0 {
		t.Fatalf("second repair must be idempotent, repaired = %d", repaired)
	}
}
