package sub

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestLinkExportUsesCurrentNamingWithoutReplacingProtocolSecrets(t *testing.T) {
	for _, protocol := range []model.Protocol{model.NaiveProxy, model.AnyTLS, model.Mieru, model.Snell, model.Sudoku, model.TrustTunnel, model.FPTN, model.VetroffProxy, model.OpenFlux, model.WireGuard, model.AmneziaWG} {
		t.Run(string(protocol), func(t *testing.T) {
			initSubDB(t)
			db := database.GetDB()
			mode := model.ConnectionNameClient
			row := model.ClientRecord{Email: "name@example.org", Enable: true, ConnectionNameMode: &mode, Password: "normalized-password", UpdatedAt: 2000}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			ib := model.Inbound{Protocol: protocol, Tag: string(protocol), Enable: true, Settings: fmt.Sprintf(`{"clients":[{"email":%q,"connectionNameMode":"remark","comment":"deleted label","password":"protocol-password","privateKey":"protocol-key","enable":true}]}`, row.Email)}
			if err := db.Create(&ib).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.ClientInbound{ClientId: row.Id, InboundId: ib.Id}).Error; err != nil {
				t.Fatal(err)
			}
			clients, err := NewSubService("").clientsForLinkExport(&ib)
			if err != nil || len(clients) != 1 {
				t.Fatalf("clients: %v %+v", err, clients)
			}
			client := clients[0]
			if got := client.ConnectionDisplayName("old inbound label"); got != row.Email || client.Comment != "" {
				t.Fatalf("stale naming: name=%q comment=%q", got, client.Comment)
			}
			if protocol != model.NaiveProxy && (client.Password != "protocol-password" || client.PrivateKey != "protocol-key") {
				t.Fatal("naming overlay changed protocol-specific secrets")
			}
		})
	}
}

func TestSubscriptionNamesRefreshAfterRenameClearAndStaleSiblingSync(t *testing.T) {
	initSubDB(t)
	db := database.GetDB()
	mode := model.ConnectionNameClient
	row := model.ClientRecord{Email: "current@example.org", SubID: "rename-sub", Enable: true, Password: "shared-password", ConnectionNameMode: &mode, UpdatedAt: 2000}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	var ids []int
	for i, protocol := range []model.Protocol{model.NaiveProxy, model.AnyTLS} {
		ib := model.Inbound{
			Protocol:       protocol,
			Tag:            string(protocol),
			Remark:         "old inbound",
			Enable:         true,
			Port:           30400 + i,
			Settings:       fmt.Sprintf(`{"clients":[{"email":%q,"password":"shared-password","enable":true,"connectionNameMode":"remark","comment":"deleted label","updated_at":1000}]}`, row.Email),
			StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"vpn.example.org"}}`,
		}
		if err := db.Create(&ib).Error; err != nil {
			t.Fatal(err)
		}
		staleMode := model.ConnectionNameRemark
		if err := (&service.ClientService{}).SyncInbound(nil, ib.Id, []model.Client{{Email: row.Email, SubID: row.SubID, Password: row.Password, Enable: true, ConnectionNameMode: &staleMode, Comment: "deleted label", UpdatedAt: 1000}}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, ib.Id)
	}
	sub := NewSubService("")
	sub.subscriptionBody = true
	t.Cleanup(sub.enableSettingsCache())
	native := NewSubJsonService("", "", "", "", sub)
	check := func(want string) {
		t.Helper()
		links, _, _, _, err := sub.GetSubs(row.SubID, "sub.example.org")
		if err != nil || len(links) != len(ids) {
			t.Fatalf("subscription: %v %v", err, links)
		}
		for _, link := range links {
			u, err := url.Parse(link)
			if err != nil || u.Fragment != want {
				t.Fatalf("stale name: link=%q want=%q err=%v", link, want, err)
			}
		}
		body, _, err := native.GetHiddifySingBoxJson(row.SubID, "sub.example.org")
		if err != nil {
			t.Fatal(err)
		}
		var profile struct {
			Outbounds []struct{ Tag string } `json:"outbounds"`
		}
		if err := json.Unmarshal([]byte(body), &profile); err != nil || len(profile.Outbounds) != len(ids) {
			t.Fatalf("native subscription: %v %s", err, body)
		}
		for _, outbound := range profile.Outbounds {
			if !strings.HasPrefix(outbound.Tag, want) {
				t.Fatalf("stale native name: %q, want prefix %q", outbound.Tag, want)
			}
		}
	}
	check(row.Email)
	mode = model.ConnectionNameRemark
	if err := db.Model(&row).Updates(map[string]any{"connection_name_mode": mode, "updated_at": 3000}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Inbound{}).Where("id IN ?", ids).Update("remark", "new inbound").Error; err != nil {
		t.Fatal(err)
	}
	check("new inbound")
	if err := db.Model(&model.Inbound{}).Where("id IN ?", ids).Update("remark", "").Error; err != nil {
		t.Fatal(err)
	}
	check(row.Email)
}
