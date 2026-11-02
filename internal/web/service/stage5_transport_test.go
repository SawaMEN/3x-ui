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
			if err := database.GetDB().Delete(stored).Error; err != nil {
				t.Fatal(err)
			}
		})
	}
}
