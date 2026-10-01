package sub

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestHostAppliesToInboundNodeScope(t *testing.T) {
	nodeID := 7
	s := &SubService{nodesByID: map[int]*model.Node{
		nodeID: {Id: nodeID, Guid: "node-fi", Address: "fi.example.com"},
	}}

	cases := []struct {
		name    string
		host    *model.Host
		inbound *model.Inbound
		want    bool
	}{
		{
			name:    "empty scope matches local",
			host:    &model.Host{},
			inbound: &model.Inbound{},
			want:    true,
		},
		{
			name:    "explicit scope rejects local",
			host:    &model.Host{NodeGuids: []string{"node-fi"}},
			inbound: &model.Inbound{},
			want:    false,
		},
		{
			name:    "direct node matches guid",
			host:    &model.Host{NodeGuids: []string{"node-fi"}},
			inbound: &model.Inbound{NodeID: &nodeID},
			want:    true,
		},
		{
			name:    "direct node rejects other guid",
			host:    &model.Host{NodeGuids: []string{"node-de"}},
			inbound: &model.Inbound{NodeID: &nodeID},
			want:    false,
		},
		{
			name:    "origin guid wins for transitive inbound",
			host:    &model.Host{NodeGuids: []string{"node-child"}},
			inbound: &model.Inbound{NodeID: &nodeID, OriginNodeGuid: "node-child"},
			want:    true,
		},
		{
			name:    "scope trims configured guids",
			host:    &model.Host{NodeGuids: []string{" node-fi "}},
			inbound: &model.Inbound{NodeID: &nodeID},
			want:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.hostAppliesToInbound(tc.host, tc.inbound); got != tc.want {
				t.Fatalf("hostAppliesToInbound() = %v, want %v", got, tc.want)
			}
		})
	}
}
