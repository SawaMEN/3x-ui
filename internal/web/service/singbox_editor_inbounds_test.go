package service

import (
	"reflect"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestEditorRoutingInboundsIncludesManagedEndpoints(t *testing.T) {
	remote := 9
	cfg := &singbox.Config{
		Inbounds: []map[string]any{{"tag": "local-vless"}, {"tag": "local-vless"}},
		Endpoints: []map[string]any{
			{"type": "wireguard", "tag": "local-wg"},
			{"type": "masque", "tag": "local-masque"},
			{"type": "wireguard", "tag": "local-vk"},
			{"type": "wireguard", "tag": "disabled"},
			{"type": "wireguard", "tag": "remote"},
			{"type": "wireguard", "tag": "external"},
		},
	}
	rows := []*model.Inbound{
		{Tag: "local-wg", Protocol: model.WireGuard, Enable: true},
		{Tag: "local-masque", Protocol: model.MASQUE, Enable: true},
		{Tag: "local-vk", Protocol: model.VKTurnProxy, Enable: true},
		{Tag: "disabled", Protocol: model.WireGuard, Enable: false},
		{Tag: "remote", Protocol: model.WireGuard, Enable: true, NodeID: &remote},
	}
	tags, editable := editorRoutingInbounds(cfg, rows)
	if want := []string{"local-vless", "local-wg", "local-masque", "local-vk"}; !reflect.DeepEqual(tags, want) {
		t.Fatalf("routing tags = %v, want %v", tags, want)
	}
	if want := []string{"disabled", "remote", "external"}; len(editable) != len(want) {
		t.Fatalf("editable endpoints = %v, want %v", editable, want)
	} else {
		for i, tag := range want {
			if editable[i]["tag"] != tag {
				t.Fatalf("editable endpoint %d = %v, want %q", i, editable[i], tag)
			}
		}
	}
}
