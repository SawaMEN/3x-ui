package sub

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestBuildSeparatedSingBoxSubscription_MultipleProxies(t *testing.T) {
	proxies := []map[string]any{
		{"type": "vless", "tag": "vless-user", "server": "vless.example.com", "server_port": 443},
		{"type": "trojan", "tag": "trojan-user", "server": "trojan.example.com", "server_port": 8443},
	}

	got, err := buildSeparatedSingBoxSubscription(nil, proxies, false)
	if err != nil {
		t.Fatalf("buildSeparatedSingBoxSubscription: %v", err)
	}

	var docs []map[string]any
	if err := json.Unmarshal([]byte(got), &docs); err != nil {
		t.Fatalf("subscription is not a JSON array: %v\n%s", err, got)
	}
	if len(docs) != 2 {
		t.Fatalf("profile count = %d, want 2: %s", len(docs), got)
	}

	wantTypes := []string{"vless", "trojan"}
	for i, doc := range docs {
		outs, ok := doc["outbounds"].([]any)
		if !ok {
			t.Fatalf("profile %d outbounds = %#v, want array", i, doc["outbounds"])
		}
		if len(outs) != 3 {
			t.Fatalf("profile %d outbound count = %d, want 3", i, len(outs))
		}
		first, ok := outs[0].(map[string]any)
		if !ok {
			t.Fatalf("profile %d first outbound = %#v", i, outs[0])
		}
		if first["type"] != wantTypes[i] {
			t.Fatalf("profile %d type = %v, want %s", i, first["type"], wantTypes[i])
		}
		if _, ok := first["server"]; !ok {
			t.Fatalf("profile %d proxy missing server: %#v", i, first)
		}
		for _, raw := range outs[1:] {
			ob, _ := raw.(map[string]any)
			if ob["type"] == "selector" || ob["type"] == "urltest" {
				t.Fatalf("profile %d unexpectedly contains merged selector/urltest outbound: %#v", i, ob)
			}
		}
	}
}

func TestBuildSeparatedSingBoxSubscription_AlwaysReturnArrayForSingleProxy(t *testing.T) {
	got, err := buildSeparatedSingBoxSubscription(nil, []map[string]any{
		{"type": "vless", "tag": "vless-user", "server": "example.com", "server_port": 443},
	}, true)
	if err != nil {
		t.Fatalf("buildSeparatedSingBoxSubscription: %v", err)
	}

	var docs []map[string]any
	if err := json.Unmarshal([]byte(got), &docs); err != nil {
		t.Fatalf("subscription is not a JSON array: %v\n%s", err, got)
	}
	if len(docs) != 1 {
		t.Fatalf("profile count = %d, want 1", len(docs))
	}
}


func TestBuildSeparatedSingBoxSubscriptionFailsClosedOnRoutingTranslation(t *testing.T) {
	template := map[string]any{
		"routing": map[string]any{
			"rules": []any{
				map[string]any{
					"network":     "icmp",
					"outboundTag": "direct",
				},
			},
		},
	}
	_, err := buildSeparatedSingBoxSubscription(template, []map[string]any{
		{"type": "vless", "tag": "vless-user", "server": "example.com", "server_port": 443},
	}, false)
	if !errors.Is(err, errSubscriptionFormatUnsupported) {
		t.Fatalf("error = %v, want errSubscriptionFormatUnsupported", err)
	}
}


func TestGenNativeNaivePreservesNativeType(t *testing.T) {
	service := &SubJsonService{}
	inbound := &model.Inbound{
		Protocol: model.NaiveProxy,
		Listen:   "naive.example.com",
		Port:     443,
		Settings: `{"network":"tcp","tls":{"serverName":"naive.example.com","certificatePath":"/cert.pem","keyPath":"/key.pem"},"clients":[{"email":"user","password":"secret"}]}`,
		StreamSettings: `{"security":"tls"}`,
	}
	subReq := &SubService{}
	raw := service.genNativeNaive(subReq, inbound, model.Client{Email: "user", Password: "secret"}, nil)
	if raw == nil {
		t.Fatal("genNativeNaive returned nil")
	}
	got := raw
	if got["type"] != "naive" {
		t.Fatalf("type = %v, want naive; config=%#v", got["type"], got)
	}
	if got["server_port"] != float64(443) {
		t.Fatalf("server_port = %v, want 443", got["server_port"])
	}
	if got["username"] != "user" || got["password"] != "secret" {
		t.Fatalf("credentials = (%v, %v), want user/secret", got["username"], got["password"])
	}
	tls, ok := got["tls"].(map[string]any)
	if !ok || tls["enabled"] != true {
		t.Fatalf("tls = %#v, want enabled=true", got["tls"])
	}
}

func TestGenNativeTUICPreservesClientSettings(t *testing.T) {
	svc := &SubJsonService{}
	inbound := &model.Inbound{
		Protocol: model.TUIC,
		Listen: "tuic.example.com",
		Port: 443,
		Settings: `{"certificate":"/cert.pem","private_key":"/key.pem","congestion_control":"bbr","udp_relay_mode":"quic","zero_rtt_handshake":true,"clients":[{"uuid":"11111111-2222-3333-4444-555555555555","password":"secret","email":"user","enable":true}]}`,
		StreamSettings: `{"security":"tls","tlsSettings":{"serverName":"tuic.example.com","alpn":["h3"]}}`,
	}
	raw := svc.genNativeTUIC(inbound, unmarshalStreamSettings(inbound.StreamSettings), model.Client{ID:"11111111-2222-3333-4444-555555555555",Password:"secret",Email:"user"})
	if raw == nil {
		t.Fatal("genNativeTUIC returned nil")
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal native TUIC: %v", err)
	}
	for key, want := range map[string]any{
		"congestion_control": "bbr",
		"udp_relay_mode":     "quic",
		"zero_rtt_handshake":  true,
	} {
		if got[key] != want {
			t.Fatalf("%s = %v, want %v; config=%#v", key, got[key], want, got)
		}
	}
}
