package sub

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

func TestManagedDomainsAppearInClientLinksWithoutSubscriptionOnlyHost(t *testing.T) {
	seedSubDB(t)
	inbound := seedSubInbound(t, "domain-client", "domain-ws", 15002, 1, `{"network":"ws","security":"none","wsSettings":{"path":"/vpn"}}`)
	seedHost(t, &model.Host{InboundId: inbound.Id, Remark: "manual", Address: "manual.example.com", Port: 8443, Security: "tls"})
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.SubscriptionDomain = "subscription.example.com"
	cfg.Domains = []domainfront.DomainFrontDomain{
		{Name: "direct.example.com", Enabled: true, Mode: "direct", TLS: "auto", InboundIDs: []int{inbound.Id}},
		{Name: "cdn.example.com", Enabled: true, Mode: "cdn", TLS: "auto", InboundIDs: []int{inbound.Id}},
		{Name: "subscription.example.com", Enabled: true, Mode: "sub_link_only", TLS: "auto"},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.Setting{Key: "domainFrontConfig", Value: string(raw)}).Error; err != nil {
		t.Fatal(err)
	}
	links, _, _, _, err := NewSubService("").GetSubs("domain-client", "request.example.com")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(links, "\n")
	for _, host := range []string{"manual.example.com:8443", "direct.example.com:443", "cdn.example.com:443"} {
		if !strings.Contains(joined, host) {
			t.Fatalf("missing host %q in %s", host, joined)
		}
	}
	if strings.Contains(joined, "subscription.example.com") {
		t.Fatalf("subscription-only domain became a VPN endpoint: %s", joined)
	}
	if strings.Count(joined, "security=tls") != 3 {
		t.Fatalf("managed TLS was not applied to links: %s", joined)
	}
}
