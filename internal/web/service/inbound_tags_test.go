package service

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestGetInboundTagsOnlyReturnsCurrentUsersInbounds(t *testing.T) {
	setupConflictDB(t)

	for _, item := range []struct {
		userID int
		tag    string
		port   int
	}{
		{10, "owner-second", 12002},
		{20, "other-user", 12003},
		{10, "owner-first", 12001},
	} {
		inbound := &model.Inbound{
			UserId:         item.userID,
			Tag:            item.tag,
			Port:           item.port,
			Enable:         true,
			Protocol:       model.VLESS,
			StreamSettings: `{"network":"tcp"}`,
			Settings:       `{"clients":[]}`,
		}
		if err := database.GetDB().Create(inbound).Error; err != nil {
			t.Fatalf("create inbound %q: %v", item.tag, err)
		}
	}

	raw, err := (&InboundService{}).GetInboundTags(10)
	if err != nil {
		t.Fatalf("GetInboundTags: %v", err)
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		t.Fatalf("decode inbound tags: %v", err)
	}
	want := []string{"owner-second", "owner-first"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("inbound tags = %v, want %v", tags, want)
	}
}
