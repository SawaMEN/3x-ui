package service

import (
	"fmt"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestStage5TransportRefusedBeforePersistence(t *testing.T) {
	setupBulkDB(t)
	for _, tc := range []struct {
		core              string
		protocol          model.Protocol
		network, security string
	}{
		{CoreTypeXray, model.VLESS, "http", "tls"},
		{CoreTypeXray, model.VLESS, "quic", "tls"},
		{CoreTypeXray, model.VLESS, "masque", "none"},
		{CoreTypeSingBoxUpstream, model.VLESS, "kcp", "none"},
		{CoreTypeSingBoxUpstream, model.VLESS, "xhttp", "none"},
		{CoreTypeSingBoxUpstream, model.VLESS, "xdrive", "none"},
		{CoreTypeSingBox, model.VMESS, "xdrive", "none"},
		{CoreTypeSingBox, model.VLESS, "quic", "tls"},
		{CoreTypeSingBox, model.Shadowsocks, "ws", "none"},
		{CoreTypeSingBox, model.Shadowsocks, "tcp", "tls"},
	} {
		t.Run(tc.core+"/"+string(tc.protocol)+"/"+tc.network+"/"+tc.security, func(t *testing.T) {
			if err := (&SettingService{}).setString("coreType", tc.core); err != nil {
				t.Fatal(err)
			}
			svc := &InboundService{}
			stream := fmt.Sprintf(`{"network":%q,"security":%q}`, tc.network, tc.security)
			candidate := &model.Inbound{Protocol: tc.protocol, Settings: `{"clients":[]}`, StreamSettings: stream, Tag: t.Name()}
			if _, _, err := svc.AddInbound(candidate); err == nil {
				t.Fatal("unsupported transport saved")
			}
			var count int64
			if err := database.GetDB().Model(&model.Inbound{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("add modified DB: %d %v", count, err)
			}
			stored := &model.Inbound{Protocol: tc.protocol, Tag: t.Name(), Settings: `{"clients":[]}`, StreamSettings: `{"network":"tcp","security":"none"}`}
			if err := database.GetDB().Create(stored).Error; err != nil {
				t.Fatal(err)
			}
			update := *stored
			update.StreamSettings = stream
			if _, _, err := svc.UpdateInbound(&update); err == nil {
				t.Fatal("unsupported new stream accepted using old stored stream")
			}
			got, err := svc.GetInbound(stored.Id)
			if err != nil || got.StreamSettings != stored.StreamSettings {
				t.Fatal("update modified DB", err)
			}
			// Imported rows may still use the method alias. Enable must apply
			// the same precedence as Add/Update without rewriting stored JSON.
			alias := fmt.Sprintf(`{"network":"tcp","method":%q,"security":%q}`, tc.network, tc.security)
			if err := database.GetDB().Model(stored).Updates(map[string]any{"stream_settings": alias, "enable": false}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := svc.SetInboundEnable(stored.Id, true); err == nil {
				t.Fatal("unsupported method alias enabled")
			}
			got, err = svc.GetInbound(stored.Id)
			if err != nil || got.Enable || got.StreamSettings != alias {
				t.Fatal("enable modified DB", err)
			}
			if err := database.GetDB().Delete(stored).Error; err != nil {
				t.Fatal(err)
			}
		})
	}
}
