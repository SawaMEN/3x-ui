package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service/outbound"
)

// Use the existing subscription exporter and native parser rather than
// reimplementing REALITY key derivation or guessing the client's flow.
// Profiles remain in memory; never put a UUID or share link in the journal.
func buildTelemtRealityMigrationProbe(inbound *model.Inbound, move telemtVPNMigrationPlan) (map[string]any, error) {
	if inbound == nil || inbound.Protocol != model.VLESS || move.Security != "reality" {
		return nil, errors.New("unsupported REALITY probe profile")
	}
	svc := &InboundService{}
	// The normalized client table is authoritative for VLESS. Embedded
	// settings can contain stale identities, limits or enabled flags.
	clients, err := svc.ListClientsForInbound(inbound.Id)
	if err == nil && len(clients) == 0 {
		clients, err = svc.GetClients(inbound)
	}
	if err != nil {
		return nil, errors.New("could not load REALITY probe clients")
	}
	for _, client := range clients {
		// Do not use a limited or expired client's allowance for setup checks.
		if !client.Enable || client.Email == "" || client.ID == "" || client.TotalGB > 0 || client.LimitIP > 0 ||
			(client.ExpiryTime > 0 && client.ExpiryTime <= time.Now().UnixMilli()) || client.Reverse != nil {
			continue
		}
		// Link generation can project the inbound through a fallback master.
		// Keep the original row immutable when inspecting multiple clients.
		candidate := *inbound
		links, err := svc.getNodeRouteLinks("127.0.0.1", &candidate, client.Email)
		if err != nil {
			continue
		}
		for _, raw := range links {
			profile, err := ParseNativeOutboundShareLink(raw)
			if err == nil && validateTelemtRealityProbeProfile(profile, move.SNI) == nil {
				return profile, nil
			}
		}
	}
	return nil, fmt.Errorf("inbound %d requires an enabled, unlimited VLESS REALITY client with a compatible profile", move.ID)
}

func validateTelemtRealityProbeProfile(profile map[string]any, sni string) error {
	tls, _ := profile["tls"].(map[string]any)
	reality, _ := tls["reality"].(map[string]any)
	key, _ := reality["public_key"].(string)
	uuid, _ := profile["uuid"].(string)
	serverName, _ := tls["server_name"].(string)
	if profile["type"] != "vless" || strings.TrimSpace(uuid) == "" || tls["enabled"] != true ||
		!strings.EqualFold(serverName, sni) || sni == "" || reality["enabled"] != true || strings.TrimSpace(key) == "" {
		return errors.New("not an authenticated VLESS REALITY profile for the selected SNI")
	}
	if detour, ok := profile["detour"]; ok && detour != "" {
		return errors.New("REALITY probe cannot use a chained outbound")
	}
	return nil
}

// A wildcard IPv6 listener may be IPv6-only. Probe its own address family
// rather than assuming that 127.0.0.1 reaches it.
func telemtRealityOriginalProbeHost(listen string) string {
	switch listen {
	case "", "0.0.0.0":
		return "127.0.0.1"
	case "::", "::0":
		return "::1"
	default:
		return listen
	}
}

// A separate native-core process performs a real HTTPS request through the
// existing client protocol. No production route or kernel interface is
// created. Only the dial destination is replaced; SNI, keys and flow remain.
func telemtRealityProbeOutbound(profile map[string]any, host string, port int) ([]byte, error) {
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() || port < 1 || port > 65535 {
		return nil, errors.New("REALITY migration probe requires a loopback endpoint")
	}
	settings := make(map[string]any, len(profile))
	for k, v := range profile {
		if k != "tag" && k != "type" {
			settings[k] = v
		}
	}
	settings["server"] = host
	settings["server_port"] = port
	return json.Marshal([]map[string]any{{
		"protocol": "singbox:vless", "tag": "web-reality-migration-probe", "settings": settings,
	}})
}

func verifyTelemtRealityMigration(ctx context.Context, profile map[string]any, host string, port int, coreType string) error {
	data, err := telemtRealityProbeOutbound(profile, host, port)
	if err != nil {
		return err
	}
	probe, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	const target = "https://www.google.com/generate_204"
	results, err := (&outbound.OutboundService{}).TestAutomaticOutboundsContext(probe, string(data), target, string(data), "real", coreType)
	// Deliberately discard raw process/probe errors, which could contain
	// generated configurations or credentials.
	if err != nil || len(results) != 1 || results[0] == nil || !results[0].Success || results[0].HTTPStatus != 204 {
		return errors.New("authenticated REALITY HTTPS probe did not return HTTP 204")
	}
	return nil
}
