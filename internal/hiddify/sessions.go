package hiddify

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// NormalizeIP unifies IPv4-mapped IPv6 and compressed IPv6 addresses for
// presence and per-IP session limits. Hostnames are not client IP identities.
func NormalizeIP(value string) string {
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return ""
	}
	return addr.Unmap().String()
}

// Hiddify exposes authenticated user sessions through the owner's Clash API.
// Its V2Ray API is for byte counters, not sing-box's newer connection service.
func ActiveSessions(ctx context.Context) ([]singbox.ActiveSession, error) {
	connections, err := singbox.NewClashStatsClient().Connections(ctx)
	if err != nil {
		return nil, err
	}
	sessions := make([]singbox.ActiveSession, 0, len(connections))
	for _, connection := range connections {
		if connection.ID == "" {
			continue
		}
		kind, inbound, _ := strings.Cut(connection.Metadata.Type, "/")
		created := int64(0)
		if !connection.Start.IsZero() {
			created = connection.Start.UnixMilli()
		}
		sessions = append(sessions, singbox.ActiveSession{
			ID: connection.ID, Core: "hiddify-core", Inbound: inbound, InboundType: kind,
			User: connection.Metadata.User, Network: connection.Metadata.Network,
			Source:      net.JoinHostPort(NormalizeIP(connection.Metadata.SourceIP), connection.Metadata.SourcePort),
			Destination: net.JoinHostPort(connection.Metadata.DestinationIP, connection.Metadata.DestinationPort),
			Domain:      connection.Metadata.Host, CreatedAt: created,
			Upload: connection.Upload, Download: connection.Download, Chain: connection.Chains,
		})
	}
	return sessions, nil
}

func DisconnectSessions(ctx context.Context, match func(singbox.ActiveSession) bool) (int, error) {
	sessions, err := ActiveSessions(ctx)
	if err != nil {
		return 0, err
	}
	api := singbox.NewClashStatsClient()
	count := 0
	var failures []error
	for _, session := range sessions {
		if !match(session) {
			continue
		}
		if err := api.CloseConnection(ctx, session.ID); err != nil {
			failures = append(failures, err)
		} else {
			count++
		}
	}
	return count, errors.Join(failures...)
}

func DisconnectUsers(ctx context.Context, inbound string, users []string) (int, error) {
	wanted := make(map[string]bool)
	for _, user := range users {
		if user != "" {
			wanted[user] = true
		}
	}
	if len(wanted) == 0 {
		return 0, errors.New("at least one user is required")
	}
	return DisconnectSessions(ctx, func(session singbox.ActiveSession) bool {
		return wanted[session.User] && (inbound == "" || inbound == session.Inbound)
	})
}

func DisconnectInbound(ctx context.Context, inbound string) (int, error) {
	if inbound == "" {
		return 0, errors.New("inbound is required")
	}
	return DisconnectSessions(ctx, func(session singbox.ActiveSession) bool { return session.Inbound == inbound })
}
