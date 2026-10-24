package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

func TestDomainGatewayRoutesRejectConflictingPathsAndRemoteNodes(t *testing.T) {
	setupConflictDB(t)
	s := &SettingService{}
	if err := s.setString("subPath", "/sub/"); err != nil {
		t.Fatal(err)
	}
	inbound := model.Inbound{Enable: true, UserId: 1, Tag: "managed-ws", Protocol: model.VLESS, Port: 15001, StreamSettings: `{"network":"ws","security":"none","wsSettings":{"path":"/vpn?ed=2048"}}`, Settings: `{"clients":[]}`}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.AcceptTerms = true
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: "direct.example.com", Enabled: true, Mode: "direct", TLS: "auto", InboundIDs: []int{inbound.Id}}}
	backend, err := s.domainFrontBackend(cfg)
	if err != nil {
		t.Fatal(err)
	}
	route := backend.Routes["direct.example.com"][0]
	if route.Path != "/vpn" || route.Target != "http://127.0.0.1:15001" {
		t.Fatalf("route = %#v", route)
	}
	inbound.StreamSettings = `{"network":"ws","security":"none","wsSettings":{"path":"/sub/"}}`
	if err := database.GetDB().Save(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.domainFrontBackend(cfg); err == nil || !strings.Contains(err.Error(), "path conflicts") {
		t.Fatalf("reserved path error = %v", err)
	}
	node := 12
	inbound.NodeID = &node
	if _, err := domainInboundRoute(&inbound, "direct.example.com"); err == nil || !strings.Contains(err.Error(), "enabled and local") {
		t.Fatalf("remote inbound error = %v", err)
	}
}

func TestDomainGatewayCanonicalURLsAndHostRoles(t *testing.T) {
	setupConflictDB(t)
	s := &SettingService{}
	if err := s.setString("subPath", "/sub/"); err != nil {
		t.Fatal(err)
	}
	inbound := &model.Inbound{Id: 21, Enable: true}
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.SubscriptionDomain = "only.example.com"
	cfg.Domains = []domainfront.DomainFrontDomain{
		{Name: "only.example.com", Mode: "sub_link_only", Enabled: true, TLS: "auto"},
		{Name: "cdn.example.com", Mode: "cdn", Enabled: true, TLS: "auto", InboundIDs: []int{21}},
		{Name: "off.example.com", Mode: "direct", Enabled: false, TLS: "auto", InboundIDs: []int{21}},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.setString("domainFrontConfig", string(raw)); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetSubURI(); err != nil || got != "https://only.example.com/sub/" {
		t.Fatalf("subscription URL = %q, %v", got, err)
	}
	hosts, err := s.DomainFrontHosts([]*model.Inbound{inbound})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts[21]) != 1 || hosts[21][0].Address != "cdn.example.com" || hosts[21][0].Security != "tls" || hosts[21][0].Port != 443 {
		t.Fatalf("managed hosts = %#v", hosts[21])
	}
	allowed, err := s.GetSubscriptionAllowedHosts("origin.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed["only.example.com"] || !allowed["cdn.example.com"] || allowed["off.example.com"] {
		t.Fatalf("allowed subscription hosts = %#v", allowed)
	}
}
