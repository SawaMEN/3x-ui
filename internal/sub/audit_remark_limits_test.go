package sub

import (
	"net/url"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestAuditRemarkRecoversOmittedRuntimeLimits(t *testing.T) {
	seedSubDB(t)
	row := coremodel.ClientTraffic{Email: "remark@e", Up: 11, Down: 22, LastOnline: 42}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	client := model.Client{Email: row.Email, Enable: true, TotalGB: 1024, ExpiryTime: 1893456000000}
	for _, source := range []string{"inbound", "cache", "database"} {
		t.Run(source, func(t *testing.T) {
			s := &SubService{}
			inbound := &model.Inbound{}
			switch source {
			case "inbound":
				inbound.ClientStats = []coremodel.ClientTraffic{row}
			case "cache":
				s.statsByEmail = map[string]coremodel.ClientTraffic{row.Email: row}
			}
			got := s.statsForClient(inbound, client)
			if got.Total != client.TotalGB || got.ExpiryTime != client.ExpiryTime || got.Up != 11 || got.Down != 22 || got.LastOnline != 42 || got.Enable != row.Enable {
				t.Fatalf("remark limits/usage mismatch: %+v", got)
			}
		})
	}
	var stored coremodel.ClientTraffic
	if err := database.GetDB().Where("email = ?", row.Email).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Total != 0 || stored.ExpiryTime != 0 {
		t.Fatal("rendering must not write presentation fallbacks to runtime statistics")
	}
}

func TestAuditRawRemarkMatchesSubscriptionLimits(t *testing.T) {
	seedSubDB(t)
	seedSubInbound(t, "remark-limits", "remark", 4601, 1, dnsTestStream)
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "remark@e").Updates(map[string]any{"total_gb": 1024, "expiry_time": int64(1893456000000)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&coremodel.ClientTraffic{Email: "remark@e", Enable: true, Up: 11, Down: 22}).Error; err != nil {
		t.Fatal(err)
	}
	s := NewSubService("{{INBOUND}}|{{TRAFFIC_TOTAL_BYTES}}|{{EXPIRE_UNIX}}").ForRequest("sub.example")
	s.subscriptionBody = true
	links, _, _, traffic, err := s.getSubs("remark-limits")
	if err != nil || len(links) != 1 {
		t.Fatalf("links=%d err=%v", len(links), err)
	}
	u, err := url.Parse(links[0])
	if err != nil {
		t.Fatal(err)
	}
	if u.Fragment != "remark|1024|1893456000" || traffic.Total != 1024 || traffic.ExpiryTime != 1893456000000 {
		t.Fatalf("remark=%q does not match aggregate=%+v", u.Fragment, traffic)
	}
}
