package sub

import (
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"testing"
)

func TestNativeMieruOutbounds(t *testing.T) {
	svc := NewSubService("")
	svc.address = "mieru.example.com"
	inbound := &model.Inbound{Protocol: model.Mieru, Port: 20000, Settings: `{"tcpPorts":["20001","20002-20003"],"udpPorts":["21001"],"multiplexing":"MULTIPLEXING_HIGH","handshakeMode":"HANDSHAKE_NO_WAIT"}`}
	client := model.Client{Email: "user@example.com", Password: "secret"}
	outs := nativeMieruOutbounds(svc, inbound, client)
	if len(outs) != 1 {
		t.Fatalf("outbounds = %d, want 1: %#v", len(outs), outs)
	}
	out := outs[0]
	if out["type"] != "mieru" || out["server"] != "mieru.example.com" || out["username"] != client.Email || out["password"] != client.Password {
		t.Fatalf("unexpected Mieru outbound: %#v", out)
	}
	bindings, ok := out["portBindings"].([]any)
	if !ok || len(bindings) != 3 {
		t.Fatalf("portBindings = %#v, want 3 entries", out["portBindings"])
	}
	if bindings[1].(map[string]any)["portRange"] != "20002-20003" {
		t.Fatalf("portRange not preserved: %#v", bindings[1])
	}
	if out["multiplexing"] != "MULTIPLEXING_HIGH" || out["handshake_mode"] != "HANDSHAKE_NO_WAIT" {
		t.Fatalf("Mieru options lost: %#v", out)
	}
}

func TestNativeMieruOutboundsExternalProxyUsesPublicPort(t *testing.T) {
	svc := NewSubService("")
	svc.address = "origin.example.com"
	inbound := &model.Inbound{Protocol: model.Mieru, Port: 20000, Settings: `{"tcpPorts":["20001","20002"],"udpPorts":["21001"]}`, StreamSettings: `{"externalProxy":[{"dest":"edge.example.com","port":443}]}`}
	client := model.Client{Email: "user", Password: "secret"}
	outs := nativeMieruOutbounds(svc, inbound, client)
	if len(outs) != 1 || outs[0]["server"] != "edge.example.com" {
		t.Fatalf("unexpected external Mieru outbound: %#v", outs)
	}
	bindings, ok := outs[0]["portBindings"].([]any)
	if !ok || len(bindings) != 2 {
		t.Fatalf("public portBindings = %#v, want TCP+UDP", outs[0]["portBindings"])
	}
	for _, raw := range bindings {
		if raw.(map[string]any)["port"] != 443 {
			t.Fatalf("binding leaked private port: %#v", raw)
		}
	}
}
