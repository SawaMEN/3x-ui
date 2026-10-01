package service

import (
	"context"
	"sort"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// SingBoxConnectionInfo is the panel-safe view of one live sing-box session.
type SingBoxConnectionInfo struct {
	ID           string   `json:"id"`
	Inbound      string   `json:"inbound,omitempty"`
	InboundType  string   `json:"inboundType,omitempty"`
	Network      string   `json:"network,omitempty"`
	Source       string   `json:"source,omitempty"`
	Destination  string   `json:"destination,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	Protocol     string   `json:"protocol,omitempty"`
	User         string   `json:"user,omitempty"`
	Outbound     string   `json:"outbound,omitempty"`
	OutboundType string   `json:"outboundType,omitempty"`
	Chain        []string `json:"chain,omitempty"`
	CreatedAt    int64    `json:"createdAt,omitempty"`
	Uplink       int64    `json:"uplink,omitempty"`
	Downlink     int64    `json:"downlink,omitempty"`
	UplinkTotal  int64    `json:"uplinkTotal,omitempty"`
	DownlinkTotal int64   `json:"downlinkTotal,omitempty"`
}

func singBoxConnectionMatches(item SingBoxConnectionInfo, resource, tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(resource)) {
	case "user", "client":
		return item.User == tag
	case "inbound":
		return item.Inbound == tag
	case "outbound":
		return item.Outbound == tag || containsString(item.Chain, tag)
	default:
		return item.User == tag || item.Inbound == tag || item.Outbound == tag || containsString(item.Chain, tag)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// Connections returns the current native sing-box sessions, newest first.
func (s *SingBoxService) Connections(ctx context.Context, resource, tag string) ([]SingBoxConnectionInfo, error) {
	api := singbox.NewConnectionAPIClient()
	defer api.Close()

	connections, err := api.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]SingBoxConnectionInfo, 0, len(connections))
	for _, connection := range connections {
		if connection == nil {
			continue
		}
		item := SingBoxConnectionInfo{
			ID:            connection.ID,
			Inbound:       connection.Inbound,
			InboundType:   connection.InboundType,
			Network:       connection.Network,
			Source:        connection.Source,
			Destination:   connection.Destination,
			Domain:        connection.Domain,
			Protocol:      connection.Protocol,
			User:          connection.User,
			Outbound:      connection.Outbound,
			OutboundType:  connection.OutboundType,
			Chain:         append([]string(nil), connection.Chain...),
			CreatedAt:     connection.CreatedAt,
			Uplink:        connection.Uplink,
			Downlink:      connection.Downlink,
			UplinkTotal:   connection.UplinkTotal,
			DownlinkTotal: connection.DownlinkTotal,
		}
		if singBoxConnectionMatches(item, resource, tag) {
			result = append(result, item)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CreatedAt > result[j].CreatedAt
	})
	return result, nil
}

// CloseConnection terminates one live native sing-box connection by ID.
func (s *SingBoxService) CloseConnection(ctx context.Context, id string) error {
	api := singbox.NewConnectionAPIClient()
	defer api.Close()
	return api.CloseConnection(ctx, strings.TrimSpace(id))
}
