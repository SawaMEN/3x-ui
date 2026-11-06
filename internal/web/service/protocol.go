package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/common"
	"github.com/xtls/xray-core/infra/conf"
)

// validateInboundRuntimeTarget validates the protocol against the core that
// will actually host the inbound. Node inbounds are checked against the node's
// persisted core so an Xray master can manage a sing-box node (and vice versa)
// without validating against the wrong engine.
func validateInboundRuntimeTarget(inbound *model.Inbound) error {
	if inbound == nil {
		return nil
	}
	core, err := (&InboundService{}).coreTypeForInbound(inbound)
	if err != nil {
		return err
	}
	if inbound.NodeID != nil && !isNodeEligibleProtocol(inbound.Protocol) {
		return common.NewErrorf("%s inbounds cannot be assigned to a node", inbound.Protocol)
	}
	if !coreSupportsInboundProtocol(core, inbound.Protocol) {
		if inbound.NodeID != nil {
			return common.NewErrorf("%s is not supported by %s on the selected node", inbound.Protocol, core)
		}
		return common.NewErrorf("%s is not supported by %s as the selected core", inbound.Protocol, core)
	}
	if core == CoreTypeXray {
		if err := validateXDNSMaskShape(inbound.StreamSettings); err != nil {
			return err
		}
	}
	if err := validateInboundTransport(core, inbound); err != nil {
		return err
	}
	// Node binaries are checked by the node. Only the local host can inspect
	// the installed engine's version and optional features.
	if inbound.NodeID == nil && inbound.Protocol == model.MASQUE {
		if _, err := NativeCore(core).CachedVersion(context.Background()); err != nil {
			return common.NewErrorf("MASQUE requires an installed sing-box 1.15+: %v", err)
		}
		if !NativeCore(core).process().SupportsMASQUE() {
			return common.NewErrorf("MASQUE requires sing-box 1.15.0-alpha.7 or newer; installed: %s", NativeCore(core).process().GetVersion())
		}
	}
	return nil
}

// Validate a protocol replacement against the inbound's existing host assignment.
// Update payloads cannot redirect validation to an unrelated node.
func validateInboundRuntimeProtocol(protocol model.Protocol, existing *model.Inbound) error {
	if existing == nil {
		return nil
	}
	candidate := *existing
	candidate.Protocol = protocol
	return validateInboundRuntimeTarget(&candidate)
}

// Check the panel transport contract before persisting the row. Native JSON
// templates have a separate binary preflight; "quic" there is not an Xray
// panel streamSettings transport and must not silently become plain TCP here.
func validateInboundTransport(core string, inbound *model.Inbound) error {
	switch inbound.Protocol {
	case model.VLESS, model.VMESS, model.Trojan, model.Shadowsocks:
	default:
		return nil
	}
	var stream struct {
		Network  string `json:"network"`
		Method   string `json:"method"`
		Security string `json:"security"`
	}
	if strings.TrimSpace(inbound.StreamSettings) != "" {
		if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
			return common.NewErrorf("invalid streamSettings: %v", err)
		}
	}
	networkName := stream.Network
	if stream.Method != "" {
		networkName = stream.Method
	}
	network := strings.ToLower(strings.TrimSpace(networkName))
	if network == "" {
		network = "tcp"
	}
	if core == CoreTypeXray {
		switch network {
		case "tcp", "raw", "ws", "grpc", "httpupgrade", "xhttp", "kcp", "hysteria":
			return nil
		case "xdrive":
			if inbound.Protocol == model.VLESS {
				return nil
			}
		}
		return common.NewErrorf("Xray transport %q is removed or unsupported for %s; select a supported transport", network, inbound.Protocol)
	}
	if inbound.Protocol == model.Shadowsocks {
		if (network == "tcp" || network == "raw") && (stream.Security == "" || strings.EqualFold(stream.Security, "none")) {
			return nil
		}
		return common.NewErrorf("%s Shadowsocks requires plain TCP/raw; use a supported explicit wrapper for TLS or another transport", core)
	}
	switch network {
	case "tcp", "raw", "ws", "grpc", "http", "httpupgrade":
		return nil
	case "kcp", "xhttp":
		if core == CoreTypeSingBox {
			return nil
		}
	case "xdrive":
		if core == CoreTypeSingBox && inbound.Protocol == model.VLESS {
			return nil
		}
	}
	return common.NewErrorf("%s transport %q is not supported by %s for panel inbounds", inbound.Protocol, network, core)
}

// shadowTLSTarget resolves the runtime that will host an inbound carrying a
// ShadowTLS transport wrapper. Update payloads cannot be trusted for NodeID, so
// recover the stored assignment before checking the target core.
func shadowTLSTarget(inbound *model.Inbound) (*model.Inbound, error) {
	if inbound == nil || inbound.Id == 0 {
		return inbound, nil
	}
	stored, err := (&InboundService{}).GetInbound(inbound.Id)
	if err != nil {
		return nil, err
	}
	candidate := *stored
	candidate.Protocol = inbound.Protocol
	return &candidate, nil
}

func validateShadowTLSTransport(inbound *model.Inbound) error {
	if inbound == nil {
		return nil
	}
	if model.ShadowTLSTransport(inbound.Settings) == nil {
		return nil
	}

	// ShadowTLS as a wrapper is sing-box-only even when its inner protocol
	// (VLESS/VMess/Trojan/Shadowsocks/etc.) is also valid in Xray. Resolve the
	// actual target separately or an Xray master/node can accept a row that its
	// generated runtime config can never represent.
	target, err := shadowTLSTarget(inbound)
	if err != nil {
		return err
	}
	core, err := (&InboundService{}).coreTypeForInbound(target)
	if err != nil {
		return err
	}
	if !IsNativeCore(core) {
		if target != nil && target.NodeID != nil {
			return common.NewError("ShadowTLS transport requires sing-box on the selected node")
		}
		return common.NewError("ShadowTLS transport requires sing-box as the selected core")
	}

	if !model.SupportsShadowTLSTransport(inbound.Protocol) {
		return common.NewErrorf("ShadowTLS cannot wrap %s", inbound.Protocol)
	}
	if inbound.Protocol == model.Mixed {
		var settings struct {
			UDP bool `json:"udp"`
		}
		_ = json.Unmarshal([]byte(inbound.Settings), &settings)
		if settings.UDP {
			return common.NewError("ShadowTLS only supports TCP; disable Mixed UDP")
		}
	}
	if inbound.Protocol == model.VLESS {
		var settings struct {
			Encryption string `json:"encryption"`
			Decryption string `json:"decryption"`
		}
		_ = json.Unmarshal([]byte(inbound.Settings), &settings)
		if (settings.Encryption != "" && settings.Encryption != "none") || (settings.Decryption != "" && settings.Decryption != "none") {
			return common.NewError("sing-box cannot represent VLESS encryption with ShadowTLS")
		}
	}
	var stream struct {
		Network  string `json:"network"`
		Security string `json:"security"`
	}
	_ = json.Unmarshal([]byte(inbound.StreamSettings), &stream)
	if (stream.Network != "" && stream.Network != "tcp") || (stream.Security != "" && !strings.EqualFold(stream.Security, "none")) {
		return common.NewError("ShadowTLS requires RAW TCP without inner TLS or REALITY")
	}
	return nil
}

func isXrayManagedProtocol(protocol model.Protocol) bool {
	return protocol != model.VKTurnProxy &&
		protocol != model.MASQUE && protocol != model.Snell && protocol != model.NaiveProxy &&
		protocol != model.AnyTLS &&
		protocol != model.ShadowTLS &&
		protocol != model.MTProto &&
		protocol != model.AmneziaWG &&
		protocol != model.TUIC &&
		protocol != model.Pingtunnel &&
		protocol != model.TrustTunnel && protocol != model.FPTN && protocol != model.VetroffProxy && protocol != model.OpenFlux &&
		protocol != model.Mieru &&
		protocol != model.Sudoku
}

func isVKTurnProxyProtocol(protocol model.Protocol) bool {
	return protocol == model.VKTurnProxy
}

// XDNS changed its wire format in the selected Xray release. Legacy resolver
// strings couple domains to addresses; the new format decouples them. Reject
// rather than silently changing that pairing or dropping explicit RR types.
func validateXDNSMaskShape(streamSettings string) error {
	if strings.TrimSpace(streamSettings) == "" {
		return nil
	}
	var stream struct {
		FinalMask struct {
			UDP []struct {
				Type     string          `json:"type"`
				Settings json.RawMessage `json:"settings"`
			} `json:"udp"`
		} `json:"finalmask"`
	}
	if err := json.Unmarshal([]byte(streamSettings), &stream); err != nil {
		return common.NewErrorf("invalid streamSettings: %v", err)
	}
	for _, mask := range stream.FinalMask.UDP {
		if mask.Type != "xdns" {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(mask.Settings, &fields); err == nil {
			if _, exists := fields["domain"]; exists {
				return common.NewError("xDNS legacy domain field is removed; convert both peers to domains [{names, types}] and resolvers [{addrs, worker}]")
			}
		}
		var settings conf.XDNS
		if err := json.Unmarshal(mask.Settings, &settings); err != nil {
			return common.NewErrorf("xDNS requires the current Xray object format: domains [{names, types}] and resolvers [{addrs, worker}]; convert both peers explicitly: %v", err)
		}
	}
	return nil
}
