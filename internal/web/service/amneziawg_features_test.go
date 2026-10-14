package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/wireguard"
)

func TestNativeAWGOutboundGeneratedConfig(t *testing.T) {
	priv, pub, err := wireguard.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "bridge", true: "native"}[native], func(t *testing.T) {
			settings := map[string]any{
				"native": native, "secretKey": priv, "address": []string{"10.8.1.2/32"}, "jc": 4, "jmin": 10, "jmax": 20, "s4": 12,
				"peers": []any{map[string]any{"publicKey": pub, "endpoint": "127.0.0.1:51820", "allowedIPs": []string{"0.0.0.0/0"}, "keepAlive": 25}},
			}
			template, _ := json.Marshal(map[string]any{"outbounds": []any{map[string]any{"protocol": "amneziawg", "tag": "awg-egress", "settings": settings}, map[string]any{"protocol": "freedom", "tag": "direct"}}, "routing": map[string]any{"rules": []any{map[string]any{"type": "field", "domain": []string{"example.test"}, "outboundTag": "awg-egress"}}}})
			snapshot := &nativeConfigSnapshot{capturedAt: time.Now(), settings: map[string]string{"xrayTemplateConfig": string(template), "singBoxConfigTemplate": "{}"}}
			cfg, err := NativeCore(CoreTypeHiddify).generateNativeConfig(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if native {
				if len(cfg.Endpoints) != 1 || cfg.Endpoints[0]["type"] != "awg" || cfg.Endpoints[0]["tag"] != "awg-egress" {
					t.Fatal("missing native AWG endpoint")
				}
				for _, outbound := range cfg.Outbounds {
					if outbound["tag"] == "awg-egress" {
						t.Fatal("native AWG still emitted a SOCKS bridge")
					}
				}
			} else {
				if len(cfg.Endpoints) != 0 {
					t.Fatal("existing bridge profile migrated implicitly")
				}
				found := false
				for _, outbound := range cfg.Outbounds {
					if outbound["tag"] == "awg-egress" && outbound["type"] == "socks" {
						found = true
					}
				}
				if !found {
					t.Fatal("existing bridge profile lost")
				}
			}
			checkGeneratedHiddifyConfig(t, cfg)
		})
	}
}

func TestAWGClientOptionsPersistInRecordAndPartialUpdates(t *testing.T) {
	setupBulkDB(t)
	jc := 0
	client := model.Client{Email: "awg-options", Enable: true, SubID: "awg-options-sub", AWGClientOptions: &model.AWGClientOptions{Jc: &jc}}
	inbound := mkInbound(t, 51820, model.AmneziaWG, clientsSettings(t, []model.Client{client}))
	svc := &ClientService{}
	if err := svc.SyncInbound(nil, inbound.Id, []model.Client{client}); err != nil {
		t.Fatal(err)
	}
	record := lookupClientRecord(t, client.Email)
	if record.AWGClientOptions == nil || record.AWGClientOptions.Jc == nil || *record.AWGClientOptions.Jc != 0 {
		t.Fatal("explicit zero override was not persisted")
	}
	client.AWGClientOptions = nil
	if err := svc.SyncInbound(nil, inbound.Id, []model.Client{client}); err != nil {
		t.Fatal(err)
	}
	record = lookupClientRecord(t, client.Email)
	if record.AWGClientOptions == nil || record.ToClient().AWGClientOptions.Jc == nil {
		t.Fatal("partial update erased options")
	}
	client.AWGClientOptions = &model.AWGClientOptions{}
	if err := svc.SyncInbound(nil, inbound.Id, []model.Client{client}); err != nil {
		t.Fatal(err)
	}
	record = lookupClientRecord(t, client.Email)
	if record.AWGClientOptions == nil || record.AWGClientOptions.Jc != nil {
		t.Fatal("empty object did not reset overrides")
	}
}

func TestAWGClientOptionsUpdateWithoutAttachments(t *testing.T) {
	setupBulkDB(t)
	jc := 4
	client := model.Client{Email: "orphan-awg", SubID: "orphan-awg-sub", Enable: true, AWGClientOptions: &model.AWGClientOptions{Jc: &jc}}
	record := client.ToRecord()
	if err := database.GetDB().Create(record).Error; err != nil {
		t.Fatal(err)
	}
	zero := 0
	client.AWGClientOptions = &model.AWGClientOptions{Jc: &zero}
	if _, err := (&ClientService{}).Update(&InboundService{}, record.Id, client, 0); err != nil {
		t.Fatal(err)
	}
	got := lookupClientRecord(t, client.Email)
	if got.AWGClientOptions == nil || got.AWGClientOptions.Jc == nil || *got.AWGClientOptions.Jc != 0 {
		t.Fatal("orphan update did not persist the object")
	}
}

func TestAmneziaWGMetricsDeduplicateActivityAndHideCredentials(t *testing.T) {
	setupBulkDB(t)
	client := model.Client{Email: "private@example.test", Enable: true, SubID: "secret-sub"}
	a := mkInbound(t, 51820, model.AmneziaWG, clientsSettings(t, []model.Client{client}))
	b := mkInbound(t, 51821, model.AmneziaWG, clientsSettings(t, []model.Client{client}))
	svc := &ClientService{}
	for _, ib := range []*model.Inbound{a, b} {
		if err := svc.SyncInbound(nil, ib.Id, []model.Client{client}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.GetDB().Create(&coremodel.ClientTraffic{Email: client.Email, InboundId: a.Id, LastOnline: time.Now().UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	metrics, err := (&ServerService{}).AmneziaWGMetrics()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"awg_inbounds_configured 2", "awg_current_online 0", "awg_active_users{window=\"24h\"} 1", "awg_active_users{window=\"30d\"} 1"} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("missing %q in metrics", want)
		}
	}
	for _, forbidden := range []string{client.Email, client.SubID, "privateKey", "publicKey", "endpoint="} {
		if strings.Contains(metrics, forbidden) {
			t.Fatalf("sensitive label %q in metrics", forbidden)
		}
	}
}
