package sub

import (
	"encoding/json"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/util/json_util"
)

func TestXDriveSubscription(t *testing.T) {
	ib := &model.Inbound{Protocol: model.VLESS, Port: 443, Listen: "vpn.example.com", Settings: `{"decryption":"private-key","encryption":"public-key","clients":[]}`, StreamSettings: `{"network":"xdrive","security":"none","xdriveSettings":{"service":"local","remoteFolder":"/shared/tunnel"}}`}
	request := &SubService{}
	service := &SubJsonService{}
	client := model.Client{ID: "12345678-1234-1234-1234-123456789012", Email: "alice", Enable: true}
	if request.GetLink(ib, client.Email) != "" {
		t.Fatal("XDrive must not advertise an incomplete VLESS URI")
	}
	if !containsUnsupportedClashProtocol([]*model.Inbound{ib}) {
		t.Fatal("Clash cannot represent XDrive")
	}
	stream := json_util.RawMessage(ib.StreamSettings)
	raw := service.genVless(request, ib, stream, client, "")
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if config["mux"].(map[string]any)["enabled"] != true {
		t.Fatal("Xray subscription requires Mux")
	}
	request.nativeProfile = true
	raw = service.genVless(request, ib, stream, client, "")
	config = nil
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if config["mux"] != nil {
		t.Fatal("native profile must not enable Xray Mux.Cool")
	}
	translated, err := hiddify.TranslateOutbound(config)
	if err != nil {
		t.Fatal(err)
	}
	if translated["transport"].(map[string]any)["type"] != "xdrive" || translated["encryption"] != "public-key" {
		t.Fatalf("native subscription lost XDrive: %#v", translated)
	}
}
