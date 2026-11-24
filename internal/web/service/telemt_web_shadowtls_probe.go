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

// Standalone ShadowTLS carries traffic through an inner Shadowsocks 2022
// detour. A plain TLS handshake only reaches the handshake target and cannot
// establish that either authentication layer works.
type telemtShadowTLSProbe struct {
	SNI           string
	OuterPassword string
	InnerPassword string
}

func buildTelemtShadowTLSMigrationProbe(inbound *model.Inbound, move telemtVPNMigrationPlan) (telemtShadowTLSProbe, error) {
	if inbound == nil || inbound.Protocol != model.ShadowTLS || move.SNI == "" {
		return telemtShadowTLSProbe{}, errors.New("unsupported ShadowTLS profile")
	}
	var settings struct {
		InnerKey string `json:"innerKey"`
		Version  int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil || !model.ValidShadowTLSInnerKey(settings.InnerKey) || (settings.Version != 0 && settings.Version != 3) {
		return telemtShadowTLSProbe{}, errors.New("ShadowTLS migration requires a valid version 3 inner key")
	}
	svc := &InboundService{}
	// ShadowTLS credentials are stored in the inbound settings.
	clients, err := svc.GetClients(inbound)
	if err != nil {
		return telemtShadowTLSProbe{}, errors.New("could not load ShadowTLS clients")
	}
	for _, client := range clients {
		if !client.Enable || strings.TrimSpace(client.Email) == "" || strings.TrimSpace(client.Password) == "" ||
			client.TotalGB > 0 || client.LimitIP > 0 || (client.ExpiryTime > 0 && client.ExpiryTime <= time.Now().UnixMilli()) {
			continue
		}
		return telemtShadowTLSProbe{
			SNI: move.SNI, OuterPassword: client.Password,
			InnerPassword: settings.InnerKey + ":" + model.ShadowTLSClientKey(client.Email, client.Password),
		}, nil
	}
	return telemtShadowTLSProbe{}, fmt.Errorf("inbound %d requires an enabled unlimited ShadowTLS client", move.ID)
}

func telemtShadowTLSProbeOutbounds(profile telemtShadowTLSProbe, host string, port int) ([]byte, []byte, error) {
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() || port < 1 || port > 65535 ||
		profile.SNI == "" || profile.OuterPassword == "" || profile.InnerPassword == "" {
		return nil, nil, errors.New("invalid ShadowTLS migration probe")
	}
	const outerTag = "web-shadowtls-migration-transport"
	outer := map[string]any{
		"protocol": "singbox:shadowtls", "tag": outerTag,
		"settings": map[string]any{
			"server": host, "server_port": port, "version": 3, "password": profile.OuterPassword,
			"tls": map[string]any{"enabled": true, "server_name": profile.SNI},
		},
	}
	// The detour supplies the authenticated stream. Keep the endpoint explicit
	// for the native configuration validator; no direct fallback is added.
	inner := map[string]any{
		"protocol": "singbox:shadowsocks", "tag": "web-shadowtls-migration-probe",
		"settings": map[string]any{
			"server": host, "server_port": port, "method": "2022-blake3-aes-128-gcm",
			"password": profile.InnerPassword, "detour": outerTag,
		},
	}
	items, err := json.Marshal([]map[string]any{inner})
	if err != nil {
		return nil, nil, err
	}
	all, err := json.Marshal([]map[string]any{inner, outer})
	return items, all, err
}

func verifyTelemtShadowTLSMigration(ctx context.Context, profile telemtShadowTLSProbe, host string, port int, coreType string) error {
	items, all, err := telemtShadowTLSProbeOutbounds(profile, host, port)
	if err != nil {
		return err
	}
	probe, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	results, err := (&outbound.OutboundService{}).TestAutomaticOutboundsContext(probe, string(items), "https://www.google.com/generate_204", string(all), "real", coreType)
	if err != nil || len(results) != 1 || results[0] == nil || !results[0].Success || results[0].HTTPStatus != 204 {
		// Never expose the generated client configuration in errors.
		return errors.New("authenticated ShadowTLS HTTPS probe did not return HTTP 204")
	}
	return nil
}
