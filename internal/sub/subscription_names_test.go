package sub

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	yaml "github.com/goccy/go-yaml"

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

func TestClientConnectionNameOverridesAllSubscriptionFormats(t *testing.T) {
	for _, protocol := range []model.Protocol{model.VMESS, model.VLESS, model.Trojan, model.Hysteria, model.TUIC, model.AnyTLS, model.NaiveProxy, model.Snell, model.Mieru} {
		t.Run(string(protocol), func(t *testing.T) {
			initSubDB(t)
			db := database.GetDB()
			name := "Мой VPN / #1"
			client := &model.ClientRecord{Email: "named@example.com", SubID: "named", UUID: "11111111-2222-4333-8444-555555555555", Password: "secret-password-123456789012345678", Auth: "secret-auth", Enable: true, ConnectionName: &name}
			if err := db.Create(client).Error; err != nil {
				t.Fatal(err)
			}
			settings := fmt.Sprintf(`{"version":2,"clients":[{"id":%q,"email":%q,"password":%q,"auth":%q,"enable":true}],"protocols":["TCP"]}`, client.UUID, client.Email, client.Password, client.Auth)
			if protocol == model.Snell {
				settings = fmt.Sprintf(`{"version":5,"psk":"server-psk-secret","clients":[{"email":%q,"password":%q,"enable":true}]}`, client.Email, client.Password)
			}
			inbound := &model.Inbound{UserId: 1, Tag: string(protocol), Protocol: protocol, Enable: true, Remark: "Protocol-specific name", Port: 8443, Settings: settings, StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"vpn.example.com"}}`}
			if err := db.Create(inbound).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: inbound.Id}).Error; err != nil {
				t.Fatal(err)
			}
			for _, withHost := range []bool{false, true} {
				if withHost {
					seedHost(t, &model.Host{InboundId: inbound.Id, Remark: "Host-specific name", Address: "vpn.example.com", Port: 443})
				}
				for _, template := range []string{"", "{{PROTOCOL}} {{INBOUND}} / {{HOST}}"} {
					svc := NewSubJsonService("", "", "", "", NewSubService(template))
					body, _, err := svc.GetHiddifySingBoxJson(client.SubID, "sub.example.com")
					if err != nil {
						t.Fatal(err)
					}
					var cfg struct {
						Outbounds []map[string]any `json:"outbounds"`
					}
					if err = json.Unmarshal([]byte(body), &cfg); err != nil {
						t.Fatal(err)
					}
					if len(cfg.Outbounds) != 1 || cfg.Outbounds[0]["tag"] != name {
						t.Fatalf("host=%v template=%q: %s", withHost, template, body)
					}
					// Xray's full profile carries the common name in remarks.
					if protocol == model.VMESS || protocol == model.VLESS || protocol == model.Trojan {
						xray, _, err := svc.GetJson(client.SubID, "sub.example.com", false)
						if err != nil {
							t.Fatal(err)
						}
						var profile map[string]any
						if err = json.Unmarshal([]byte(xray), &profile); err != nil {
							t.Fatal(err)
						}
						if profile["remarks"] != name {
							t.Fatalf("Xray: %s", xray)
						}
					}
				}
			}
		})
	}
}

func TestClientConnectionNameRawLinksAndUniqueProfileNames(t *testing.T) {
	initSubDB(t)
	db := database.GetDB()
	name := "VPN / #1"
	client := &model.ClientRecord{Email: "raw-name@example.com", SubID: "raw-names", UUID: "11111111-2222-4333-8444-555555555555", Password: "test-secret", Enable: true, ConnectionName: &name}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	for i, protocol := range []model.Protocol{model.VLESS, model.Trojan} {
		settings := fmt.Sprintf(`{"clients":[{"id":%q,"email":%q,"password":%q,"enable":true}]}`, client.UUID, client.Email, client.Password)
		inbound := &model.Inbound{UserId: 1, Tag: string(protocol), Protocol: protocol, Enable: true, Remark: "Different " + string(protocol), Port: 8443 + i, Settings: settings, StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"vpn.example.com"}}`}
		if err := db.Create(inbound).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: inbound.Id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewSubService("{{PROTOCOL}} {{INBOUND}}")
	links, _, _, _, err := svc.GetSubs(client.SubID, "sub.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("links: %v", links)
	}
	for _, link := range links {
		parsed, err := url.Parse(link)
		if err != nil || parsed.Fragment != name {
			t.Fatalf("raw name: %q %v", link, err)
		}
	}
	clash, _, err := NewSubClashService(false, "", svc).GetClash(client.SubID, "sub.example.com")
	if err != nil {
		t.Fatal(err)
	}
	var profile struct {
		Proxies []struct {
			Name string `yaml:"name"`
		} `yaml:"proxies"`
	}
	if err = yaml.Unmarshal([]byte(clash), &profile); err != nil {
		t.Fatal(err)
	}
	if len(profile.Proxies) != 2 || profile.Proxies[0].Name != name || profile.Proxies[1].Name != name+"-2" {
		t.Fatalf("Clash names: %+v", profile.Proxies)
	}
	body, _, err := NewSubJsonService("", "", "", "", svc).GetHiddifySingBoxJson(client.SubID, "sub.example.com")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err = json.Unmarshal([]byte(body), &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Outbounds) != 2 || config.Outbounds[0]["tag"] != name || config.Outbounds[1]["tag"] != name+"-2" {
		t.Fatalf("sing-box names: %s", body)
	}
}

func TestClientConnectionNameAppliesToExternalLinks(t *testing.T) {
	initSubDB(t)
	db := database.GetDB()
	name := "Единый VPN"
	row := model.ClientRecord{Email: "external-name", SubID: "external-name-sub", Enable: true, ConnectionName: &name}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	ext := model.ClientExternalLink{ClientId: row.Id, Kind: model.ExternalLinkKindLink, Value: "trojan://password@vpn.example.com:443#different", Remark: "Separate remark"}
	if err := db.Create(&ext).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewSubService("")
	links, _, _, _, err := svc.GetSubs(row.SubID, "sub.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("links: %v", links)
	}
	parsed, err := url.Parse(links[0])
	if err != nil || parsed.Fragment != name {
		t.Fatalf("link name: %s %v", links[0], err)
	}
}
