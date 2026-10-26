package sub

import (
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestNativeSubscriptionHostBindingsAndOrder(t *testing.T) {
	for _, mode := range []string{"inbound-first", "hosts-first"} {
		t.Run(mode, func(t *testing.T) {
			seedSubDB(t)
			setHostSortMode(t, mode)
			a := seedSubInbound(t, "native-hosts", "native-a", 47090, 0, tcpTLSStream)
			b := seedSubInbound(t, "native-hosts", "native-b", 47091, 10, tcpTLSStream)
			allowed := seedHost(t, &model.Host{InboundId: a.Id, GroupId: "native-a", SortOrder: 20, Address: "allowed-a.example.com"})
			seedHost(t, &model.Host{InboundId: a.Id, GroupId: "forbidden-a", SortOrder: 0, Address: "forbidden-a.example.com"})
			seedHost(t, &model.Host{InboundId: b.Id, GroupId: "native-b", SortOrder: 10, Address: "allowed-b.example.com"})
			if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "native-a@e").
				Update("client_host_rule_ids", model.EncodeClientHostRuleIds([]int{allowed.Id})).Error; err != nil {
				t.Fatal(err)
			}
			for _, hiddify := range []bool{false, true} {
				svc := NewSubJsonService("", "", "", "", NewSubService(""))
				render := func() (string, string, error) {
					if hiddify {
						return svc.GetHiddifySingBoxJson("native-hosts", "sub.example.com")
					}
					return svc.GetSingBoxJson("native-hosts", "sub.example.com", false)
				}
				body, _, err := render()
				if err != nil {
					t.Fatal(err)
				}
				ia, ib := strings.Index(body, "allowed-a.example.com"), strings.Index(body, "allowed-b.example.com")
				if ia < 0 || ib < 0 || strings.Contains(body, "forbidden-a.example.com") {
					t.Fatalf("native host binding was ignored (hiddify=%v): %s", hiddify, body)
				}
				if (mode == "hosts-first") != (ib < ia) {
					t.Fatalf("native host order was ignored (mode=%s, hiddify=%v): %s", mode, hiddify, body)
				}
			}
			if err := database.GetDB().Model(allowed).Update("is_disabled", true).Error; err != nil {
				t.Fatal(err)
			}
			body, header, err := NewSubJsonService("", "", "", "", NewSubService("")).GetHiddifySingBoxJson("native-hosts", "sub.example.com")
			if err != nil || header == "" || !strings.Contains(body, "allowed-b.example.com") || strings.Contains(body, "allowed-a.example.com") || strings.Contains(body, "forbidden-a.example.com") {
				t.Fatalf("disabled binding leaked another native endpoint: body=%s header=%s err=%v", body, header, err)
			}
		})
	}
}
