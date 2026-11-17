package sub

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
)

func TestHysteriaNativeSubscriptionOmitsServerOnlySettings(t *testing.T) {
	ib := &model.Inbound{Protocol: model.Hysteria, Listen: "vpn.example.com", Port: 443, Settings: `{"version":2}`, StreamSettings: `{"network":"hysteria","security":"tls","hysteriaSettings":{"version":2,"udpIdleTimeout":3600,"ignoreClientBandwidth":true,"masquerade":{"type":"proxy","url":"https://example.com"},"up":100,"down":250,"obfs":{"type":"salamander","password":"obfs-secret"}}}`}
	raw := (&SubJsonService{}).genHy(ib, map[string]any{}, model.Client{Auth: "client-secret"}, "")
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	options := config["streamSettings"].(map[string]any)["hysteriaSettings"].(map[string]any)
	for _, key := range []string{"udpIdleTimeout", "masquerade", "ignoreClientBandwidth"} {
		if options[key] != nil {
			t.Errorf("server-only option %s leaked into client", key)
		}
	}
	out, err := hiddify.TranslateOutbound(config)
	if err != nil {
		t.Fatal(err)
	}
	if out["type"] != "hysteria2" || out["password"] != "client-secret" || out["up_mbps"] != 100 {
		t.Fatalf("native Hysteria options lost: %#v", out)
	}
}

func TestHysteriaNativeObfsAppearsInURIAndClash(t *testing.T) {
	ib := &model.Inbound{Protocol: model.Hysteria, Listen: "vpn.example.com", Port: 443, Settings: `{"version":2,"clients":[{"email":"alice","auth":"secret","enable":true}]}`, StreamSettings: `{"network":"hysteria","security":"tls","hysteriaSettings":{"version":2,"obfs":{"type":"gecko","password":"obfs-secret","minPacketSize":512,"maxPacketSize":1200}}}`}
	link := (&SubService{}).genHysteriaLink(ib, "alice")
	if !strings.Contains(link, "obfs=gecko") || !strings.Contains(link, "obfs-password=obfs-secret") || !strings.Contains(link, "minPacketSize=512") {
		t.Fatalf("native obfs missing from URI: %s", link)
	}
	proxy := (&SubClashService{}).buildHysteriaProxy(&SubService{}, ib, model.Client{Email: "alice", Auth: "secret"}, nil)
	if proxy["obfs"] != "gecko" || proxy["obfs-password"] != "obfs-secret" || proxy["obfs-min-packet-size"] != 512 {
		t.Fatalf("native obfs missing from Clash: %#v", proxy)
	}
}
