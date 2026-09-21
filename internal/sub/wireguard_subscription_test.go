package sub

import (
	"net/url"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestGenWireguardLinkPreservesAllAllowedIPs(t *testing.T) {
	inbound := &model.Inbound{
		Id:       1,
		Protocol: model.WireGuard,
		Port:     51820,
		Listen:   "0.0.0.0",
		Settings: `{"secretKey":""}`,
	}
	client := model.Client{
		Email:      "wg@example.com",
		PrivateKey: "client-private-key",
		AllowedIPs: []string{"10.0.0.2/32", "fd00::2/128"},
	}

	s := &SubService{
		address:             "vpn.example.com",
		clientsByInbound:    map[int]map[string]model.Client{1: {client.Email: client}},
		fullyPrimedInbounds: map[int]bool{1: true},
		settingsByInbound:   map[int]map[string]any{},
	}

	link := s.genWireguardLink(inbound, client.Email)
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse link: %v", err)
	}
	if got := parsed.Query().Get("address"); got != strings.Join(client.AllowedIPs, ",") {
		t.Fatalf("address = %q, want %q", got, strings.Join(client.AllowedIPs, ","))
	}
}

func TestGenWireguardLinkFormatsIPv6Endpoint(t *testing.T) {
	inbound := &model.Inbound{
		Id:       2,
		Protocol: model.WireGuard,
		Port:     51820,
		Listen:   "::",
		Settings: `{"secretKey":""}`,
	}
	client := model.Client{
		Email:      "wg6@example.com",
		PrivateKey: "client-private-key",
		AllowedIPs: []string{"fd00::2/128"},
	}

	s := &SubService{
		address:             "2001:db8::10",
		clientsByInbound:    map[int]map[string]model.Client{2: {client.Email: client}},
		fullyPrimedInbounds: map[int]bool{2: true},
		settingsByInbound:   map[int]map[string]any{},
	}

	link := s.genWireguardLink(inbound, client.Email)
	if !strings.Contains(link, "@[2001:db8::10]:51820") {
		t.Fatalf("link = %q, missing bracketed IPv6 endpoint", link)
	}
}
