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

func TestInstanceFromInboundRejectsEntryWithoutServerAddress(t *testing.T) {
	inbound := &model.Inbound{
		Port:     443,
		Protocol: model.Psiphon,
		Settings: `{"tunnelProtocol":"TLS-OSSH","serverEntry":"existing-entry"}`,
	}
	if _, ok := InstanceFromInbound(inbound); ok {
		t.Fatal("serverEntry is client metadata and must not replace the server address")
	}
}

func TestInstanceFingerprintIgnoresServerEntry(t *testing.T) {
	base := Instance{ServerAddress:"203.0.113.10", Protocol:"OSSH", Port:443}
	withEntry := base
	withEntry.ServerEntry = "different-client-metadata"
	if base.fingerprint() != withEntry.fingerprint() {
		t.Fatal("server-entry changes must not restart the Psiphon server process")
	}
}
