package service

import (
	"fmt"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

func TestMieruRelayReservedBeforeCoreRestart(t *testing.T) {
	setupConflictDB(t)
	ib := &model.Inbound{Tag: "mieru-relay", Port: 24000, Protocol: model.Mieru, Settings: "{}"}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	port := sidecaregress.Port(ib.Id)
	svc := &InboundService{}
	for _, candidate := range []*model.Inbound{
		{Protocol: model.VLESS, Port: port},
		{Protocol: model.Mieru, Port: 24001, Settings: fmt.Sprintf(`{"tcpPorts":["%d-%d"]}`, port-1, port+1)},
	} {
		hit, err := svc.checkPortConflict(candidate, 0)
		if err != nil || hit == nil || !hit.Relay || hit.Port != port {
			t.Fatalf("relay collision missed: %v, %v", hit, err)
		}
	}
	self := *ib
	self.Port = port
	if hit, err := svc.checkPortConflict(&self, self.Id); err != nil || hit == nil || !hit.Relay {
		t.Fatalf("self collision missed: %v, %v", hit, err)
	}
	wrapped := &model.Inbound{Id: ib.Id+sidecaregress.Slots, Protocol: model.TrustTunnel, Port: 24002}
	if hit, err := svc.checkPortConflict(wrapped, wrapped.Id); err != nil || hit == nil || !hit.Relay {
		t.Fatalf("wrapped slot collision missed: %v, %v", hit, err)
	}
}

func TestMieruRelayReverseConflict(t *testing.T) {
	setupConflictDB(t)
	ib := &model.Inbound{Tag: "new-mieru", Port: 24000, Protocol: model.Mieru, Settings: "{}"}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	seedInboundConflict(t, "existing-listener", "0.0.0.0", sidecaregress.Port(ib.Id), model.VLESS, "", "{}")
	if hit, err := (&InboundService{}).checkPortConflict(ib, ib.Id); err != nil || hit == nil || !hit.Relay {
		t.Fatalf("reverse collision missed: %v, %v", hit, err)
	}
}
