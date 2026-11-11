package sub

import (
	"fmt"
	"math"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestAuditUserinfoBeforeFirstTrafficSnapshot(t *testing.T) {
	router, subID := initHwidSubRouter(t, 0)
	const quota, expiry = int64(10000), int64(1893456000000)
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("sub_id = ?", subID).
		Updates(map[string]any{"total_gb": quota, "expiry_time": expiry}).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/sub/", "/json/", "/clash/"} {
		response := requestSub(t, router, http.MethodGet, path+subID, "", "")
		want := fmt.Sprintf("upload=0; download=0; total=%d; expire=%d", quota, expiry/1000)
		if response.Code != http.StatusOK || response.Header().Get("Subscription-Userinfo") != want {
			t.Errorf("%s: status=%d userinfo=%q, want %q", path, response.Code, response.Header().Get("Subscription-Userinfo"), want)
		}
	}
	var count int64
	if err := database.GetDB().Model(&coremodel.ClientTraffic{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("read-only aggregation must not create traffic rows")
	}
}

func TestAuditUserinfoAggregationCacheParity(t *testing.T) {
	seedSubDB(t)
	db := database.GetDB()
	for _, email := range []string{"snapshot@e", "not-yet-seen@e"} {
		if err := db.Create(&model.ClientRecord{Email: email, TotalGB: 1000, ExpiryTime: 1893456000000, ResetDay: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	row := coremodel.ClientTraffic{Email: "snapshot@e", Up: 10, Down: 20, LastOnline: 42}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	for _, indexed := range []bool{false, true} {
		s := &SubService{statsIndexed: indexed, statsByEmail: map[string]coremodel.ClientTraffic{row.Email: row}}
		agg, online := s.AggregateTrafficByEmails([]string{row.Email, "not-yet-seen@e", row.Email})
		if agg.Up != 10 || agg.Down != 20 || agg.Total != 2000 || agg.ExpiryTime != 1893456000000 || agg.ResetDay != 1 || online != 42 {
			t.Errorf("indexed=%v: aggregate=%+v online=%d", indexed, agg, online)
		}
	}
}

func TestAuditUserinfoAggregationSaturates(t *testing.T) {
	seedSubDB(t)
	for _, email := range []string{"large-a@e", "large-b@e"} {
		if err := database.GetDB().Create(&coremodel.ClientTraffic{Email: email, Up: math.MaxInt64 - 1, Down: math.MaxInt64 - 2, Total: math.MaxInt64}).Error; err != nil {
			t.Fatal(err)
		}
	}
	agg, _ := (&SubService{}).AggregateTrafficByEmails([]string{"large-a@e", "large-b@e"})
	if agg.Up != math.MaxInt64 || agg.Down != math.MaxInt64 || agg.Total != math.MaxInt64 {
		t.Fatalf("aggregate wrapped instead of saturating: %+v", agg)
	}
	if got := subscriptionExpiryFromClient(100, math.MinInt64); got != math.MaxInt64 {
		t.Fatalf("delayed expiry overflow: got %d", got)
	}
}

func TestAuditSubscriptionReadsTrafficOnce(t *testing.T) {
	seedSubDB(t)
	seedSubInbound(t, "traffic-query-budget", "query-client", 4596, 1, dnsTestStream)
	if err := database.GetDB().Create(&coremodel.ClientTraffic{Email: "query-client@e", Up: 123, Down: 456}).Error; err != nil {
		t.Fatal(err)
	}
	queries := 0
	cb := database.GetDB().Callback().Query()
	if err := cb.Before("gorm:query").Register("audit:traffic-query-budget", func(tx *gorm.DB) {
		if tx.Statement.Table == "client_traffics" {
			queries++
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cb.Remove("audit:traffic-query-budget") })
	links, _, _, traffic, err := NewSubService("{{TRAFFIC_USED}}").GetSubs("traffic-query-budget", "sub.example")
	if err != nil || len(links) != 1 || traffic.Up != 123 || traffic.Down != 456 {
		t.Fatalf("subscription changed: links=%d traffic=%+v err=%v", len(links), traffic, err)
	}
	if queries != 1 {
		t.Fatalf("traffic queries=%d, want one subscriber-scoped snapshot", queries)
	}
}
