package sub

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestManagerKeepsNativeProtocolLinks(t *testing.T) {
	seedSubDB(t)
	service.RegisterSubLinkProvider(NewLinkProvider())
	t.Cleanup(func() { service.RegisterSubLinkProvider(nil) })
	db := database.GetDB()
	c := &model.ClientRecord{Email: "manager-native", SubID: "original", UUID: uuid.NewString(), Password: strings.Repeat("p", 48), Secret: strings.Repeat("a", 32), Enable: true}
	if err := db.Create(c).Error; err != nil {
		t.Fatal(err)
	}
	sub := &model.ManagerSubscription{UserID: 1, Token: uuid.NewString(), Name: "Native", Enabled: true, ClientID: c.Id, IntervalMinutes: 60}
	for i, proto := range []model.Protocol{model.VLESS, model.AnyTLS, model.TUIC, model.MTProto, model.Mieru, model.VetroffProxy} {
		settings := map[string]any{"clients": []model.Client{*c.ToClient()}, "hostname": "vpn.example.com", "certificate": "/etc/ssl/vetroff/fullchain.pem", "privateKey": "/etc/ssl/vetroff/privkey.pem", "tls": map[string]any{"serverName": "vpn.example.com"}, "protocols": []string{"TCP"}}
		stream := map[string]any{"network": "tcp", "security": "none"}
		if proto == model.AnyTLS || proto == model.TUIC {
			stream = map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{"serverName": "vpn.example.com"}}
		}
		raw, _ := json.Marshal(settings)
		wire, _ := json.Marshal(stream)
		ib := &model.Inbound{UserId: 1, Tag: "manager-test-" + string(proto), Enable: true, Protocol: proto, Port: 18000 + i, Settings: string(raw), StreamSettings: string(wire), ShareAddrStrategy: "custom", ShareAddr: "vpn.example.com"}
		if err := db.Create(ib).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: c.Id, InboundId: ib.Id}).Error; err != nil {
			t.Fatal(err)
		}
		sub.Published = append(sub.Published, model.ManagerEntry{Mode: "existing", ActiveInboundID: ib.Id, Label: "Office " + string(proto)})
	}
	if err := db.Create(sub).Error; err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	service.ServeManagerSubscription(w, httptest.NewRequest(http.MethodGet, "https://panel.example/bus/"+sub.Token, nil), sub.Token)
	data, err := base64.StdEncoding.DecodeString(w.Body.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"vless://", "anytls://", "tuic://", "tg://proxy?", "mierus://", "vetroff://"} {
		if !strings.Contains(string(data), prefix) {
			t.Fatalf("missing %s: %s", prefix, data)
		}
	}
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response %d", w.Code)
	}
	if !strings.Contains(string(data), "Office%20anytls") {
		t.Fatalf("label lost: %s", data)
	}
}

func TestManagerJSONKeepsMASQUEAndExcludesOtherClientAttachments(t *testing.T) {
	seedSubDB(t)
	service.RegisterSubLinkProvider(NewLinkProvider())
	t.Cleanup(func() { service.RegisterSubLinkProvider(nil) })
	db := database.GetDB()
	c := &model.ClientRecord{Email: "masque-manager", SubID: "masque-original", Password: "current-secret", Enable: true}
	if err := db.Create(c).Error; err != nil {
		t.Fatal(err)
	}
	ib := &model.Inbound{UserId: 1, Tag: "masque-manager", Protocol: model.MASQUE, Enable: true, Port: 18443, ShareAddrStrategy: "custom", ShareAddr: "masque.example.com", Settings: `{"version":[1,2],"path":"/tunnel{?target,ipproto}","tls":{"serverName":"tls.example"},"clients":[{"email":"masque-manager","password":"current-secret","subId":"masque-original","enable":true}]}`}
	other := &model.Inbound{UserId: 1, Tag: "unselected", Protocol: model.VLESS, Enable: true, Port: 19999, ShareAddrStrategy: "custom", ShareAddr: "unselected.example.com", Settings: `{"clients":[],"decryption":"none"}`, StreamSettings: `{"network":"tcp","security":"none"}`}
	for _, row := range []*model.Inbound{ib, other} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: c.Id, InboundId: row.Id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	sub := &model.ManagerSubscription{UserID: 1, Token: uuid.NewString(), Name: "MASQUE", Enabled: true, ClientID: c.Id, IntervalMinutes: 60, Published: []model.ManagerEntry{{Mode: "existing", ActiveInboundID: ib.Id, Label: "MASQUE"}}}
	if err := db.Create(sub).Error; err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	service.ServeManagerSubscription(w, httptest.NewRequest(http.MethodGet, "https://panel.example/bus/"+sub.Token+"?format=singbox", nil), sub.Token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "masque-client") || !strings.Contains(w.Body.String(), "current-secret") || strings.Contains(w.Body.String(), "unselected.example.com") {
		t.Fatalf("JSON selection: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	service.ServeManagerSubscription(w, httptest.NewRequest(http.MethodGet, "https://panel.example/bus/"+sub.Token, nil), sub.Token)
	if w.Code != 406 {
		t.Fatalf("raw format silently discarded MASQUE: %d %s", w.Code, w.Body.String())
	}
}
