package service

import (
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

// certificateFirewallRules makes explicit port requirements part of the
// managed firewall's desired state. Standalone cron renewals cannot be
// predicted precisely, so known challenge ports stay allowed while the
// referenced certificate remains in use. DNS-01 does not require inbound ports.
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
		add(cfg.HTTPPort, "domain-gateway", "Domain HTTP / ACME HTTP-01")
		add(cfg.HTTPSPort, "domain-gateway", "Domain HTTPS / TLS-ALPN-01")
	}
	referenced := []string{}
	if settings != nil {
		if settings.WebCertFile != "" && settings.WebKeyFile != "" {
			referenced = append(referenced, settings.WebCertFile)
		}
		if settings.SubCertFile != "" && settings.SubKeyFile != "" {
			referenced = append(referenced, settings.SubCertFile)
		}
	}
	for _, domain := range cfg.Domains {
		if domain.Enabled && domain.Mode != "shared_sni" && domain.TLS == "manual" && domain.CertFile != "" && domain.KeyFile != "" {
			referenced = append(referenced, domain.CertFile)
		}
	}
	if telemt != nil && telemt.Enabled {
		add(443, "telemt", "Telemt WEB HTTPS")
		if telemt.CertFile != "" && telemt.KeyFile != "" {
			referenced = append(referenced, telemt.CertFile)
		}
	}
	for _, certFile := range referenced {
		challenge := ""
		for _, state := range states {
			if certFile == state.values["Le_RealFullChainPath"] {
				challenge = acmeSHChallenge(state.values["Le_Webroot"])
				break
			}
		}
		if challenge == "" {
			challenge = certbotChallenge(certFile)
		}
		for _, port := range challengeFirewallPorts(challenge, 80, 443) {
			add(port, "certificate-renewal", "External certificate renewal "+challenge)
		}
	}
	return rules
}
