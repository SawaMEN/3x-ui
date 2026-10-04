package service

import (
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"testing"
)

func TestGatewayReservesPanelListenerAndTag(t *testing.T) {
	setupConflictDB(t)
	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"inbounds":` + gatewayNativeInbound + `}`); err != nil {
		t.Fatal(err)
	}
	inbound := &model.Inbound{Enable: true, Port: 52345, Tag: "user", Protocol: model.VLESS}
	conflict, err := checkPortConflictTx(database.GetDB(), inbound, 0)
	if err != nil || conflict == nil || conflict.Tag != "in-tproxy" {
		t.Fatalf("Gateway collision missed: %+v %v", conflict, err)
	}
	node := 12
	inbound.NodeID = &node
	if conflict, err := checkPortConflictTx(database.GetDB(), inbound, 0); err != nil || conflict != nil {
		t.Fatalf("remote listener unnecessarily reserved: %+v %v", conflict, err)
	}
	inbound.NodeID = nil
	inbound.Port = 23456
	inbound.Tag = "in-tproxy"
	if _, err := checkPortConflictTx(database.GetDB(), inbound, 0); err == nil {
		t.Fatal("Gateway tag collision ignored")
	}
	if err := singBoxSettingService.SetSingBoxConfigTemplate(""); err != nil {
		t.Fatal(err)
	}
	inbound.Tag = "user"
	inbound.Port = 52345
	if conflict, err := checkPortConflictTx(database.GetDB(), inbound, 0); err != nil || conflict != nil {
		t.Fatalf("disabled Gateway still reserves port: %+v %v", conflict, err)
	}
}
