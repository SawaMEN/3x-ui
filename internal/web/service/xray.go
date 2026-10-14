package service

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/amneziawg"
	"github.com/SawaMEN/3x-ui/v3/internal/amneziawgnet"
	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/tuic"
	"github.com/SawaMEN/3x-ui/v3/internal/util/json_util"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

var presenceRegistry presenceState

type presenceState struct {
	mu      sync.RWMutex
	process *xray.Presence
	result  string
}

func (s *presenceState) snapshot() (*xray.Presence, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.process, s.result
}

func (s *presenceState) replace(process *xray.Presence) {
	s.mu.Lock()
	s.process = process
	s.result = ""
	s.mu.Unlock()
}

func (s *presenceState) storeResult(process *xray.Presence, result string) {
	s.mu.Lock()
	if s.process == process && s.result == "" {
		s.result = result
	}
	s.mu.Unlock()
}

func currentOnlinePresence() *xray.Presence {
	process, _ := presenceRegistry.snapshot()
	return process
}

// CoreService manages the owner runtime and retains legacy wire-config encoding.
type CoreService struct {
	inboundService InboundService
	settingService SettingService
	nodeService    NodeService
}

// IsCoreRunning checks the sole owner sing-box runtime.
func (s *CoreService) IsCoreRunning() bool    { return SelectedNativeCore().IsRunning() }
func (s *CoreService) GetCoreVersion() string { return SelectedNativeCore().process().GetVersion() }
func (s *CoreService) GetCoreState() ProcessState {
	if SelectedNativeCore().IsRunning() {
		return Running
	}
	if SelectedNativeCore().LastError() != nil {
		return Error
	}
	return Stop
}

// OnlinePresence exposes local and remote online-client state independently of a process.
func OnlinePresence() *xray.Presence {
	return currentOnlinePresence()
}

// GetXrayResult returns the result string from the Xray process.
func (s *CoreService) GetCoreResult() string {
	if err := SelectedNativeCore().LastError(); err != nil {
		return err.Error()
	}
	return ""
}

// RemoveIndex removes an element at the specified index from a slice.
// Returns a new slice with the element removed.
func RemoveIndex(s []any, index int) []any {
	return append(s[:index], s[index+1:]...)
}

// GetXrayConfig retrieves and builds the Xray configuration from settings and inbounds.
func (s *CoreService) GetXrayConfig() (*xray.Config, error) {
	templateConfig, err := s.settingService.GetXrayConfigTemplate()
	if err != nil {
		return nil, err
	}
	routingLists, err := loadRoutingLists(templateConfig)
	if err != nil {
		return nil, err
	}
	templateConfig, err = materializeRoutingLists(templateConfig, routingLists)
	if err != nil {
		return nil, err
	}

	xrayConfig := &xray.Config{}
	err = json.Unmarshal([]byte(templateConfig), xrayConfig)
	if err != nil {
		return nil, err
	}
	xrayConfig.LogConfig = resolveXrayLogPaths(xrayConfig.LogConfig)
	xrayConfig.API = ensureAPIServices(xrayConfig.API)
	xrayConfig.Policy = ensureStatsPolicy(xrayConfig.Policy)
	xrayConfig.RouterConfig = stripDisabledRules(xrayConfig.RouterConfig)
	// Template outbounds authored before the xray-core #6258 XHTTP rename may
	// still carry sessionPlacement/sessionKey; lift them too (same reason as
	// the per-inbound lift below).
	xrayConfig.OutboundConfigs = liftOutboundsXhttpSessionIDKeys(xrayConfig.OutboundConfigs)
	// Bridge amneziawg outbounds before anything else reads OutboundConfigs;
	// the core has no amneziawg proxy and would reject the raw entry.
	if err := transformAmneziaWGOutbounds(xrayConfig); err != nil {
		return nil, err
	}

	_, _, _ = s.inboundService.AddTraffic(nil, nil)

	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		if !inbound.Enable {
			continue
		}
		if inbound.NodeID != nil {
			continue
		}
		if model.ShadowTLSTransport(inbound.Settings) != nil || inbound.Protocol == model.MTProto || inbound.Protocol == model.AmneziaWG || inbound.Protocol == model.TUIC || inbound.Protocol == model.Pingtunnel || inbound.Protocol == model.TrustTunnel || inbound.Protocol == model.VKTurnProxy || inbound.Protocol == model.NaiveProxy || inbound.Protocol == model.MASQUE || inbound.Protocol == model.Snell || inbound.Protocol == model.AnyTLS || inbound.Protocol == model.ShadowTLS || inbound.Protocol == model.Mieru || inbound.Protocol == model.Sudoku {
			// NaiveProxy is a sing-box-only inbound. A legacy Naive row may still
			// exist when the selected core was switched back to Xray; never emit it
			// into an Xray config because xray-core has no Naive inbound handler.
			continue
		}
		settings := map[string]any{}
		_ = json.Unmarshal([]byte(inbound.Settings), &settings)
		var wireguardClientsByEmail map[string]model.Client
		if inbound.Protocol == model.WireGuard {
			inboundClients, _ := ParseInboundSettingsClients(inbound.Settings)
			if len(inboundClients) > 0 {
				wireguardClientsByEmail = make(map[string]model.Client, len(inboundClients))
				for _, client := range inboundClients {
					wireguardClientsByEmail[strings.ToLower(strings.TrimSpace(client.Email))] = client
				}
			}
		}

		dbClients, listErr := s.inboundService.clientService.ListForInbound(nil, inbound.Id)
		if listErr != nil {
			return nil, listErr
		}

		clientStats := inbound.ClientStats
		enableMap := make(map[string]bool, len(clientStats))
		for _, clientTraffic := range clientStats {
			enableMap[clientTraffic.Email] = clientTraffic.Enable
		}

		finalClients := make([]any, 0, len(dbClients))
		var wgPeers []any
		for i := range dbClients {
			c := dbClients[i]
			if enable, exists := enableMap[c.Email]; exists && !enable {
				logger.Infof("Remove Inbound User %s due to expiration or traffic limit", c.Email)
				continue
			}
			if !c.Enable {
				continue
			}
			flow := c.Flow
			if flow == "xtls-rprx-vision-udp443" {
				flow = "xtls-rprx-vision"
			}
			if inbound.DisableFlow {
				flow = ""
			}
			entry := map[string]any{"email": c.Email}
			switch inbound.Protocol {
			case model.VLESS:
				if c.ID != "" {
					entry["id"] = c.ID
				}
				if flow != "" {
					entry["flow"] = flow
				}
				if c.Reverse != nil {
					entry["reverse"] = c.Reverse
				}
			case model.VMESS:
				if c.ID != "" {
					entry["id"] = c.ID
				}
				if c.Security != "" {
					entry["security"] = c.Security
				}
			case model.Trojan:
				if c.Password != "" {
					entry["password"] = c.Password
				}
				if flow != "" {
					entry["flow"] = flow
				}
			case model.Shadowsocks:
				if c.Password != "" {
					entry["password"] = c.Password
				}
			case model.Hysteria:
				if c.Auth != "" {
					entry["auth"] = c.Auth
				}
			case model.WireGuard:
				if inboundClient, ok := wireguardClientsByEmail[strings.ToLower(strings.TrimSpace(c.Email))]; ok {
					c.AllowedIPs = inboundClient.AllowedIPs
					c.PreSharedKey = inboundClient.PreSharedKey
				}
				wgPeers = append(wgPeers, model.WireguardPeerFromClient(c))
				continue
			}
			finalClients = append(finalClients, entry)
		}

		var mutated bool
		if inbound.Protocol == model.WireGuard {
			delete(settings, "clients")
			if wgPeers == nil {
				wgPeers = []any{}
			}
			settings["peers"] = wgPeers
			mutated = true
		} else {
			_, hadClients := settings["clients"]
			mutated = hadClients || len(finalClients) > 0
			if mutated {
				settings["clients"] = finalClients
			}
		}

		if inboundCanHostFallbacks(inbound) {
			fallbacks, fbErr := s.inboundService.fallbackService.BuildFallbacksJSON(nil, inbound.Id)
			if fbErr != nil {
				return nil, fbErr
			}
			if len(fallbacks) > 0 {
				generic := make([]any, 0, len(fallbacks))
				for _, f := range fallbacks {
					generic = append(generic, f)
				}
				settings["fallbacks"] = generic
				mutated = true
			}
		}

		if mutated {
			modifiedSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return nil, err
			}
			inbound.Settings = string(modifiedSettings)
		}

		if len(inbound.StreamSettings) > 0 {
			// Unmarshal stream JSON
			var stream map[string]any
			_ = json.Unmarshal([]byte(inbound.StreamSettings), &stream)

			// Remove the "settings" field under "tlsSettings" and "realitySettings"
			tlsSettings, ok1 := stream["tlsSettings"].(map[string]any)
			realitySettings, ok2 := stream["realitySettings"].(map[string]any)
			if ok1 || ok2 {
				if ok1 {
					delete(tlsSettings, "settings")
				} else if ok2 {
					delete(realitySettings, "settings")
				}
			}

			delete(stream, "externalProxy")

			// finalmask.tcp + REALITY panics Xray-core on the first connection
			// (XTLS/Xray-core#6453). AddInbound/UpdateInbound reject this
			// combination at save time, but a row saved before that guard
			// existed (upgrade, node sync, restored backup, direct DB edit)
			// would still crash Xray on the next restart without this — drop
			// it here too, the same way liftXhttpSessionIDKeys and
			// HealShadowsocksClientMethods heal other legacy data in place.
			if len(finalMaskRealityTcpMasks(stream)) > 0 {
				logger.Warningf("Inbound %q: dropping finalmask, incompatible with REALITY security (crashes Xray-core, see XTLS/Xray-core#6453)", inbound.Tag)
				delete(stream, "finalmask")
			}

			dropEmptyRandPackets(stream["finalmask"])

			if dropped := stripIncompleteXmcMasks(stream); dropped > 0 {
				logger.Warningf("Inbound %q: dropping %d XMC finalmask mask(s) without complete Minecraft profiles — reconfigure them to restore the obfuscation (see XTLS/Xray-core#6487)", inbound.Tag, dropped)
			}

			// xray-core v26.6.22 (#6258) renamed the XHTTP session keys and
			// kept no fallback. Lift legacy sessionPlacement/sessionKey onto the
			// new names here so inbounds stored before the rename keep working
			// without the admin re-saving them.
			liftXhttpSessionIDKeys(stream)

			newStream, err := json.MarshalIndent(stream, "", "  ")
			if err != nil {
				return nil, err
			}
			inbound.StreamSettings = string(newStream)
		}

		if inbound.Protocol == model.Shadowsocks {
			if healed, ok := model.HealShadowsocksClientMethods(inbound.Settings); ok {
				inbound.Settings = healed
			}
		}

		inboundConfig := xray.InboundFromPanel(inbound.GenInboundConfig())
		xrayConfig.InboundConfigs = append(xrayConfig.InboundConfigs, *inboundConfig)
	}

	// Merge subscription-derived outbounds (if any) into the final outbounds array.
	// These are additive: each subscription is placed before or after the template
	// outbounds based on its Prepend flag, ordered by Priority. Tags assigned by the
	// subscription service are kept stable across refreshes so that balancers and
	// routing rules continue to work.
	subSvc := &OutboundSubscriptionService{}
	if prepend, appendList, err := subSvc.activeOutboundsSplit(); err == nil && (len(prepend) > 0 || len(appendList) > 0) {
		mergeSubscriptionOutbounds(xrayConfig, prepend, appendList)
	}

	if err := injectAutomaticOutboundRoutes(xrayConfig); err != nil {
		return nil, err
	}

	// Route opted-in local mtproto inbounds through the core's router. Each one
	// gets a loopback SOCKS bridge — tagged with the inbound's own tag so it is
	// matchable in routing rules — that its mtg sidecar dials Telegram through.
	// Done after the subscription merge so a selected subscription outbound (or
	// balancer) is a valid rule target.
	for i := range inbounds {
		inbound := inbounds[i]
		if inbound.Protocol != model.MTProto || !inbound.Enable || inbound.NodeID != nil {
			continue
		}
		injectMtprotoEgress(xrayConfig, inbound)
	}

	// Every AmneziaWG inbound is embedded (internal/amneziawgnet: amneziawg-go
	// over a gVisor netstack, no kernel module) and relays every peer's
	// decapsulated traffic into its own loopback SOCKS5 inbound, always on —
	// unlike mtproto's bridge above, there's no opt-in gate here: once
	// traffic is decapsulated in gVisor, Xray's own freedom outbound is the
	// only way it reaches the real internet at all, not an optional extra
	// hop. Whether it goes anywhere beyond Xray's default routing is up to
	// whatever rules the admin adds through the stock Routing page, exactly
	// like routing any other protocol.
	injectAmneziawgnetSocks(xrayConfig, inbounds)
	injectTuicSocks(xrayConfig, inbounds)

	// Restores each opted-in peer's own distinct public IPv6 source identity
	// for its outbound connections — a peer that has an IPv6 address in its
	// AllowedIPs, on an inbound with IPv6Enabled, gets its own freedom
	// outbound bound to that exact address via sendThrough.
	// internal/amneziawgnet's own Manager is responsible for actually
	// aliasing that address onto the host (see v6alias.go) so the kernel
	// lets Xray bind an egress socket to it at all; this call only builds
	// the Xray-side outbound/routing-rule half.
	injectAmneziawgV6Egress(xrayConfig, inbounds)

	// Wire the panel's own HTTP traffic through the configured outbound, after
	// the subscription merge so subscription outbound tags are valid targets.
	if egressTag, err := s.settingService.GetPanelOutbound(); err != nil {
		logger.Warning("read panelOutbound setting failed:", err)
	} else if egressTag != "" {
		injectPanelEgress(xrayConfig, egressTag)
	}

	nodes, err := s.nodeService.GetAll()
	if err != nil {
		logger.Warning("read nodes for egress injection failed:", err)
	} else {
		injectNodeEgresses(xrayConfig, nodes)
	}

	if err := s.settingService.applyXrayAdBlock(xrayConfig); err != nil {
		return nil, err
	}
	if err := s.settingService.applyXrayYouTubeServer(xrayConfig); err != nil {
		return nil, err
	}
	if err := transformAdditionalOutbounds(xrayConfig); err != nil {
		return nil, err
	}
	return xrayConfig, nil
}

// PanelEgressInboundTag is the tag of the loopback SOCKS inbound injected into
// the generated config when a panel outbound is configured. The panel's own
// HTTP clients dial through it to egress via the chosen outbound.
const PanelEgressInboundTag = "panel-egress"

// panelEgressBasePort is the first port tried for the egress bridge; ports
// already taken by other inbounds in the generated config are skipped.
const panelEgressBasePort = 62790

// injectPanelEgress appends a loopback SOCKS inbound and routing rule only when
// outboundTag resolves in the final outbound or balancer set. Otherwise the
// entire injection is skipped. Generated state is hot-appliable and never
// modifies the stored template or restarts the core.
func injectPanelEgress(cfg *xray.Config, outboundTag string) {
	for i := range cfg.InboundConfigs {
		if cfg.InboundConfigs[i].Tag == PanelEgressInboundTag {
			logger.Warning("panel egress: inbound tag [", PanelEgressInboundTag, "] already exists, skipping injection")
			return
		}
	}

	// The rule must exist before the inbound takes traffic, otherwise the
	// bridge would silently egress through the default outbound instead.
	routing := map[string]any{}
	if len(cfg.RouterConfig) > 0 {
		if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
			logger.Warning("panel egress: routing section is unparsable, skipping injection:", err)
			return
		}
	}
	if !routingTargetExists(routing, cfg.OutboundConfigs, outboundTag) {
		logger.Warning("panel egress: target tag [", outboundTag, "] not found, skipping injection")
		return
	}
	rules, _ := routing["rules"].([]any)
	rule := map[string]any{
		"type":       "field",
		"inboundTag": []any{PanelEgressInboundTag},
	}
	// The configured tag may name a routing balancer instead of a concrete
	// outbound. A field rule can target either, so emit the matching key —
	// balancerTag load-balances the panel's own traffic across the balancer's
	// outbounds, while a plain outbound tag keeps the original behavior.
	if routingTagIsBalancer(routing, outboundTag) {
		rule["balancerTag"] = outboundTag
	} else {
		rule["outboundTag"] = outboundTag
	}
	routing["rules"] = append([]any{rule}, rules...)
	newRouting, err := json.Marshal(routing)
	if err != nil {
		logger.Warning("panel egress: failed to rebuild routing section, skipping injection:", err)
		return
	}
	cfg.RouterConfig = json_util.RawMessage(newRouting)

	used := make(map[int]struct{}, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		used[cfg.InboundConfigs[i].Port] = struct{}{}
	}
	port := panelEgressBasePort
	for {
		if _, taken := used[port]; !taken {
			break
		}
		port++
	}

	cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{
		Listen:   json_util.RawMessage(`"127.0.0.1"`),
		Port:     port,
		Protocol: "socks",
		Settings: json_util.RawMessage(`{"auth":"noauth","udp":false}`),
		Tag:      PanelEgressInboundTag,
	})
}

func outboundTagExists(outbounds json_util.RawMessage, tag string) bool {
	var parsed []struct {
		Tag string `json:"tag"`
	}
	if tag == "" || json.Unmarshal(outbounds, &parsed) != nil {
		return false
	}
	for _, outbound := range parsed {
		if outbound.Tag == tag {
			return true
		}
	}
	return false
}

func routingTargetExists(routing map[string]any, outbounds json_util.RawMessage, tag string) bool {
	return routingTagIsBalancer(routing, tag) || outboundTagExists(outbounds, tag)
}

// NodeEgressInboundTag returns the loopback SOCKS inbound tag for a given node.
func NodeEgressInboundTag(nodeID int) string {
	return fmt.Sprintf("node-egress-%d", nodeID)
}

// nodeEgressBasePort is the first port tried for node egress bridges.
const nodeEgressBasePort = 62800

// injectNodeEgresses appends a loopback SOCKS inbound per enabled node that has
// an OutboundTag, and prepends a routing rule sending that inbound's traffic to
// the selected outbound tag. These bridges are hot-appliable.
func injectNodeEgresses(cfg *xray.Config, nodes []*model.Node) {
	routing := map[string]any{}
	if len(cfg.RouterConfig) > 0 {
		if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
			logger.Warning("node egress: routing section is unparsable, skipping injection:", err)
			return
		}
	}

	used := make(map[int]struct{}, len(cfg.InboundConfigs))
	usedTags := make(map[string]struct{}, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		used[cfg.InboundConfigs[i].Port] = struct{}{}
		usedTags[cfg.InboundConfigs[i].Tag] = struct{}{}
	}

	rules, _ := routing["rules"].([]any)
	newRules := make([]any, 0)

	for _, n := range nodes {
		if !n.Enable || n.OutboundTag == "" {
			continue
		}
		if !routingTargetExists(routing, cfg.OutboundConfigs, n.OutboundTag) {
			logger.Warning("node egress: target tag [", n.OutboundTag, "] not found, skipping node [", n.Id, "]")
			continue
		}
		tag := NodeEgressInboundTag(n.Id)
		if _, exists := usedTags[tag]; exists {
			logger.Warning("node egress: inbound tag [", tag, "] already exists, skipping")
			continue
		}
		usedTags[tag] = struct{}{}

		rule := map[string]any{
			"type":       "field",
			"inboundTag": []any{tag},
		}
		if routingTagIsBalancer(routing, n.OutboundTag) {
			rule["balancerTag"] = n.OutboundTag
		} else {
			rule["outboundTag"] = n.OutboundTag
		}
		newRules = append(newRules, rule)

		port := nodeEgressBasePort + n.Id
		for {
			if _, taken := used[port]; !taken {
				break
			}
			port++
		}
		used[port] = struct{}{}

		cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{
			Listen:   json_util.RawMessage(`"127.0.0.1"`),
			Port:     port,
			Protocol: "socks",
			Settings: json_util.RawMessage(`{"auth":"noauth","udp":false}`),
			Tag:      tag,
		})
	}

	if len(newRules) == 0 {
		return
	}
	routing["rules"] = append(newRules, rules...)
	newRouting, err := json.Marshal(routing)
	if err != nil {
		logger.Warning("node egress: failed to rebuild routing section, skipping injection:", err)
		return
	}
	cfg.RouterConfig = json_util.RawMessage(newRouting)
}

// routingTagIsBalancer reports whether tag names a balancer in the parsed
// routing section. The panel-egress rule targets a balancer via balancerTag and
// a concrete outbound via outboundTag, so the caller picks the key from this.
func routingTagIsBalancer(routing map[string]any, tag string) bool {
	if tag == "" {
		return false
	}
	balancers, ok := routing["balancers"].([]any)
	if !ok {
		return false
	}
	for _, b := range balancers {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := bm["tag"].(string); ok && t == tag {
			return true
		}
	}
	return false
}

// mtprotoEgressSocksSettings is the loopback SOCKS server a routed mtproto
// inbound exposes for its mtg sidecar to dial Telegram through. mtg makes plain
// TCP connections, so UDP is left off (matching the panel egress bridge).
const mtprotoEgressSocksSettings = `{"auth":"noauth","udp":false}`

// injectMtprotoEgress wires one routed mtproto inbound into the generated
// config after any selected outbound resolves in the final target set. Invalid
// selected targets or routing data skip the entire injection; without a selected
// outbound, the bridge retains default-route behavior. Generated state remains
// hot-appliable, leaves the stored template untouched, and never forces a full
// Xray restart. Mirrors injectPanelEgress.
func injectMtprotoEgress(cfg *xray.Config, inbound *model.Inbound) {
	var parsed struct {
		RouteThroughXray bool   `json:"routeThroughXray"`
		RouteXrayPort    int    `json:"routeXrayPort"`
		OutboundTag      string `json:"outboundTag"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &parsed); err != nil {
		return
	}
	if !parsed.RouteThroughXray || parsed.RouteXrayPort <= 0 || inbound.Tag == "" {
		return
	}
	tag := inbound.Tag
	for i := range cfg.InboundConfigs {
		if cfg.InboundConfigs[i].Tag == tag {
			logger.Warning("mtproto egress: inbound tag [", tag, "] already present in generated config, skipping bridge")
			return
		}
	}

	if parsed.OutboundTag != "" {
		routing := map[string]any{}
		if len(cfg.RouterConfig) > 0 {
			if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
				logger.Warning("mtproto egress: routing section is unparsable, skipping injection:", err)
				return
			}
		}
		if !routingTargetExists(routing, cfg.OutboundConfigs, parsed.OutboundTag) {
			logger.Warning("mtproto egress: target tag [", parsed.OutboundTag, "] not found, skipping injection")
			return
		}
		rules, _ := routing["rules"].([]any)
		rule := map[string]any{
			"type":       "field",
			"inboundTag": []any{tag},
		}
		if routingTagIsBalancer(routing, parsed.OutboundTag) {
			rule["balancerTag"] = parsed.OutboundTag
		} else {
			rule["outboundTag"] = parsed.OutboundTag
		}
		routing["rules"] = append([]any{rule}, rules...)
		newRouting, err := json.Marshal(routing)
		if err != nil {
			logger.Warning("mtproto egress: failed to rebuild routing section, skipping injection:", err)
			return
		}
		cfg.RouterConfig = json_util.RawMessage(newRouting)
	}

	cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{
		Listen:   json_util.RawMessage(`"127.0.0.1"`),
		Port:     parsed.RouteXrayPort,
		Protocol: "socks",
		Settings: json_util.RawMessage(mtprotoEgressSocksSettings),
		Tag:      tag,
	})
}

// amneziawgEgressSniffingSettings matches this fork's normal per-inbound
// default (see default.json's "mixed" inbound). Without this, domain-based
// Routing rules can never match this relay: the peer resolved DNS
// itself, through the tunnel, before ever sending a packet — by the time the
// embedded forwarder recovers the decapsulated traffic, the destination is
// already a bare IP, with no domain name attached at the network layer at
// all. Sniffing recovers it from the payload itself (TLS SNI / HTTP Host /
// QUIC) the same way it already does for every other inbound; without it,
// only tag/IP/network-based rules can ever match this traffic, and any
// domain rule above it in the list is silently unreachable.
const amneziawgEgressSniffingSettings = `{"enabled":true,"destOverride":["http","tls","quic","fakedns"]}`

// injectAmneziawgnetSocks gives every enabled AmneziaWG inbound with at
// least one qualifying peer its own loopback SOCKS5 inbound for the
// embedded (amneziawg-go) relay path (internal/amneziawgnet) -- always on,
// since there is no alternative datapath once traffic is decapsulated in
// gVisor: Xray's own freedom outbound is how it reaches the real internet at
// all (see internal/amneziawgnet/relay.go's doc comment, Finding 3 of the
// migration plan). Tagged with the inbound's own real tag: it's already
// selectable in the panel's stock Routing page (InboundService.GetInboundTags
// is protocol-blind), and per-inbound traffic totals
// (internal/web/service/inbound_traffic.go's addClientTraffic) match by
// exact tag -- reusing it isn't a style choice.
func injectAmneziawgnetSocks(cfg *xray.Config, inbounds []*model.Inbound) {
	existingTags := make(map[string]struct{}, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		existingTags[cfg.InboundConfigs[i].Tag] = struct{}{}
	}

	for _, inbound := range inbounds {
		if inbound.Protocol != model.AmneziaWG || !inbound.Enable || inbound.NodeID != nil {
			continue
		}
		inst, ok := amneziawg.InstanceFromInbound(inbound)
		if !ok {
			continue
		}
		if _, taken := existingTags[inbound.Tag]; taken {
			logger.Warning("amneziawgnet socks: inbound tag [", inbound.Tag, "] already present in generated config, skipping its relay inbound")
			continue
		}

		emails := make([]string, 0, len(inst.Peers))
		for _, p := range inst.Peers {
			if p.Email != "" {
				emails = append(emails, p.Email)
			}
		}
		if len(emails) == 0 {
			continue
		}

		settings, err := amneziawgnet.SocksInboundSettings(emails, amneziawgnet.SocksPassword())
		if err != nil {
			logger.Warning("amneziawgnet socks: building settings for inbound [", inbound.Tag, "]: ", err)
			continue
		}

		existingTags[inbound.Tag] = struct{}{}
		cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{
			Listen:   json_util.RawMessage(`"127.0.0.1"`),
			Port:     amneziawgnet.SOCKSPortForInbound(inbound.Id),
			Protocol: "socks",
			Settings: json_util.RawMessage(settings),
			Sniffing: json_util.RawMessage(amneziawgEgressSniffingSettings),
			Tag:      inbound.Tag,
		})
	}
}

const (
	tuicEgressSocksSettings    = `{"auth":"noauth","udp":true}`
	tuicEgressSniffingSettings = `{"enabled":true,"destOverride":["http","tls","quic","fakedns"]}`
)

func injectTuicSocks(cfg *xray.Config, inbounds []*model.Inbound) {
	existingTags := make(map[string]struct{}, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		existingTags[cfg.InboundConfigs[i].Tag] = struct{}{}
	}

	for _, inbound := range inbounds {
		if inbound.Protocol != model.TUIC || !inbound.Enable || inbound.NodeID != nil {
			continue
		}
		if _, taken := existingTags[inbound.Tag]; taken {
			logger.Warning("tuic socks: inbound tag [", inbound.Tag, "] already present in generated config, skipping its relay inbound")
			continue
		}

		existingTags[inbound.Tag] = struct{}{}
		cfg.InboundConfigs = append(cfg.InboundConfigs, xray.InboundConfig{
			Listen:   json_util.RawMessage(`"127.0.0.1"`),
			Port:     tuic.SOCKSPortForInbound(inbound.Id),
			Protocol: "socks",
			Settings: json_util.RawMessage(tuicEgressSocksSettings),
			Sniffing: json_util.RawMessage(tuicEgressSniffingSettings),
			Tag:      inbound.Tag,
		})
	}
}

// amneziawgV6EgressTag returns the stable, globally-unique freedom outbound
// tag for one peer's IPv6 source-identity egress. Stable across config
// regenerations (a pure function of two stable identifiers), so
// internal/xray/hot_diff.go's tag-keyed outbound/routing diffing recognizes
// "unchanged" rather than remove+recreate on every poll. The inbound.Id
// prefix is defense in depth, not load-bearing on its own: email is already
// enforced globally unique across the whole panel's client table
// (model.ClientRecord.Email has a gorm uniqueIndex) — kept anyway since it
// costs nothing and makes the tag self-describing, matching
// NodeEgressInboundTag's own style.
func amneziawgV6EgressTag(inboundID int, email string) string {
	return fmt.Sprintf("amneziawg-v6-%d-%s", inboundID, email)
}

// injectAmneziawgV6Egress gives every enabled, non-node-hosted AmneziaWG
// peer with an IPv6 AllowedIPs entry its own single-purpose freedom
// outbound, bound via sendThrough to that exact address, plus a routing
// rule sending only that peer's own traffic through it — restoring the
// per-client public IPv6 identity the hard cutover temporarily dropped
// (Phase 3.5 of the migration plan). Scoped to outbound source identity
// only: it depends on internal/amneziawgnet's own alias mechanism actually
// giving the host that address at the OS level (see v6alias.go's
// V6AliasesActive, the exact same gate this function uses below) — without
// that, sendThrough fails to bind and every connection through it errors
// outright (freedom.go's dial failure); there is no fallback outbound.
//
// The routing rule matches both inboundTag and user: SocksInboundSettings
// (used by injectAmneziawgnetSocks above) already authenticates each
// connection as the peer's own email via stock SOCKS5 auth, and a stock
// Xray SOCKS5 inbound sets that connection's stats/routing identity from
// the authenticated username — so "user" reliably isolates exactly one
// peer's traffic, the same building block Finding 3 of the migration plan
// already established for per-client stats.
//
// Modeled on injectNodeEgresses (the established N-per-slice inbound+rule
// precedent, not injectAmneziawgnetSocks itself, which only ever emits a
// single inbound and never touches outbounds/routing) and
// mergeSubscriptionOutbounds's unmarshal-append-remarshal pattern for
// cfg.OutboundConfigs. Synthetic rules are prepended ahead of whatever's
// already in the routing rules array, the same pattern injectNodeEgresses/
// injectMtprotoEgress already use for their own always-must-win infra
// rules — this never touches the admin's own saved Routing-page rule
// order.
func injectAmneziawgV6Egress(cfg *xray.Config, inbounds []*model.Inbound) {
	// Protocol is checked alongside Tag, not just Tag alone: a tag collision
	// with some unrelated (non-socks) inbound must not be mistaken for this
	// instance's own relay having been created.
	liveInboundTags := make(map[string]struct{}, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		if cfg.InboundConfigs[i].Protocol == "socks" {
			liveInboundTags[cfg.InboundConfigs[i].Tag] = struct{}{}
		}
	}

	var existingOutbounds []any
	if len(cfg.OutboundConfigs) > 0 {
		if err := json.Unmarshal(cfg.OutboundConfigs, &existingOutbounds); err != nil {
			logger.Warning("amneziawg v6 egress: outbounds section is unparsable, skipping injection:", err)
			return
		}
	}
	usedOutboundTags := make(map[string]struct{}, len(existingOutbounds))
	for _, o := range existingOutbounds {
		if m, ok := o.(map[string]any); ok {
			if t, ok := m["tag"].(string); ok {
				usedOutboundTags[t] = struct{}{}
			}
		}
	}

	routing := map[string]any{}
	if len(cfg.RouterConfig) > 0 {
		if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
			logger.Warning("amneziawg v6 egress: routing section is unparsable, skipping injection:", err)
			return
		}
	}
	rules, _ := routing["rules"].([]any)
	newRules := make([]any, 0)
	newOutbounds := make([]any, 0)

	for _, inbound := range inbounds {
		if inbound.Protocol != model.AmneziaWG || !inbound.Enable || inbound.NodeID != nil {
			continue
		}
		if _, live := liveInboundTags[inbound.Tag]; !live {
			// The relay inbound itself wasn't created this pass (e.g. a tag
			// collision inside injectAmneziawgnetSocks) -- no SOCKS5 inbound
			// exists for hot_diff.go's inboundTag match to ever fire against.
			continue
		}
		inst, ok := amneziawg.InstanceFromInbound(inbound)
		if !ok || !amneziawgnet.V6AliasesActive(inst) {
			continue
		}
		for _, p := range inst.Peers {
			if p.Email == "" {
				continue
			}
			v6 := amneziawg.FirstIPv6(p.AllowedIPs)
			if v6 == "" {
				continue
			}
			tag := amneziawgV6EgressTag(inbound.Id, p.Email)
			if _, taken := usedOutboundTags[tag]; taken {
				logger.Warning("amneziawg v6 egress: outbound tag [", tag, "] already exists, skipping peer [", p.Email, "]")
				continue
			}
			usedOutboundTags[tag] = struct{}{}
			newOutbounds = append(newOutbounds, map[string]any{
				"tag":         tag,
				"protocol":    "freedom",
				"sendThrough": v6,
				"settings":    map[string]any{},
			})
			newRules = append(newRules, map[string]any{
				"type":        "field",
				"inboundTag":  []any{inbound.Tag},
				"user":        []any{p.Email},
				"outboundTag": tag,
			})
		}
	}

	if len(newOutbounds) == 0 {
		return
	}

	merged := make([]any, 0, len(existingOutbounds))
	merged = append(merged, existingOutbounds...)
	merged = append(merged, newOutbounds...)
	combined, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		logger.Warning("amneziawg v6 egress: failed to rebuild outbounds section, skipping injection:", err)
		return
	}
	cfg.OutboundConfigs = json_util.RawMessage(combined)

	routing["rules"] = append(newRules, rules...)
	newRouting, err := json.Marshal(routing)
	if err != nil {
		logger.Warning("amneziawg v6 egress: failed to rebuild routing section, skipping injection:", err)
		return
	}
	cfg.RouterConfig = json_util.RawMessage(newRouting)
}

// mergeSubscriptionOutbounds appends the subscription outbounds to the
// OutboundConfigs array of the xray config. It works on the already-unmarshaled
// template so that manually configured outbounds are never overwritten.
//
// Safety: if we cannot parse the template's outbounds array, we leave
// OutboundConfigs exactly as it came from the template (we do not inject
// subscription outbounds). This prevents us from accidentally dropping the
// user's manually configured outbounds when the template is in a weird state.
func mergeSubscriptionOutbounds(cfg *xray.Config, prepend, appendList []any) {
	if len(prepend) == 0 && len(appendList) == 0 {
		return
	}
	var templateOutbounds []any
	if len(cfg.OutboundConfigs) > 0 {
		if err := json.Unmarshal(cfg.OutboundConfigs, &templateOutbounds); err != nil {
			// Corrupt template outbounds — do not touch the field at all.
			// The user will see problems on Xray start / next save.
			return
		}
	}
	var merged []any
	merged = append(merged, prepend...)
	merged = append(merged, templateOutbounds...)
	merged = append(merged, appendList...)
	combined, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return
	}
	cfg.OutboundConfigs = json_util.RawMessage(combined)
}

// ensureAPIServices guarantees the gRPC services the panel depends on are
// listed in the generated config's api block: HandlerService and StatsService
// have always been required for inbound/user management and traffic polling,
// and RoutingService enables hot routing reload on templates saved before it
// was added to the default template. The stored template itself is not
// modified — only the generated runtime config.
func ensureAPIServices(api json_util.RawMessage) json_util.RawMessage {
	if len(api) == 0 {
		// No api block means the panel's API integration is deliberately
		// disabled; don't resurrect it behind the user's back.
		return api
	}
	var parsed map[string]any
	if err := json.Unmarshal(api, &parsed); err != nil {
		return api
	}
	services, _ := parsed["services"].([]any)
	have := make(map[string]bool, len(services))
	for _, svc := range services {
		if name, ok := svc.(string); ok {
			have[name] = true
		}
	}
	added := false
	for _, name := range []string{"HandlerService", "StatsService", "RoutingService"} {
		if !have[name] {
			services = append(services, name)
			added = true
		}
	}
	if !added {
		return api
	}
	parsed["services"] = services
	out, err := json.Marshal(parsed)
	if err != nil {
		return api
	}
	return out
}

// ensureStatsPolicy guarantees every policy level in the generated config has
// statsUserOnline enabled, so the core tracks per-email online IPs for the
// panel's online view and access-log-free IP limiting. Generated clients carry
// no explicit level, so level "0" is created when absent. The flag is panel
// infrastructure and is forced on even over an explicit false in the template,
// same as the api services above. An entirely missing or unparsable policy
// block is left alone; the stored template itself is never modified — only the
// generated runtime config.
func ensureStatsPolicy(policy json_util.RawMessage) json_util.RawMessage {
	if len(policy) == 0 {
		return policy
	}
	var parsed map[string]any
	if err := json.Unmarshal(policy, &parsed); err != nil {
		return policy
	}
	levels, _ := parsed["levels"].(map[string]any)
	if levels == nil {
		levels = make(map[string]any)
	}
	if _, ok := levels["0"]; !ok {
		levels["0"] = map[string]any{}
	}
	changed := false
	for _, raw := range levels {
		level, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if enabled, ok := level["statsUserOnline"].(bool); !ok || !enabled {
			level["statsUserOnline"] = true
			changed = true
		}
	}
	if !changed {
		return policy
	}
	parsed["levels"] = levels
	out, err := json.Marshal(parsed)
	if err != nil {
		return policy
	}
	return out
}

// caseVariantKeys returns every key of parsed that equals want ignoring case,
// lowest first so the fold is deterministic when several variants are present.
func caseVariantKeys(parsed map[string]any, want string) []string {
	var keys []string
	for key := range parsed {
		if strings.EqualFold(key, want) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func resolveXrayLogPaths(logCfg json_util.RawMessage) json_util.RawMessage {
	if len(logCfg) == 0 {
		return logCfg
	}
	var parsed map[string]any
	if err := json.Unmarshal(logCfg, &parsed); err != nil {
		return logCfg
	}
	changed := false
	for _, key := range []string{"access", "error"} {
		// xray-core decodes this object with encoding/json, whose case-insensitive
		// field match makes "Access" reach AccessLog too — fold every variant.
		variants := caseVariantKeys(parsed, key)
		value, hasValue := parsed[key]
		for _, variant := range variants {
			if variant == key {
				continue
			}
			if !hasValue {
				value, hasValue = parsed[variant], true
			}
			delete(parsed, variant)
			changed = true
		}
		v, ok := value.(string)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(v)
		if trimmed == "" || strings.EqualFold(trimmed, "none") {
			if changed {
				parsed[key] = v
			}
			continue
		}
		base := path.Base(filepath.ToSlash(trimmed))
		if base == "" || base == "." || base == ".." || base == "/" {
			continue
		}
		confined := filepath.Join(config.GetLogFolder(), base)
		if confined == trimmed {
			continue
		}
		parsed[key] = confined
		changed = true
	}
	if !changed {
		return logCfg
	}
	out, err := json.Marshal(parsed)
	if err != nil {
		return logCfg
	}
	return out
}

// stripDisabledRules removes routing rules marked `enabled: false` from the
// generated runtime config and strips panel-only keys (`enabled`, `comment`)
// from the rest, since xray-core has no such fields. The internal api rule is
// always kept (see isApiRule) so traffic stats can't be toggled off. The stored
// template is untouched — only the generated config is filtered.
func stripDisabledRules(routerCfg json_util.RawMessage) json_util.RawMessage {
	if len(routerCfg) == 0 {
		return routerCfg
	}
	var parsed map[string]any
	if err := json.Unmarshal(routerCfg, &parsed); err != nil {
		return routerCfg
	}
	rules, ok := parsed["rules"].([]any)
	if !ok || len(rules) == 0 {
		return routerCfg
	}

	var activeRules []any
	changed := false
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			activeRules = append(activeRules, rawRule)
			continue
		}

		if enabledRaw, exists := rule["enabled"]; exists {
			// The internal api rule carries traffic stats and must never be
			// dropped, even if it was somehow marked disabled.
			enabled, ok := enabledRaw.(bool)
			if ok && !enabled && !isApiRule(rule) {
				changed = true
				continue
			}
			delete(rule, "enabled")
			changed = true
		}
		if _, exists := rule["comment"]; exists {
			delete(rule, "comment")
			changed = true
		}
		activeRules = append(activeRules, rule)
	}

	if !changed {
		return routerCfg
	}

	parsed["rules"] = activeRules
	out, err := json.Marshal(parsed)
	if err != nil {
		return routerCfg
	}
	return out
}

// BalancerStatus is the live view of one balancer for the panel UI. Running
// is false when the balancer isn't present in the running core (e.g. xray is
// stopped or the balancer hasn't been saved/applied yet).
type BalancerStatus struct {
	Tag      string   `json:"tag"`
	Running  bool     `json:"running"`
	Override string   `json:"override"`
	Selected []string `json:"selected"`
}

// RestartXray is the legacy method name for restarting the sole owner sing-box.
func (s *CoreService) RestartXray(isForce bool) error {
	coreType, err := (&SettingService{}).GetCoreType()
	if err != nil {
		return err
	}
	if !IsNativeCore(coreType) {
		return fmt.Errorf("legacy database requires offline core-migrate before starting sing-box")
	}
	return SelectedNativeCore().Restart(context.Background())
}

// StopXray stops the running Xray process.
func (s *CoreService) StopXray() error {
	return SelectedNativeCore().Stop(context.Background())
}

// SetToNeedRestart marks that Xray needs to be restarted.
func (s *CoreService) SetToNeedRestart() {
	SelectedNativeCore().SetToNeedRestart()
}

// IsNeedRestartAndSetFalse checks if restart is needed and resets the flag to false.
func (s *CoreService) IsNeedRestartAndSetFalse() bool {
	return hiddifyNeedRestart.CompareAndSwap(true, false)
}

// ApplyPendingRestart consumes the need-restart flag and restarts Xray. If the
// restart fails (for example GetXrayConfig hits a transient DB error and leaves
// the old process running), it re-arms the flag so the next tick retries instead
// of silently dropping the pending config change.
func (s *CoreService) ApplyPendingRestart() {
	if !s.IsNeedRestartAndSetFalse() {
		return
	}
	if err := s.RestartXray(false); err != nil {
		logger.Error("restart xray failed:", err)
		s.SetToNeedRestart()
	}
}

// liftXhttpSessionIDKeys renames the legacy XHTTP session keys
// (sessionPlacement/sessionKey) to the v26.6.22 #6258 names
// (sessionIDPlacement/sessionIDKey) inside a streamSettings map. xray-core kept
// no fallback for the old names, so a config stored before the rename would be
// silently ignored by the engine. Returns true if it changed anything.
func liftXhttpSessionIDKeys(stream map[string]any) bool {
	xhttp, ok := stream["xhttpSettings"].(map[string]any)
	if !ok {
		return false
	}
	changed := false
	for legacy, renamed := range map[string]string{
		"sessionPlacement": "sessionIDPlacement",
		"sessionKey":       "sessionIDKey",
	} {
		v, has := xhttp[legacy]
		if !has {
			continue
		}
		if _, exists := xhttp[renamed]; !exists {
			xhttp[renamed] = v
		}
		delete(xhttp, legacy)
		changed = true
	}
	return changed
}

// liftOutboundsXhttpSessionIDKeys applies liftXhttpSessionIDKeys to every
// outbound's streamSettings in the raw outbounds array. The original bytes are
// returned untouched when nothing needs lifting, so an unchanged config never
// looks modified to the hot-reload diff.
func liftOutboundsXhttpSessionIDKeys(raw json_util.RawMessage) json_util.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var outbounds []map[string]any
	if err := json.Unmarshal(raw, &outbounds); err != nil {
		return raw
	}
	changed := false
	for _, ob := range outbounds {
		if stream, ok := ob["streamSettings"].(map[string]any); ok {
			if liftXhttpSessionIDKeys(stream) {
				changed = true
			}
		}
	}
	if !changed {
		return raw
	}
	if rewritten, err := json.Marshal(outbounds); err == nil {
		return rewritten
	}
	return raw
}
