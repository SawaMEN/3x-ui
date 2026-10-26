package sub

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestNativeSubscriptionUsesSharedRemarkTemplate(t *testing.T) {
	for _, protocol := range []model.Protocol{model.VLESS, model.Trojan, model.Hysteria, model.TUIC, model.AnyTLS, model.NaiveProxy, model.Snell, model.Mieru} {
		t.Run(string(protocol), func(t *testing.T) {
			initSubDB(t)
			db := database.GetDB()
			client := &model.ClientRecord{Email: "alice@example.com", SubID: "names", UUID: "11111111-2222-4333-8444-555555555555", Password: "secret-password-123456789012345678", Enable: true}
			if err := db.Create(client).Error; err != nil {
				t.Fatal(err)
			}
			settings := fmt.Sprintf(`{"version":2,"clients":[{"id":%q,"email":%q,"password":%q,"enable":true}],"protocols":["TCP"]}`, client.UUID, client.Email, client.Password)
			if protocol == model.Snell {
				settings = fmt.Sprintf(`{"version":5,"psk":"server-psk-secret","clients":[{"email":%q,"password":%q,"enable":true}]}`, client.Email, client.Password)
			}
			inbound := &model.Inbound{UserId: 1, Tag: string(protocol), Protocol: protocol, Enable: true, Remark: "Office", Port: 8443, Settings: settings, StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"vpn.example.com"}}`}
			if err := db.Create(inbound).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: inbound.Id}).Error; err != nil {
				t.Fatal(err)
			}
			seedHost(t, &model.Host{InboundId: inbound.Id, Remark: "Relay", Address: "vpn.example.com", Port: 443})
			svc := NewSubJsonService("", "", "", "", NewSubService("{{INBOUND}} / {{HOST}}"))
			body, _, err := svc.GetHiddifySingBoxJson(client.SubID, "sub.example.com")
			if err != nil {
				t.Fatal(err)
			}
			var cfg struct {
				Outbounds []map[string]any `json:"outbounds"`
			}
			if err := json.Unmarshal([]byte(body), &cfg); err != nil {
				t.Fatal(err)
			}
			if len(cfg.Outbounds) != 1 || cfg.Outbounds[0]["tag"] != "Office / Relay" {
				t.Fatalf("unexpected names: %s", body)
			}
		})
	}
}
