package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/amneziawg"
	"github.com/SawaMEN/3x-ui/v3/internal/amneziawgnet"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/protocolserver"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/xray/geodata"
)

// generateNativeConfig is the shared compatibility translator. Hiddify supplies
// a database snapshot for the owner runtime.
func (s *SingBoxService) generateNativeConfig(snapshot *nativeConfigSnapshot, editorOnly ...bool) (*singbox.Config, error) {
	var inbounds []*model.Inbound
	var err error
	if snapshot != nil {
		inbounds = snapshot.inbounds
	} else {
		inbounds, err = singBoxInboundService.GetAllInbounds()
		if err != nil {
			return nil, err
		}
	}

	cfg := singbox.NewConfig()
	// Generated outbounds come from the Xray template. Keep NewConfig's
	// defaults only for its standalone editor fallback.
	cfg.Outbounds = nil

	managedOutboundTags := map[string]bool{}
	subscriptionTags := map[string]bool{}
	dnsOutboundTags := make(map[string]struct{})
	var template string
	var templateErr error
	if snapshot != nil {
		template = snapshot.setting("xrayTemplateConfig")
	} else {
		template, templateErr = singBoxSettingService.GetXrayConfigTemplate()
	}
	if templateErr != nil {
		return nil, templateErr
	}
	var routingLists []model.RoutingList
	if snapshot != nil {
		routingLists = snapshot.routingLists
	} else {
		routingLists, err = loadRoutingLists(template)
		if err != nil {
			return nil, err
		}
	}
	template, err = materializeRoutingLists(template, routingLists)
	if err != nil {
		return nil, err
	}
	{
		var xrayCfg map[string]any
		if err := json.Unmarshal([]byte(template), &xrayCfg); err != nil {
			return nil, fmt.Errorf("parse Xray template: %w", err)
		}
		if xrayCfg == nil {
			return nil, fmt.Errorf("panel template must be a JSON object")
		}
		{
			if err := validatePanelConfigSections(xrayCfg); err != nil {
				return nil, err
			}
		}
		{
			var prepend, tail []any
			var err error
			if snapshot != nil {
				prepend, tail, err = splitActiveOutboundsForCore(snapshot.subscriptions, s.coreType())
			} else {
				prepend, tail, err = (&OutboundSubscriptionService{}).activeOutboundsSplit()
			}
			if err != nil {
				return nil, err
			}
			for _, list := range [][]any{prepend, tail} {
				for _, item := range list {
					if row, ok := item.(map[string]any); ok {
						if tag, _ := row["tag"].(string); tag != "" {
							subscriptionTags[tag] = true
						}
					}
				}
			}
			manual, _ := xrayCfg["outbounds"].([]any)
			xrayCfg["outbounds"] = append(append(prepend, manual...), tail...)
			if rawDNS, ok := xrayCfg["dns"].(map[string]any); ok && len(rawDNS) > 0 {
				rawRouting, _ := xrayCfg["routing"].(map[string]any)
				if err := singbox.ValidateXrayDNSRouting(rawDNS, rawRouting); err != nil {
					return nil, fmt.Errorf("sing-box DNS routing: %w", err)
				}
				dns, err := singbox.TranslateXrayDNS(rawDNS)
				if err != nil {
					return nil, fmt.Errorf("sing-box DNS template: %w", err)
				}
				if len(dns) > 0 {
					// sing-box 1.14 requires a resolver for domain-based outbound
					// server addresses. Keep the system resolver available even when
					// the Xray DNS template replaces the generated default.
					servers, _ := dns["servers"].([]map[string]any)
					hasLocal := false
					for _, server := range servers {
						if tag, _ := server["tag"].(string); tag == "local" {
							hasLocal = true
							break
						}
					}
					if !hasLocal {
						servers = append(servers, map[string]any{"type": "local", "tag": "local"})
						dns["servers"] = servers
					}
					cfg.DNS = dns
				}
			}
			if rawRouting, ok := xrayCfg["routing"].(map[string]any); ok && len(rawRouting) > 0 {
				if route, err := singbox.TranslateXrayRoutingWithGeoData(rawRouting, geodata.NewStore(assetDir())); err != nil {
					return nil, err
				} else if len(route) > 0 {
					cfg.Route = route
				}
			}
			// sing-box 1.14 requires a resolver for domain-based outbound
			// server addresses. Apply this after translated routing so the
			// compatibility route cannot accidentally discard it.
			cfg.Route["default_domain_resolver"] = cfg.DNS["final"]
			if rawRouting, ok := xrayCfg["routing"].(map[string]any); ok {
				if strategy := singbox.TranslateXrayDomainStrategy(fmt.Sprint(rawRouting["domainStrategy"])); strategy != "" {
					cfg.Route["default_domain_resolver"] = map[string]any{
						"server":   cfg.DNS["final"],
						"strategy": strategy,
					}
				}
			}
			if rawBalancers, ok := xrayCfg["routing"].(map[string]any); ok {
				var available []map[string]any
				for _, value := range xrayCfg["outbounds"].([]any) {
					if outbound, ok := value.(map[string]any); ok {
						available = append(available, outbound)
					}
				}
				balancers, err := hiddify.TranslateBalancers(rawBalancers, xrayCfg, available...)
				if err != nil {
					return nil, err
				}
				cfg.Outbounds = append(cfg.Outbounds, balancers...)
			}
			if rawOutbounds, ok := xrayCfg["outbounds"].([]any); ok {
				if len(rawOutbounds) > 0 {
					if first, ok := rawOutbounds[0].(map[string]any); ok {
						if tag, ok := first["tag"].(string); ok && tag != "" {
							cfg.Route["final"] = tag
						}
					}
				}

				for i, raw := range rawOutbounds {
					ob, ok := raw.(map[string]any)
					if !ok {
						continue
					}
					if id, ok := automaticAliasID(ob); ok {
						var sub *model.OutboundSubscription
						var err error
						if snapshot != nil {
							sub, err = snapshot.subscription(id)
						} else {
							sub, err = (&OutboundSubscriptionService{}).Get(id)
						}
						if err != nil {
							return nil, err
						}
						var members []any
						if sub.LastFetchedOutbounds != "" {
							if err := json.Unmarshal([]byte(sub.LastFetchedOutbounds), &members); err != nil {
								return nil, err
							}
						}
						members, _, _ = filterSubscriptionOutboundsForCore(s.coreType()+" automatic selector", s.coreType(), members)
						cfg.Outbounds = append(cfg.Outbounds, automaticSingBoxSelector(sub, members))
						continue
					}
					if protocol, _ := ob["protocol"].(string); strings.EqualFold(strings.TrimSpace(protocol), "dns") {
						settings, _ := ob["settings"].(map[string]any)
						for _, key := range []string{"address", "port", "rewriteAddress", "rewritePort", "nonIPQuery"} {
							if value, present := settings[key]; present && fmt.Sprint(value) != "" && fmt.Sprint(value) != "0" && fmt.Sprint(value) != "skip" {
								return nil, fmt.Errorf("DNS outbound %v option %s cannot be preserved by hijack-dns", ob["tag"], key)
							}
						}
						if tag, _ := ob["tag"].(string); strings.TrimSpace(tag) != "" {
							dnsOutboundTags[tag] = struct{}{}
						}
						continue
					}
					if endpoint, handled, err := singbox.TranslatePanelEndpoint(ob); handled {
						if err != nil {
							return nil, err
						}
						cfg.Endpoints = append(cfg.Endpoints, endpoint)
						continue
					}
					if amneziawg.IsAmneziaWGOutbound(mustJSON(ob)) {
						if amneziawg.UsesNativeOutbound(mustJSON(ob)) {
							native, err := amneziawg.NativeOutbound(mustJSON(ob))
							if err != nil {
								return nil, err
							}
							endpoint, _, err := singbox.TranslatePanelEndpoint(native)
							if err != nil {
								return nil, err
							}
							cfg.Endpoints = append(cfg.Endpoints, endpoint)
							continue
						}
						bridged, bridgeOK := amneziawgnet.BuildSocksBridge(mustJSON(ob))
						if !bridgeOK {
							return nil, fmt.Errorf("amneziawg outbound %d: cannot create sing-box SOCKS bridge", i)
						}
						if err := json.Unmarshal(bridged, &ob); err != nil {
							return nil, fmt.Errorf("amneziawg outbound %d: invalid bridge: %w", i, err)
						}
					}
					if externalvpn.IsAdditionalOutbound(ob) {
						tag, _ := ob["tag"].(string)
						managedOutboundTags[tag] = true
						var bridge map[string]any
						var err error
						{
							bridge, err = externalvpn.PlanOutbound(ob)
							cfg.ManagedOutbounds = append(cfg.ManagedOutbounds, ob)
						}

						if err != nil {
							return nil, fmt.Errorf("outbound %q: %w", tag, err)
						}
						ob = bridge
					}
					translated, err := s.translateOutbound(ob)
					if err != nil {
						return nil, err
					}
					cfg.Outbounds = append(cfg.Outbounds, translated)
				}
			}
			if cfg.Route != nil {
				singbox.RewriteDNSOutboundRoutes(cfg.Route, dnsOutboundTags)
			}
		}
	}

	hasDirect, hasBlocked := false, false
	for _, outbound := range cfg.Outbounds {
		tag, _ := outbound["tag"].(string)
		switch tag {
		case "direct":
			hasDirect = true
		case "blocked":
			hasBlocked = true
		}
	}
	if !hasDirect {
		cfg.Outbounds = append(cfg.Outbounds, map[string]any{"type": "direct", "tag": "direct"})
	}
	if !hasBlocked {
		cfg.Outbounds = append(cfg.Outbounds, map[string]any{"type": "block", "tag": "blocked"})
	}

	inboundIDs := make([]int, 0, len(inbounds))
	for _, inbound := range inbounds {
		if inbound == nil || !inbound.Enable || inbound.NodeID != nil || isLocalSidecarInbound(inbound.Protocol) {
			continue
		}
		inboundIDs = append(inboundIDs, inbound.Id)
	}
	var clientsByInbound map[int][]model.Client
	if snapshot != nil {
		clientsByInbound = snapshot.clients
	} else {
		clientsByInbound, err = singBoxInboundService.clientService.ListForInbounds(nil, inboundIDs)
	}
	if err != nil {
		return nil, err
	}

	var unsupported []string
	var sniffRules []any
	for _, inbound := range inbounds {
		if inbound == nil || !inbound.Enable || inbound.NodeID != nil || isLocalSidecarInbound(inbound.Protocol) {
			continue
		}
		runtimeInbound := inbound
		if snapshot == nil {
			runtimeInbound, err = singBoxInboundService.buildInboundForNodePush(database.GetDB(), inbound)
		}
		if err != nil {
			return nil, err
		}
		rawBytes, err := json.Marshal(runtimeInbound)
		if err != nil {
			return nil, err
		}
		var raw map[string]any
		if err := json.Unmarshal(rawBytes, &raw); err != nil {
			return nil, err
		}

		// Xray treats an empty inbound listen address as all interfaces. The
		// sing-box default is loopback, which makes a successfully started
		// inbound unreachable from the network. Preserve Xray semantics.
		if listen, ok := raw["listen"].(string); !ok || strings.TrimSpace(listen) == "" {
			raw["listen"] = "0.0.0.0"
		}
		dbClients := clientsByInbound[inbound.Id]
		// Snell user keys belong to this inbound. Keep the runtime credentials
		// identical to the native subscription even when the global client has
		// a different password for another protocol.
		snellKeys := map[string]string{}
		if inbound.Protocol == model.Snell {
			var local struct {
				Clients []model.Client `json:"clients"`
			}
			if err := json.Unmarshal([]byte(inbound.Settings), &local); err != nil {
				return nil, err
			}
			for _, client := range local.Clients {
				if client.Enable {
					snellKeys[strings.ToLower(strings.TrimSpace(client.Email))] = client.Password
				}
			}
		}

		enableMap := make(map[string]bool, len(inbound.ClientStats))
		for _, stat := range inbound.ClientStats {
			enableMap[stat.Email] = stat.Enable
		}

		clients := make([]any, 0, len(dbClients))
		for _, client := range dbClients {
			if enabled, exists := enableMap[client.Email]; exists && !enabled {
				continue
			}
			if !client.Enable {
				continue
			}
			if (inbound.Protocol == model.NaiveProxy || inbound.Protocol == model.Hysteria) && protocolserver.Installed(inbound.Protocol) && client.LimitIP > 0 {
				reason := fmt.Sprintf("standalone %s cannot preserve client source-IP limits; use sing-box", inbound.Protocol)
				if s.strictProtocolServers == inbound.Protocol {
					return nil, fmt.Errorf("%s", reason)
				}
				if cfg.ProtocolServerFallbacks == nil {
					cfg.ProtocolServerFallbacks = map[string]string{}
				}
				cfg.ProtocolServerFallbacks[inbound.Tag] = reason
			}
			entry := map[string]any{"email": client.Email}
			if inbound.Protocol == model.MASQUE {
				entry["enable"] = true
			}
			switch inbound.Protocol {
			case model.VLESS:
				if client.Reverse != nil {
					entry["reverse"] = map[string]any{"tag": client.Reverse.Tag}
				}
				if client.ID != "" {
					entry["id"] = client.ID
				}
				if client.Flow != "" && !inbound.DisableFlow {
					entry["flow"] = client.Flow
				}
			case model.VMESS:
				if client.ID != "" {
					entry["id"] = client.ID
				}
				if client.Security != "" {
					entry["security"] = client.Security
				}
			case model.Snell:
				password, present := snellKeys[strings.ToLower(strings.TrimSpace(client.Email))]
				if !present {
					continue
				}
				entry["password"] = password
			case model.Trojan:
				if client.Password != "" {
					entry["password"] = client.Password
				}
				if client.Flow != "" && !inbound.DisableFlow {
					entry["flow"] = client.Flow
				}
			case model.Shadowsocks:
				if client.Password != "" {
					entry["password"] = client.Password
				}
			case model.Hysteria:
				if client.Auth != "" {
					entry["auth"] = client.Auth
				}
			case model.TUIC:
				if client.ID != "" {
					entry["uuid"] = client.ID
				}
				if client.Password != "" {
					entry["password"] = client.Password
				}
			case model.HTTP, model.Mixed, model.MASQUE, model.NaiveProxy, model.Mieru, model.AnyTLS, model.ShadowTLS:
				if client.Password != "" {
					entry["password"] = client.Password
				}
			}
			clients = append(clients, entry)
		}
		managedProxy := (inbound.Protocol == model.HTTP || inbound.Protocol == model.Mixed) && len(dbClients) > 0
		if (singBoxInboundRequiresUsers(inbound.Protocol) || managedProxy) && len(clients) == 0 {
			logger.Warningf("Skipping sing-box inbound %q (%s): no active users", inbound.Tag, inbound.Protocol)
			continue
		}
		if inbound.Protocol == model.TUIC {
			translated, err := singBoxTUICInbound(inbound, clients)
			if err != nil {
				unsupported = append(unsupported, fmt.Sprintf("%s: %v", inbound.Tag, err))
				continue
			}
			cfg.Inbounds = append(cfg.Inbounds, translated)
			continue
		}

		settings, _ := raw["settings"].(map[string]any)
		if settings == nil {
			settings = map[string]any{}
		}
		if managedProxy {
			// A stale accounts copy must not re-enable disabled managed users.
			delete(settings, "accounts")
		}
		settings["clients"] = clients
		var sniffRule map[string]any
		var sniffErr error
		{
			sniffRule, sniffErr = hiddify.TranslateSniffingRule(raw)
		}

		if sniffErr != nil {
			unsupported = append(unsupported, sniffErr.Error())
			continue
		}
		if sniffRule != nil {
			sniffRules = append(sniffRules, sniffRule)
		}

		// NaiveProxy is a native TLS protocol in sing-box. Keep the ordinary
		// inbound form simple by reusing the panel's HTTPS certificate/key when
		// the inbound does not explicitly provide its own pair.
		if inbound.Protocol == model.MASQUE || inbound.Protocol == model.NaiveProxy || inbound.Protocol == model.AnyTLS {
			tls, _ := settings["tls"].(map[string]any)
			if tls == nil {
				tls = map[string]any{}
			}
			tls["enabled"] = true
			certPath, _ := tls["certificatePath"].(string)
			certPath = strings.TrimSpace(certPath)
			keyPath, _ := tls["keyPath"].(string)
			keyPath = strings.TrimSpace(keyPath)
			if (certPath == "") != (keyPath == "") {
				protocolName := string(inbound.Protocol)
				if inbound.Protocol == model.AnyTLS {
					protocolName = "AnyTLS"
				}
				return nil, fmt.Errorf("%s inbound %q must provide both TLS certificate and private key, or neither", protocolName, inbound.Tag)
			}
			if certPath == "" {
				// Prefer the panel HTTPS certificate, then fall back to the
				// dedicated subscription HTTPS certificate. Both are valid
				// certificate sources for a native Naive TLS listener.
				webCertPath, _ := nativeConfigSetting(snapshot, "webCertFile", singBoxSettingService.GetCertFile)
				webKeyPath, _ := nativeConfigSetting(snapshot, "webKeyFile", singBoxSettingService.GetKeyFile)
				webCertPath = strings.TrimSpace(webCertPath)
				webKeyPath = strings.TrimSpace(webKeyPath)
				if webCertPath != "" && webKeyPath != "" {
					certPath = webCertPath
					keyPath = webKeyPath
				} else {
					subCertPath, _ := nativeConfigSetting(snapshot, "subCertFile", singBoxSettingService.GetSubCertFile)
					subKeyPath, _ := nativeConfigSetting(snapshot, "subKeyFile", singBoxSettingService.GetSubKeyFile)
					subCertPath = strings.TrimSpace(subCertPath)
					subKeyPath = strings.TrimSpace(subKeyPath)
					if subCertPath != "" && subKeyPath != "" {
						certPath = subCertPath
						keyPath = subKeyPath
					}
				}
			}
			if certPath == "" || keyPath == "" {
				protocolName := string(inbound.Protocol)
				if inbound.Protocol == model.AnyTLS {
					protocolName = "AnyTLS"
				}
				return nil, fmt.Errorf("%s inbound %q requires a complete TLS certificate/private-key pair (inbound, panel HTTPS, or subscription HTTPS)", protocolName, inbound.Tag)
			}
			tls["certificatePath"] = certPath
			tls["keyPath"] = keyPath
			settings["tls"] = tls
		}

		raw["settings"] = settings
		if model.ShadowTLSTransport(inbound.Settings) != nil {
			outer, inner, err := singbox.TranslateShadowTLSWrappedInbound(raw)
			if err != nil {
				unsupported = append(unsupported, fmt.Sprintf("%s: %v", inbound.Tag, err))
				continue
			}
			cfg.Inbounds = append(cfg.Inbounds, inner, outer)
			continue
		}

		if inbound.Protocol == model.MASQUE {
			endpoint, err := singbox.TranslateMASQUEEndpoint(raw)
			if err != nil {
				return nil, err
			}
			cfg.Endpoints = append(cfg.Endpoints, endpoint)
			continue
		}

		translated, err := s.translateInbound(raw)
		if err != nil {
			unsupported = append(unsupported, fmt.Sprintf("%s: %v", inbound.Tag, err))
			continue
		}
		if inbound.Protocol == model.ShadowTLS {
			inner, err := singbox.TranslateShadowTLSInnerInbound(raw)
			if err != nil {
				unsupported = append(unsupported, fmt.Sprintf("%s: %v", inbound.Tag, err))
				continue
			}
			// Route and account bytes under the panel identity, on the injected listener.
			inner["tag"] = inbound.Tag
			translated["tag"] = "__shadowtls_transport_" + inbound.Tag
			translated["detour"] = inbound.Tag
			cfg.Inbounds = append(cfg.Inbounds, inner)
		}
		cfg.Inbounds = append(cfg.Inbounds, translated)
	}
	if len(unsupported) > 0 {
		return nil, fmt.Errorf("%s cannot represent enabled inbounds: %s", s.coreType(), strings.Join(unsupported, "; "))
	}
	managedMASQUETags := make(map[string]bool)
	for _, inbound := range inbounds {
		if inbound != nil && inbound.Protocol == model.MASQUE && inbound.NodeID == nil {
			managedMASQUETags[inbound.Tag] = true
		}
	}
	protectedOutbounds := selectSubscriptionConnections(cfg.Outbounds, subscriptionTags)
	protectedEndpoints := selectSubscriptionConnections(cfg.Endpoints, subscriptionTags)
	for _, endpoint := range cfg.Endpoints {
		if tag, _ := endpoint["tag"].(string); tag != "" {
			managedMASQUETags[tag] = true
		}
	}
	var nativeErr error
	if snapshot != nil {
		nativeErr = s.applyNativeTemplateValue(cfg, managedMASQUETags, snapshot.setting("singBoxConfigTemplate"), snapshot.routingLists)
	} else {
		nativeErr = s.applyNativeTemplate(cfg, managedMASQUETags)
	}
	if nativeErr != nil {
		return nil, nativeErr
	}
	if len(editorOnly) > 0 && editorOnly[0] {
		// The editor owns source policies, never the runtime's injected rules.
		// Keep catalog references instead of freezing their expanded contents.
		var source map[string]any
		if snapshot != nil && strings.TrimSpace(snapshot.setting("singBoxConfigTemplate")) != "" {
			if err := json.Unmarshal([]byte(snapshot.setting("singBoxConfigTemplate")), &source); err != nil {
				return nil, err
			}
			if route, ok := source["route"].(map[string]any); ok {
				rawRoute, _ := json.Marshal(route)
				if _, gatewayOnly := mergeGatewayOnlyRoute(nil, rawRoute); !gatewayOnly {
					cfg.Route = route
				}
			}
		}
		removeGeneratedSniffRules(cfg, sniffRules)
		return cfg, nil
	}
	cfg.Outbounds = mergeSubscriptionConnections(cfg.Outbounds, protectedOutbounds, subscriptionTags)
	cfg.Endpoints = mergeSubscriptionConnections(cfg.Endpoints, protectedEndpoints, subscriptionTags)
	{
		if err := configureHiddifyTunRouting(cfg, inbounds); err != nil {
			return nil, err
		}
	}
	for _, inbound := range inbounds {
		if err := injectSingBoxMtprotoEgress(cfg, inbound); err != nil {
			return nil, err
		}
	}
	if err := hiddify.AddReversePortals(cfg); err != nil {
		return nil, err
	}
	if len(sniffRules) > 0 {
		if cfg.Route == nil {
			cfg.Route = map[string]any{}
		}
		removeGeneratedSniffRules(cfg, sniffRules)
		cfg.Route["rules"] = append(sniffRules, singBoxRouteRules(cfg.Route)...)
	}
	ensureAutomaticClashAPI(cfg)
	if snapshot != nil {
		if err := snapshot.applyOverlays(cfg); err != nil {
			return nil, err
		}
	} else {
		applySingBoxInfrastructureEgress(cfg)
		if err := singBoxSettingService.applySingBoxAdBlock(cfg); err != nil {
			return nil, err
		}
		if err := singBoxSettingService.applySingBoxYouTubeServer(cfg); err != nil {
			return nil, err
		}
	}
	{
		if err := configureHiddifyGatewayDNS(cfg); err != nil {
			return nil, err
		}
		if err := s.filterManagedOutboundPlans(cfg); err != nil {
			return nil, err
		}
		reserved := make([]int, 0, len(inbounds))
		for _, inbound := range inbounds {
			if inbound != nil && inbound.Enable && inbound.NodeID == nil {
				reserved = append(reserved, inbound.Port)
			}
		}
		if err := prepareProtocolServersWithPolicy(cfg, s.strictProtocolServers, reserved...); err != nil {
			return nil, err
		}
		if err := hiddify.FinalizeServerConfig(cfg); err != nil {
			return nil, err
		}
		if cfg.Log == nil {
			cfg.Log = map[string]any{"level": "info"}
		}
		cfg.Log["output"] = s.logPath()
	}

	return cfg, nil
}

// Reject malformed sections rather than turning them into empty generated data.
func validatePanelConfigSections(template map[string]any) error {
	for _, key := range []string{"dns", "routing"} {
		if value := template[key]; value != nil {
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("hiddify panel template %s must be an object", key)
			}
		}
	}
	if value := template["outbounds"]; value != nil {
		members, ok := value.([]any)
		if !ok {
			return fmt.Errorf("hiddify panel template outbounds must be an array")
		}
		for i, item := range members {
			if _, ok := item.(map[string]any); !ok {
				return fmt.Errorf("hiddify panel template outbound %d must be an object", i)
			}
		}
	}
	return nil
}

// Native templates can replace generated outbounds. Start a planned sidecar
// only if the final outbound still uses its loopback bridge credentials.
func (s *SingBoxService) filterManagedOutboundPlans(cfg *singbox.Config) error {
	retained := make([]map[string]any, 0, len(cfg.ManagedOutbounds))
	for _, raw := range cfg.ManagedOutbounds {
		bridge, err := externalvpn.PlanOutbound(raw)
		if err != nil {
			return err
		}
		expected, err := s.translateOutbound(bridge)
		if err != nil {
			return err
		}
		for _, actual := range cfg.Outbounds {
			matches := true
			for _, key := range []string{"tag", "type", "server", "server_port", "uuid", "username", "password"} {
				wanted, _ := json.Marshal(expected[key])
				got, _ := json.Marshal(actual[key])
				if string(wanted) != string(got) {
					matches = false
					break
				}
			}
			if matches {
				retained = append(retained, raw)
				break
			}
		}
	}
	cfg.ManagedOutbounds = retained
	return nil
}
