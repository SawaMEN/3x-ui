package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
)

const (
	telemtVPNMigrationFirstPort = 18200
	telemtVPNMigrationLastPort  = 18399
)

// A read-only proposal; it must never be interpreted as permission to move
// a running listener. The future apply path must revalidate the exact row.
type telemtVPNMigrationPlan struct {
	ID                  int             `json:"id"`
	Name                string          `json:"name"`
	Protocol            string          `json:"protocol"`
	OldListen           string          `json:"oldListen"`
	OldPort             int             `json:"oldPort"`
	NewListen           string          `json:"newListen,omitempty"`
	NewPort             int             `json:"newPort,omitempty"`
	SNI                 string          `json:"sni,omitempty"`
	Public              string          `json:"public,omitempty"`
	Reason              string          `json:"reason,omitempty"`
	Revision            string          `json:"revision,omitempty"`
	Security            string          `json:"security,omitempty"`
	VetroffH3Bind       string          `json:"vetroffH3Bind,omitempty"`
	VetroffPinH3        bool            `json:"vetroffPinH3,omitempty"`
	VetroffOldH3        json.RawMessage `json:"vetroffOldH3,omitempty"`
	TrustTunnelQUICBind string          `json:"trustTunnelQuicBind,omitempty"`
}

// Only supported native TCP listeners, managed Naive and VetrOFF TCP
// profiles can use this workflow. VetrOFF HTTP/3 is preserved independently.
// Other mixed transports, wrappers and dependent fallbacks are excluded.
func telemtVPNMigrationCandidate(inbound model.Inbound, domain string, webHosts []string) (telemtVPNMigrationPlan, bool) {
	item := telemtVPNMigrationPlan{
		ID: inbound.Id, Name: inbound.Remark, Protocol: string(inbound.Protocol),
		OldListen: inbound.Listen, OldPort: inbound.Port,
	}
	if item.Name == "" {
		item.Name = inbound.Tag
	}
	if item.Name == "" {
		item.Name = fmt.Sprintf("#%d", inbound.Id)
	}
	if inbound.NodeID != nil {
		return item, false
	}
	if port, _ := spansOverlap(inboundListenerSpans(&inbound), []listenerSpan{{443, 443, transportTCP}}); port == 0 {
		return item, false
	}
	if !inbound.Enable {
		item.Reason = "disabled"
		return item, true
	}
	if inbound.Port != 443 {
		item.Reason = "dependent-listener"
		return item, true
	}
	if inbound.Protocol == model.TrustTunnel && inboundTransports(inbound.Protocol, inbound.StreamSettings, inbound.Settings) != transportTCP {
		host := strings.Trim(inbound.Listen, "[]")
		if host == "" {
			host = "0.0.0.0"
		}
		if net.ParseIP(host) == nil || trustTunnelSeparateUDPListener(&inbound) != nil {
			item.Reason = "trusttunnel-shared-bind"
			return item, true
		}
		item.TrustTunnelQUICBind = net.JoinHostPort(host, strconv.Itoa(inbound.Port))
	}
	if inbound.Protocol != model.VetroffProxy && item.TrustTunnelQUICBind == "" && (inboundTransports(inbound.Protocol, inbound.StreamSettings, inbound.Settings) != transportTCP || telemtInboundUsesUDP443(&inbound)) {
		item.Reason = "udp-or-mixed"
		return item, true
	}
	switch inbound.Protocol {
	case model.VLESS, model.VMESS, model.Trojan, model.Shadowsocks,
		model.AnyTLS, model.MASQUE, model.NaiveProxy, model.VetroffProxy, model.ShadowTLS, model.TrustTunnel:
	default:
		item.Reason = "unsupported-protocol"
		return item, true
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		item.Reason = "invalid-settings"
		return item, true
	}
	if fallback, ok := settings["fallbacks"]; ok && string(fallback) != "null" && string(fallback) != "[]" {
		item.Reason = "dependent-listener"
		return item, true
	}
	if model.ShadowTLSTransport(inbound.Settings) != nil {
		item.Reason = "wrapper"
		return item, true
	}
	// An explicit address other than a wildcard or loopback may distinguish
	// two publicly exposed services. Do not collapse those addresses.
	if !isAnyListen(inbound.Listen) {
		ip := net.ParseIP(inbound.Listen)
		if ip == nil || !ip.IsLoopback() {
			item.Reason = "specific-address"
			return item, true
		}
	}
	candidate := inbound
	if inbound.Protocol == model.VetroffProxy {
		if err := planTelemtVetroffH3(inbound, &item); err != nil {
			item.Reason = "vetroff-h3"
			return item, true
		}
		if item.VetroffPinH3 {
			var err error
			candidate.Settings, err = patchTelemtVetroffH3(inbound.Settings, item, false)
			if err != nil {
				item.Reason = "vetroff-h3"
				return item, true
			}
		}
	}
	candidate.Listen, candidate.Port = "127.0.0.1", telemtVPNMigrationFirstPort
	if item.TrustTunnelQUICBind != "" {
		var err error
		candidate.Settings, err = patchTelemtTrustTunnelBind(inbound.Settings, item, false)
		if err != nil {
			item.Reason = "trusttunnel-shared-bind"
			return item, true
		}
	}
	host, _, address, security, err := telemtIngressProxyEndpoint(&candidate, domain)
	if err != nil {
		item.Reason = "tls-sni"
		return item, true
	}
	item.SNI = host
	item.Security = security
	item.Revision = telemtVPNMigrationRevision(inbound)
	if slices.Contains(webHosts, host) {
		item.Reason = "web-sni"
		return item, true
	}
	item.NewListen = candidate.Listen
	item.Public = net.JoinHostPort(address, "443")
	if security == "reality" && inbound.Protocol != model.VLESS {
		// Limit authenticated migration to the currently supported VLESS
		// client profile. Other REALITY combinations stay manual.
		item.Reason = "reality-verification"
	}
	return item, true
}

func telemtVPNMigrationRevision(inbound model.Inbound) string {
	data, _ := json.Marshal([]any{inbound.Id, inbound.Protocol, inbound.Enable, inbound.NodeID, inbound.Listen, inbound.Port, inbound.Tag, inbound.Settings, inbound.StreamSettings, inbound.ShareAddrStrategy, inbound.ShareAddr})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func planTelemtVPNMigrations(ctx context.Context, state TelemtWebProxyState, reserved ...int) ([]telemtVPNMigrationPlan, error) {
	db := database.GetDB().WithContext(ctx)
	var inbounds []model.Inbound
	if err := db.Where("node_id IS NULL").Order("id").Find(&inbounds).Error; err != nil {
		return nil, err
	}
	ids, err := telemtIngressInboundIDs(state.SharedIngress)
	if err != nil {
		return nil, err
	}
	items := []telemtVPNMigrationPlan{}
	counts := map[string]int{}
	candidates := make(map[int]int)
	candidateIDs := make([]int, 0)
	hosts := telemtWebHosts(state)
	for index, inbound := range inbounds {
		item, relevant := telemtVPNMigrationCandidate(inbound, state.Domain, hosts)
		if relevant {
			if inbound.Protocol == model.MTProto && inbound.Id == state.ManagedInboundID {
				continue
			}
			items = append(items, item)
			if item.Reason == "" {
				counts[item.SNI]++
				candidates[item.ID] = index
				candidateIDs = append(candidateIDs, item.ID)
			}
		} else if slices.Contains(ids, inbound.Id) {
			// The proposed new route cannot reuse an existing selected SNI.
			if host, _, _, _, err := telemtIngressProxyEndpoint(&inbound, state.Domain); err == nil {
				counts[host]++
			}
		}
	}
	// Fetch references in batches; each query uses the ID list twice.
	dependent := make(map[int]bool)
	for _, batch := range chunkInts(candidateIDs, sqlInChunk/2) {
		var references []model.InboundFallback
		if err := db.Select("master_id", "child_id").Where("master_id IN ? OR child_id IN ?", batch, batch).Find(&references).Error; err != nil {
			return nil, err
		}
		for _, reference := range references {
			dependent[reference.MasterId], dependent[reference.ChildId] = true, true
		}
	}
	for i := range items {
		item := &items[i]
		if item.Reason != "" {
			continue
		}
		if dependent[item.ID] {
			item.Reason = "dependent-listener"
			continue
		}
		if item.TrustTunnelQUICBind != "" {
			if err := externalvpn.CheckTrustTunnelQUICOnly(ctx); err != nil {
				item.Reason = "trusttunnel-quic-only-unsupported"
				continue
			}
		}
		if item.Security == "reality" {
			if _, err := buildTelemtRealityMigrationProbe(&inbounds[candidates[item.ID]], *item); err != nil {
				item.Reason = "reality-client-required"
				continue
			}
		}
		if item.Protocol == string(model.ShadowTLS) {
			if _, err := buildTelemtShadowTLSMigrationProbe(&inbounds[candidates[item.ID]], *item); err != nil {
				item.Reason = "shadowtls-client-required"
				continue
			}
		}
		if counts[item.SNI] > 1 {
			item.Reason = "duplicate-sni"
			continue
		}
		port, err := telemtIngressFindPort(ctx, telemtVPNMigrationFirstPort, telemtVPNMigrationLastPort, reserved...)
		if err != nil {
			item.Reason = "port-unavailable"
			continue
		}
		item.NewPort = port
		reserved = append(reserved, port)
	}
	return items, nil
}

// Preserve the old UDP socket even when HTTP/3 inherited the TCP bind.
// Never turn on HTTP/3 for an inbound that did not have it enabled.
func planTelemtVetroffH3(inbound model.Inbound, move *telemtVPNMigrationPlan) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(inbound.Settings), &root); err != nil {
		return err
	}
	var options map[string]json.RawMessage
	if err := json.Unmarshal(root["vetroff"], &options); err != nil || options == nil {
		return fmt.Errorf("explicit VetrOFF settings required")
	}
	var enabled bool
	if err := json.Unmarshal(options["http3"], &enabled); err != nil {
		return fmt.Errorf("explicit VetrOFF HTTP/3 setting required")
	}
	if !enabled {
		return nil
	}
	var bind string
	old, present := options["h3Listen"]
	if present {
		if err := json.Unmarshal(old, &bind); err != nil {
			return err
		}
	}
	if bind == "" {
		host := strings.Trim(inbound.Listen, "[]")
		if host == "" {
			host = "0.0.0.0"
		}
		bind = net.JoinHostPort(host, strconv.Itoa(inbound.Port))
		move.VetroffPinH3 = true
		move.VetroffOldH3 = old
	}
	host, port, err := net.SplitHostPort(bind)
	number, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || number < 1 || number > 65535 || net.ParseIP(host) == nil {
		return fmt.Errorf("invalid explicit VetrOFF HTTP/3 bind")
	}
	move.VetroffH3Bind = bind
	return nil
}

// Change only the nested H3 field. RawMessage preserves unrelated settings
// and client values, and the journal never contains client credentials.
func patchTelemtVetroffH3(settings string, move telemtVPNMigrationPlan, restore bool) (string, error) {
	if !move.VetroffPinH3 {
		return settings, nil
	}
	if move.Protocol != string(model.VetroffProxy) {
		return "", fmt.Errorf("HTTP/3 pinning requires a VetrOFF inbound")
	}
	var root, options map[string]json.RawMessage
	if err := json.Unmarshal([]byte(settings), &root); err != nil {
		return "", err
	}
	if err := json.Unmarshal(root["vetroff"], &options); err != nil || options == nil {
		return "", fmt.Errorf("invalid VetrOFF settings")
	}
	if restore {
		var current string
		if err := json.Unmarshal(options["h3Listen"], &current); err != nil || current != move.VetroffH3Bind {
			// A later explicit administrator edit must not be overwritten.
			return settings, nil
		}
		if len(move.VetroffOldH3) == 0 {
			delete(options, "h3Listen")
		} else {
			options["h3Listen"] = move.VetroffOldH3
		}
	} else {
		raw, err := json.Marshal(move.VetroffH3Bind)
		if err != nil {
			return "", err
		}
		options["h3Listen"] = raw
	}
	raw, err := json.Marshal(options)
	if err != nil {
		return "", err
	}
	root["vetroff"] = raw
	raw, err = json.Marshal(root)
	return string(raw), err
}
