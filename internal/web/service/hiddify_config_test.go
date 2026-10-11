package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func checkGeneratedHiddifyConfig(t *testing.T, cfg *singbox.Config) []byte {
	t.Helper()
	data, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY"); binary != "" {
		binary, err = filepath.Abs(binary)
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("XUI_BIN_FOLDER", filepath.Dir(binary))
		path := filepath.Join(t.TempDir(), "hiddify-core.json")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := hiddify.NewProcess(path, true).Validate(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	return data
}

func seedHiddifyConfigInbound(t *testing.T, protocol model.Protocol, settings, stream string) *model.Inbound {
	t.Helper()
	ib := &model.Inbound{Enable: true, Protocol: protocol, Tag: "managed-" + string(protocol), Port: 19443, Settings: settings, StreamSettings: stream, Sniffing: `{"enabled":true,"destOverride":["http","tls"]}`}
	db := database.GetDB()
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	client := &model.ClientRecord{Email: "alice", UUID: "11111111-1111-4111-8111-111111111111", Password: "test-password", Auth: "test-password", Enable: true}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	return ib
}

func TestHiddifyFullDatabaseProtocolConfigurations(t *testing.T) {
	for _, tc := range []struct {
		name              string
		protocol          model.Protocol
		settings, network string
	}{
		{"VLESS", model.VLESS, `{"decryption":"none"}`, "tcp"},
		{"VMess", model.VMESS, `{}`, "tcp"},
		{"Trojan", model.Trojan, `{}`, "tcp"},
		{"Shadowsocks2022", model.Shadowsocks, `{"method":"2022-blake3-aes-128-gcm","password":"MDEyMzQ1Njc4OWFiY2RlZg=="}`, "tcp"},
		{"HTTP", model.HTTP, `{}`, "tcp"},
		{"Mixed", model.Mixed, `{}`, "tcp"},
		{"Hysteria1", model.Hysteria, `{}`, "hysteria1"},
		{"Hysteria2", model.Hysteria, `{}`, "hysteria2"},
		{"TUIC", model.TUIC, `{}`, "tuic"},
		{"Naive", model.NaiveProxy, `{"network":"tcp"}`, "tcp"},
		{"AnyTLS", model.AnyTLS, `{"paddingScheme":["stop=8","0=30-30"]}`, "tcp"},
		{"ShadowTLS", model.ShadowTLS, `{"version":3,"innerKey":"MDEyMzQ1Njc4OWFiY2RlZg==","handshake":{"server":"example.com","serverPort":443}}`, "tcp"},
		{"Snell6", model.Snell, `{"version":6,"psk":"test-psk-12345678","mode":"unshaped","clients":[{"email":"alice","password":"1111111111111111111111111111111111111111111111111111111111111111","enable":true}]}`, "tcp"},
		{"MASQUE", model.MASQUE, `{"version":[3],"address":["10.123.0.1/24"]}`, "tcp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupBulkDB(t)
			cert, key := writeTestCertificate(t)
			if err := singBoxSettingService.setString("webCertFile", cert); err != nil {
				t.Fatal(err)
			}
			if err := singBoxSettingService.setString("webKeyFile", key); err != nil {
				t.Fatal(err)
			}
			stream := map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{"certificates": []any{map[string]any{"certificateFile": cert, "keyFile": key}}}}
			if strings.HasPrefix(tc.network, "hysteria") {
				version := 1
				if tc.network == "hysteria2" {
					version = 2
				}
				stream["network"] = "hysteria"
				stream["hysteriaSettings"] = map[string]any{"version": version, "up": 100, "down": 100}
			}
			settings := tc.settings
			if tc.protocol == model.TUIC {
				settings = fmt.Sprintf(`{"server":{"certificate":%q,"private_key":%q,"alpn":["h3"],"congestion_control":"bbr","authentication_timeout":5}}`, cert, key)
			}
			if tc.protocol == model.Shadowsocks || tc.protocol == model.ShadowTLS || tc.protocol == model.Snell {
				stream = map[string]any{}
			}
			encoded, _ := json.Marshal(stream)
			ib := seedHiddifyConfigInbound(t, tc.protocol, settings, string(encoded))
			if tc.protocol == model.Shadowsocks {
				if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Update("password", "YWJjZGVmZ2hpamtsbW5vcA==").Error; err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := (&HiddifyConfigGenerator{}).Generate()
			if err != nil {
				t.Fatal(err)
			}
			data := checkGeneratedHiddifyConfig(t, cfg)
			if !strings.Contains(string(data), ib.Tag) || !strings.Contains(string(data), `"alice"`) {
				t.Fatal("managed listener or identity missing")
			}
		})
	}
}

func TestHiddifyDatabaseTransportConfigurations(t *testing.T) {
	for _, network := range []string{"tcp", "ws", "http", "grpc", "httpupgrade", "xhttp"} {
		t.Run(network, func(t *testing.T) {
			setupBulkDB(t)
			cert, key := writeTestCertificate(t)
			stream := map[string]any{"network": network, "security": "tls", "tlsSettings": map[string]any{"certificates": []any{map[string]any{"certificateFile": cert, "keyFile": key}}}}
			if network == "grpc" {
				stream["grpcSettings"] = map[string]any{"serviceName": "vpn"}
			} else if network != "tcp" {
				stream[network+"Settings"] = map[string]any{"path": "/vpn"}
			}
			encoded, _ := json.Marshal(stream)
			seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none"}`, string(encoded))
			cfg, err := NativeCore(CoreTypeHiddify).GetConfig()
			if err != nil {
				t.Fatal(err)
			}
			checkGeneratedHiddifyConfig(t, cfg)
		})
	}
}

func TestHiddifyFullConfigNativeRoutingDNSAndXHTTPOutbound(t *testing.T) {
	setupBulkDB(t)
	seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none"}`, `{"network":"tcp"}`)
	path := filepath.Join(t.TempDir(), "domains.json")
	if err := os.WriteFile(path, []byte(`{"version":3,"rules":[{"domain_suffix":["example.com"]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	patch := fmt.Sprintf(`{"dns":{"servers":[{"type":"udp","tag":"remote","server":"1.1.1.1"}],"final":"remote","rules":[{"domain_suffix":["example.net"],"server":"remote"}]},"outbounds":[{"type":"vless","tag":"proxy","server":"example.com","server_port":443,"uuid":"11111111-1111-4111-8111-111111111111","tls":{"enabled":true,"server_name":"example.com"},"transport":{"type":"xhttp","path":"/vpn","mode":"stream-up","headers":{"detour":"opaque-header-value"}}},{"type":"direct","tag":"direct"},{"type":"selector","tag":"select","outbounds":["proxy","direct"],"default":"proxy"}],"route":{"final":"select","default_domain_resolver":"remote","rule_set":[{"type":"local","tag":"domains","format":"source","path":%q}],"rules":[{"rule_set":["domains"],"outbound":"direct"}]}}`, path)
	if err := singBoxSettingService.SetSingBoxConfigTemplate(patch); err != nil {
		t.Fatal(err)
	}
	cfg, err := NativeCore(CoreTypeHiddify).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	data := checkGeneratedHiddifyConfig(t, cfg)
	if !strings.Contains(string(data), `"type": "xhttp"`) || cfg.Route["final"] != "select" {
		t.Fatal("native config sections lost")
	}
	if _, err := NativeCore(CoreTypeSingBox).GetConfig(); err == nil {
		t.Fatal("legacy sing-box accepts native Hiddify extension")
	}
}

func TestHiddifySnapshotCanonicalUsersAndNoDatabaseWrites(t *testing.T) {
	setupBulkDB(t)
	ib := seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none","clients":[{"email":"stale","id":"22222222-2222-4222-8222-222222222222","enable":true}]}`, `{"network":"tcp"}`)
	db := database.GetDB()
	if err := db.Create(&coremodel.ClientTraffic{Id: 14825, InboundId: ib.Id, Email: "alice", Enable: false, Up: 123, Down: 456, Total: 1000}).Error; err != nil {
		t.Fatal(err)
	}
	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"inbounds":[{"type":"vless","tag":"managed-vless","listen_port":19443,"users":[{"name":"stale","uuid":"22222222-2222-4222-8222-222222222222"}]}]}`); err != nil {
		t.Fatal(err)
	}
	var before coremodel.ClientTraffic
	if err := db.First(&before, 14825).Error; err != nil {
		t.Fatal(err)
	}
	cfg, err := NativeCore(CoreTypeHiddify).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	data := checkGeneratedHiddifyConfig(t, cfg)
	if len(cfg.Inbounds) != 0 || strings.Contains(string(data), "stale") || strings.Contains(string(data), `"alice"`) {
		t.Fatalf("disabled managed user restored: %s", data)
	}
	var after coremodel.ClientTraffic
	if err := db.First(&after, 14825).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("generation changed persisted counters")
	}
	var stored model.Inbound
	if err := db.First(&stored, ib.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Settings != ib.Settings {
		t.Fatal("generation changed stored credentials")
	}
	if err := db.Model(&coremodel.ClientTraffic{}).Where("id = ?", 14825).Update("enable", true).Error; err != nil {
		t.Fatal(err)
	}
	cfg, err = NativeCore(CoreTypeHiddify).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	data = checkGeneratedHiddifyConfig(t, cfg)
	if len(cfg.Inbounds) != 1 || !strings.Contains(string(data), `"alice"`) || strings.Contains(string(data), "stale") {
		t.Fatal("canonical credentials not restored")
	}
}

func TestHiddifyRejectsBrokenTemplateBeforeConfigPublication(t *testing.T) {
	for _, patch := range []string{`{"future_server_option":true}`, `{"route":{"final":"lost"}}`, `null`, `{"dns":{"servers":[{"type":"local","tag":"local"}],"final":"lost"}}`} {
		t.Run(patch, func(t *testing.T) {
			setupBulkDB(t)
			t.Setenv("XUI_BIN_FOLDER", t.TempDir())
			if err := singBoxSettingService.SetSingBoxConfigTemplate(patch); err != nil {
				t.Fatal(err)
			}
			path := NativeCore(CoreTypeHiddify).ProcessConfigPath()
			old := []byte(`{"previous":"valid-config"}`)
			if err := os.WriteFile(path, old, 0600); err != nil {
				t.Fatal(err)
			}
			if err := NativeCore(CoreTypeHiddify).WriteConfig(); err == nil {
				t.Fatal("broken configuration published")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != string(old) {
				t.Fatal("previous config was replaced")
			}
		})
	}
}

func TestHiddifyGenerationUsesRequestedCoreForSubscriptions(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		t.Run(fmt.Sprint(automatic), func(t *testing.T) {
			setupBulkDB(t)
			// Preview Hiddify while Xray is still selected. The subscription must
			// be filtered against the requested generator, not the running engine.
			if err := singBoxSettingService.SetCoreType(CoreTypeXray); err != nil {
				t.Fatal(err)
			}
			members := `[{"protocol":"vless","tag":"subscription-xhttp","settings":{"address":"example.com","port":443,"id":"11111111-1111-4111-8111-111111111111","encryption":"none"},"streamSettings":{"network":"xhttp","xhttpSettings":{"path":"/subscription","mode":"stream-up"}}}]`
			sub := &model.OutboundSubscription{Enabled: true, Prepend: true, AutoBalance: automatic, SelectedTag: "subscription-xhttp", LastFetchedOutbounds: members}
			if err := database.GetDB().Create(sub).Error; err != nil {
				t.Fatal(err)
			}
			cfg, err := NativeCore(CoreTypeHiddify).GetConfig()
			if err != nil {
				t.Fatal(err)
			}
			data := checkGeneratedHiddifyConfig(t, cfg)
			if !strings.Contains(string(data), `"tag": "subscription-xhttp"`) {
				t.Fatal("Hiddify profile filtered by selected Xray")
			}
			if automatic && !strings.Contains(string(data), `"tag": "`+autoOutboundTag(sub.Id)+`"`) {
				t.Fatal("automatic selector missing")
			}
			var stored model.OutboundSubscription
			if err := database.GetDB().First(&stored, sub.Id).Error; err != nil {
				t.Fatal(err)
			}
			if stored.LastFetchedOutbounds != members {
				t.Fatal("generation changed source subscription")
			}
		})
	}
}

func TestHiddifyLegacyDNSAndRoutingGenerateFullConfig(t *testing.T) {
	setupBulkDB(t)
	if err := singBoxSettingService.SetXrayConfigTemplate(`{"dns":{"servers":["1.1.1.1"]},"routing":{"domainStrategy":"AsIs","rules":[{"type":"field","domain":["domain:example.com"],"outboundTag":"direct"},{"type":"field","ip":["geoip:private"],"outboundTag":"blocked"}]},"outbounds":[{"protocol":"freedom","tag":"direct","settings":{}},{"protocol":"blackhole","tag":"blocked","settings":{}}]}`); err != nil {
		t.Fatal(err)
	}
	cfg, err := NativeCore(CoreTypeHiddify).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	data := checkGeneratedHiddifyConfig(t, cfg)
	for _, field := range []string{`"server": "1.1.1.1"`, `"domain_suffix"`, `"ip_is_private"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("translated section missing %s", field)
		}
	}
}

func TestHiddifyMalformedPanelTemplateFailsWithoutPanic(t *testing.T) {
	for _, template := range []string{`null`, `{"dns":"wrong"}`, `{"routing":[]}`, `{"outbounds":"wrong"}`, `{"outbounds":[42]}`} {
		t.Run(template, func(t *testing.T) {
			setupBulkDB(t)
			if err := singBoxSettingService.SetXrayConfigTemplate(template); err != nil {
				t.Fatal(err)
			}
			if _, err := NativeCore(CoreTypeHiddify).GetConfig(); err == nil {
				t.Fatal("malformed template silently discarded")
			}
		})
	}
}
