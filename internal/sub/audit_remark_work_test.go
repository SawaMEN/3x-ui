package sub

import (
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestAuditNameOnlyRemarksDoNotQueryTrafficPerHost(t *testing.T) {
	seedSubDB(t)
	ib := seedSubInbound(t, "remark-work", "name-only", 4602, 1, wsTLSStream)
	for i := range 8 {
		seedHost(t, &model.Host{InboundId: ib.Id, Address: fmt.Sprintf("host-%d.example", i)})
	}
	queries := 0
	cb := database.GetDB().Callback().Query()
	if err := cb.Before("gorm:query").Register("audit:name-only-traffic", func(tx *gorm.DB) {
		if tx.Statement.Table == "client_traffics" {
			queries++
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cb.Remove("audit:name-only-traffic") })
	links, _, _, _, err := NewSubService("{{INBOUND}}-{{EMAIL}}").GetSubs("remark-work", "sub.example")
	if err != nil || len(splitLinkLines(strings.Join(links, "\n"))) != 8 {
		t.Fatalf("host links changed: links=%d err=%v", len(links), err)
	}
	if queries != 2 {
		t.Fatalf("traffic queries=%d, want only preload and aggregate, independent of host count", queries)
	}
}

func BenchmarkAuditNameOnlyRemark(b *testing.B) {
	s := &SubService{remarkTemplate: "{{INBOUND}}-{{EMAIL}}"}
	inbound := &model.Inbound{Remark: "node", StreamSettings: `{"network":"ws","security":"tls","tlsSettings":{"serverName":"example.com","opaque":"` + strings.Repeat("x", 8192) + `"}}`, ClientStats: []coremodel.ClientTraffic{{Email: "alice@e", Up: 42}}}
	client := model.Client{Email: "alice@e"}
	b.ReportAllocs()
	for b.Loop() {
		if s.genTemplatedRemark(inbound, client, "", "ws") != "node-alice@e" {
			b.Fatal("remark changed")
		}
	}
}
