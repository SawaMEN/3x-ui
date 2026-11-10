package service

import (
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

// certificateFirewallRules makes explicit port requirements part of the
// managed firewall's desired state. Normal web listeners remain open while serving. Extra challenge
// ports are leased for the duration of renewal; DNS-01 needs none.
// Never open a firewall port on the controlling panel for shared remote SNI.
func certificateFirewallRules(
	cfg domainfront.DomainFrontConfig,
	settings *entity.AllSetting,
	states []domainACME,
	telemt *TelemtWebProxyState,
) []FirewallRule {
	rules := []FirewallRule{}
	seen := map[int]bool{}
	add := func(port int, source, label string) {
		if port < 1 || port > 65535 || seen[port] {
			return
		}
		seen[port] = true
		rules = append(rules, FirewallRule{Port: port, Protocol: "tcp", Source: source, Label: label})
	}
	localGateway := false
	if cfg.Enabled {
		for _, domain := range cfg.Domains {
			if domain.Enabled && domain.Mode != "shared_sni" {
				localGateway = true
				break
			}
		}
	}
	if localGateway {
		// These are serving ports as well as CertMagic's local ACME-01 ports;
		// ensure the firewall reconciler never closes either unexpectedly.
		// HTTP is allowed only during renewal, unless another administrator
		// rule already permits redirects. HTTPS remains open for normal service.
		add(cfg.HTTPSPort, "domain-gateway", "Domain HTTPS")
	}
	if telemt != nil && telemt.Enabled {
		add(443, "telemt", "Telemt WEB HTTPS")
	}
	// External ACME challenge ports are opened only by a scoped lease.
	return rules
}
