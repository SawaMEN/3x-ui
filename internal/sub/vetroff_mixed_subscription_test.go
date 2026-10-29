package sub

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestVetroffMixedSubscriptionKeepsNativeLinks(t *testing.T) {
	seedSubDB(t)
	const subID = "vetroff-mixed"
	const email = "vetroff@example.com"
	password := strings.Repeat("v", 48)
	db := database.GetDB()
	record := &model.ClientRecord{Email: email, SubID: subID, Password: password, Enable: true}
	if err := db.Create(record).Error; err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []model.Protocol{model.AnyTLS, model.VetroffProxy} {
		raw, err := json.Marshal(map[string]any{
			"hostname":    "vpn.example.com",
			"certificate": "/etc/ssl/vetroff/fullchain.pem",
			"privateKey":  "/etc/ssl/vetroff/privkey.pem",
			"tls":         map[string]any{"serverName": "vpn.example.com"},
			"clients":     []model.Client{{Email: email, Password: password, Enable: true, SubID: subID}},
		})
		if err != nil {
			t.Fatal(err)
		}
		ib := &model.Inbound{UserId: 1, Tag: "mixed-" + string(protocol), Protocol: protocol, Enable: true, Port: 18443, Settings: string(raw)}
		if err := db.Create(ib).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: record.Id, InboundId: ib.Id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := NewSubJsonService("", "", "", "", NewSubService(""))
	body, _, err := service.GetHiddifySingBoxJson(subID, "vpn.example.com")
	if !errors.Is(err, errSubscriptionFormatUnsupported) || body != "" {
		t.Fatalf("partial Hiddify JSON hides VetroffProxy: %s, %v", body, err)
	}
	links, _, _, _, err := NewSubService("").GetSubs(subID, "vpn.example.com")
	if err != nil {
		t.Fatal(err)
	}
	body = strings.Join(links, "\n")
	if !strings.Contains(body, "vetroff://") || !strings.Contains(body, "anytls://") {
		t.Fatalf("raw subscription lost a connection: %s", body)
	}
	for _, link := range links {
		if strings.HasPrefix(link, "vetroff://") && applyRemarkToLink(link, "Renamed") != link {
			t.Fatal("display name corrupts the native URI")
		}
	}
}
