package service

import (
	"reflect"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

func firewallPorts(rules []FirewallRule) []int {
	ports := []int{}
	for _, rule := range rules {
		ports = append(ports, rule.Port)
	}
	return ports
}

func TestCertificateFirewallRulesGatewayAndRemote(t *testing.T) {
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.HTTPPort, cfg.HTTPSPort = 8080, 8443
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: "vpn.example.org", Enabled: true, Mode: "direct", TLS: "certmagic"}}
	if got := firewallPorts(certificateFirewallRules(cfg, nil, nil, nil)); !reflect.DeepEqual(got, []int{8080, 8443}) {
		t.Fatalf("gateway ports = %v", got)
	}
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: "remote.example.org", Enabled: true, Mode: "shared_sni", TLS: "auto"}}
	if got := firewallPorts(certificateFirewallRules(cfg, nil, nil, nil)); len(got) > 0 {
		t.Fatalf("remote SNI must not open local ports: %v", got)
	}
}

func TestCertificateFirewallRulesExternalWebrootAndDNS(t *testing.T) {
	settings := &entity.AllSetting{WebCertFile: "/opt/web/fullchain.pem", WebKeyFile: "/opt/web/key.pem"}
	states := []domainACME{{values: map[string]string{
		"Le_RealFullChainPath": settings.WebCertFile, "Le_Webroot": "no",
	}}}
	cfg := domainfront.DefaultConfig()
	if got := firewallPorts(certificateFirewallRules(cfg, settings, states, nil)); !reflect.DeepEqual(got, []int{80}) {
		t.Fatalf("acme.sh standalone must allow TCP 80: %v", got)
	}
	states[0].values["Le_Webroot"] = "dns_cf"
	if got := firewallPorts(certificateFirewallRules(cfg, settings, states, nil)); len(got) != 0 {
		t.Fatalf("DNS-01 must not open any port: %v", got)
	}
	states[0].values["Le_Webroot"] = "alpn"
	if got := firewallPorts(certificateFirewallRules(cfg, settings, states, nil)); !reflect.DeepEqual(got, []int{443}) {
		t.Fatalf("TLS-ALPN-01 must allow TCP 443: %v", got)
	}
}
