package sub

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

func TestAuditExternalFetchStatusDoesNotCopyURLCredentials(t *testing.T) {
	seedSubDB(t)
	const address = "https://synthetic-user:synthetic-password@provider.example/sub/synthetic-path?token=synthetic-token"
	row := model.ClientExternalLink{Kind: model.ExternalLinkKindSubscription, Value: address}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	recordExternalSubscriptionFetch(address, &url.Error{Op: "Get", URL: address, Err: errors.New("synthetic transport failure")})
	var stored model.ClientExternalLink
	if err := database.GetDB().First(&stored, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.LastFetchError, "synthetic transport failure") {
		t.Fatal("fetch failure diagnostic lost")
	}
	for _, secret := range []string{"synthetic-user", "synthetic-password", "synthetic-path", "synthetic-token"} {
		if strings.Contains(stored.LastFetchError, secret) {
			t.Error("URL credentials copied into fetch error")
		}
	}
	const callback = "audit:failed-fetch-status-write"
	if err := database.GetDB().Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		tx.AddError(errors.New("synthetic status write failure"))
	}); err != nil {
		t.Fatal(err)
	}
	defer database.GetDB().Callback().Update().Remove(callback)
	recordExternalSubscriptionFetch(address, nil)
	lines := logger.GetLogs(1, "warning")
	if len(lines) != 1 || !strings.Contains(lines[0], "synthetic status write failure") {
		t.Fatal("missing storage failure diagnostic")
	}
	for _, secret := range []string{"synthetic-user", "synthetic-password", "synthetic-path", "synthetic-token"} {
		if strings.Contains(lines[0], secret) {
			t.Error("provider URL credentials leaked into the panel log")
		}
	}
}
