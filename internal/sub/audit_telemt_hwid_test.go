package sub

import (
	"net/http"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestAuditTelegramSubscriptionCannotBypassHwidGate(t *testing.T) {
	router, subID := initHwidSubRouter(t, 1)
	router.GET("/sub/telemt/:subid", serveTelemtSubscription)
	db := database.GetDB()
	if err := db.Model(&model.ClientRecord{}).Where("sub_id = ?", subID).UpdateColumn("secret", mtprotoTestSecret).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Inbound{}).Where("tag = ?", "hwid-sub").Updates(map[string]any{
		"protocol": model.MTProto, "stream_settings": "{}", "settings": "{}",
	}).Error; err != nil {
		t.Fatal(err)
	}
	paths := []string{"/sub/" + subID + "?format=telemt", "/sub/telemt/" + subID}
	for _, path := range paths {
		denied := requestSub(t, router, http.MethodGet, path, "", "")
		if denied.Code != http.StatusNotFound || denied.Header().Get("X-Hwid-Not-Supported") != "true" {
			t.Errorf("Telegram endpoint bypassed HWID: status=%d path=%s", denied.Code, path)
		}
		if strings.Contains(denied.Body.String(), mtprotoTestSecret) {
			t.Error("ungated Telegram response exposed proxy secret")
		}
	}
	for _, path := range paths {
		allowed := requestSub(t, router, http.MethodGet, path, "device-one", "")
		if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), mtprotoTestSecret) {
			t.Fatalf("registered device lost Telegram profile: status=%d", allowed.Code)
		}
		denied := requestSub(t, router, http.MethodGet, path, "device-two", "")
		if denied.Code != http.StatusNotFound || denied.Header().Get("X-Hwid-Max-Devices-Reached") != "true" {
			t.Error("Telegram endpoint ignored full device slots")
		}
	}
}
