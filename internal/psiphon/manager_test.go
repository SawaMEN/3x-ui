package psiphon

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestInstanceFromInboundWithGeneratedEntry(t *testing.T) {
	inbound := &model.Inbound{
		Id:       7,
		Tag:      "psiphon-test",
		Port:     443,
		Protocol: model.Psiphon,
		Settings: `{"serverAddress":"203.0.113.10","tunnelProtocol":"OSSH","additionalArguments":["-foo","bar"]}`,
	}
	inst, ok := InstanceFromInbound(inbound)
	if !ok {
		t.Fatal("InstanceFromInbound returned false")
	}
	if inst.ServerAddress != "203.0.113.10" || inst.Protocol != "OSSH" || inst.Port != 443 {
		t.Fatalf("unexpected instance: %#v", inst)
	}
	if len(inst.AdditionalArguments) != 2 {
		t.Fatalf("additional arguments = %#v", inst.AdditionalArguments)
	}
}

func TestInstanceFromInboundAllowsProvidedEntryWithoutPublicAddress(t *testing.T) {
	inbound := &model.Inbound{
		Port:     443,
		Protocol: model.Psiphon,
		Settings: `{"tunnelProtocol":"TLS-OSSH","serverEntry":"existing-entry"}`,
	}
	inst, ok := InstanceFromInbound(inbound)
	if !ok || inst.ServerEntry != "existing-entry" {
		t.Fatalf("expected provided server entry to be sufficient: %#v, %v", inst, ok)
	}
}
