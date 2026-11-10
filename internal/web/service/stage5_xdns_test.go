package service

import (
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestStage5XDNSFormatNeverSilentlyChanges(t *testing.T) {
	setupBulkDB(t)
	if err := (&SettingService{}).setString("coreType", CoreTypeXray); err != nil {
		t.Fatal(err)
	}
	const legacy = `{"network":"kcp","finalmask":{"udp":[{"type":"xdns","settings":{"domains":["example.com:txt"],"resolvers":["example.com:txt+udp://1.1.1.1:53"]}}]}}`
	svc := &InboundService{}
	candidate := &model.Inbound{Protocol: model.VLESS, Settings: `{"clients":[]}`, StreamSettings: legacy, Tag: "xdns-format"}
	if _, _, err := svc.AddInbound(candidate); err == nil || !strings.Contains(err.Error(), "convert both peers") {
		t.Fatal("legacy xDNS add must explain conversion", err)
	}
	var count int64
	if err := database.GetDB().Model(&model.Inbound{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("add changed storage", count, err)
	}
	stored := &model.Inbound{Protocol: model.VLESS, Settings: `{"clients":[]}`, StreamSettings: `{"network":"tcp"}`, Tag: "xdns-stored"}
	if err := database.GetDB().Create(stored).Error; err != nil {
		t.Fatal(err)
	}
	update := *stored
	update.StreamSettings = legacy
	if _, _, err := svc.UpdateInbound(&update); err == nil || !strings.Contains(err.Error(), "convert both peers") {
		t.Fatal("legacy xDNS update accepted", err)
	}
	got, err := svc.GetInbound(stored.Id)
	if err != nil || got.StreamSettings != stored.StreamSettings {
		t.Fatal("update changed storage", err)
	}
	if err := database.GetDB().Model(stored).Update("stream_settings", legacy).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetInboundEnable(stored.Id, true); err == nil || !strings.Contains(err.Error(), "convert both peers") {
		t.Fatal("legacy xDNS enable accepted", err)
	}
	got, err = svc.GetInbound(stored.Id)
	if err != nil || got.Enable || got.StreamSettings != legacy {
		t.Fatal("enable changed storage", err)
	}
	const current = `{"network":"kcp","finalmask":{"udp":[{"type":"xdns","settings":{"domains":[{"names":["example.com"],"types":[16]}],"resolvers":[{"addrs":["udp://1.1.1.1:53"],"worker":8}]}}]}}`
	if err := validateXDNSMaskShape(current); err != nil {
		t.Fatal("current format rejected", err)
	}
}
