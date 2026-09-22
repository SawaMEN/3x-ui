package mieru

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestInstanceFromInbound(t *testing.T) {
	inbound := &model.Inbound{
		Id:       42,
		Tag:      "mieru-test",
		Port:     443,
		Listen:   "0.0.0.0",
		Protocol: model.Mieru,
		Settings: `{"protocols":["TCP","UDP"],"additionalPorts":[8443],"mtu":1400,"loggingLevel":"DEBUG","userHintIsMandatory":true,"clients":[{"email":"alice","password":"secret","enable":true},{"email":"disabled","password":"x","enable":false}]}`,
	}
	inst, ok := InstanceFromInbound(inbound)
	if !ok {
		t.Fatal("InstanceFromInbound returned false")
	}
	if len(inst.PortBindings) != 3 {
		t.Fatalf("port bindings = %d, want 3", len(inst.PortBindings))
	}
	if len(inst.Users) != 1 || inst.Users[0].Name != "alice" {
		t.Fatalf("users = %#v, want alice only", inst.Users)
	}
	if inst.MTU != 1400 || inst.LoggingLevel != "DEBUG" || !inst.UserHintIsMandatory {
		t.Fatalf("unexpected settings: %#v", inst)
	}
}

func TestInstanceFromInboundRejectsWithoutUsers(t *testing.T) {
	inbound := &model.Inbound{
		Port:     443,
		Protocol: model.Mieru,
		Settings: `{"protocols":["TCP"],"clients":[]}`,
	}
	if _, ok := InstanceFromInbound(inbound); ok {
		t.Fatal("expected inbound without enabled users to be rejected")
	}
}
