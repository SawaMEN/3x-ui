package sub

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
)

func TestAuditRoutingCacheRetriesTransientDatabaseRead(t *testing.T) {
	initSubDB(t)
	const source = "https://routing.example/cache.json"
	content, err := normalizeHappRouting([]byte(`{"Name":"persisted"}`))
	if err != nil {
		t.Fatal(err)
	}
	resolver := newRemoteRoutingResolver(nil, true)
	resolver.persistEntry(remoteRoutingHapp, remoteRoutingCacheEntry{Source: source, Content: content, FetchedAt: time.Now().Unix()})
	fail := true
	cb := database.GetDB().Callback().Query()
	if err := cb.Before("gorm:query").Register("audit:routing-load-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "settings" && fail {
			fail = false
			_ = tx.AddError(errors.New("temporary read failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cb.Remove("audit:routing-load-failure") })
	resolver.ensurePersistedLoaded()
	if resolver.loaded {
		t.Fatal("failed database read permanently marked cache loaded")
	}
	resolver.ensurePersistedLoaded()
	if !resolver.loaded || resolver.entries[remoteRoutingKey{kind: remoteRoutingHapp, source: source}].Content != content {
		t.Fatal("last-good routing cache was not recovered after database retry")
	}
}

func TestAuditRoutingCacheRejectsOversizedPersistedClash(t *testing.T) {
	for _, extra := range []int{0, 1} {
		t.Run([]string{"at limit", "over limit"}[extra], func(t *testing.T) {
			initSubDB(t)
			const source = "https://routing.example/cache.yaml"
			const prefix = "rules:\n  - MATCH,DIRECT\n#"
			content := prefix + strings.Repeat("x", remoteRoutingClashMaxBody-len(prefix)-1+extra) + "\n"
			resolver := newRemoteRoutingResolver(nil, true)
			resolver.persistEntry(remoteRoutingClash, remoteRoutingCacheEntry{Source: source, Content: content, FetchedAt: time.Now().Unix()})
			resolver.ensurePersistedLoaded()
			_, exists := resolver.entries[remoteRoutingKey{kind: remoteRoutingClash, source: source}]
			if exists != (extra == 0) {
				t.Fatalf("content length=%d loaded=%v", len(content), exists)
			}
		})
	}
}
