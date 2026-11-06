package service

import (
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestTelemtWebParseAllTCP443SocketOwners(t *testing.T) {
	output := `LISTEN 0 511 0.0.0.0:443 0.0.0.0:* users:(("nginx",pid=123,fd=6),("nginx",pid=124,fd=6))
LISTEN 0 4096 [::]:443 [::]:* users:(("caddy",pid=444,fd=3),("nginx",pid=125,fd=7))`
	if got := parseTelemtWebPortOwners(output); got != "nginx, caddy" {
		t.Fatalf("owners = %q, want nginx, caddy", got)
	}
	if telemtWebPortOwnedOnlyBy(parseTelemtWebPortOwners(output), "nginx") {
		t.Fatal("mixed frontend owners must not be accepted as an nginx-only socket")
	}
	if !telemtWebPortOwnedOnlyBy("nginx, nginx", "nginx") {
		t.Fatal("duplicate nginx workers should be reusable")
	}
}

func TestTelemtWebInboundTCP443Owner(t *testing.T) {
	t.Run("identifies a configured VLESS inbound", func(t *testing.T) {
		setupConflictDB(t)
		seedInboundConflict(t, "vless-443", "", 443, model.VLESS, "", "{}")
		owner, err := telemtWebInboundPort443Owner(TelemtWebProxyState{})
		if err != nil || !strings.Contains(owner, "vless-443") ||
			!strings.Contains(owner, "vless") || !strings.Contains(owner, "ID ") {
			t.Fatalf("panel inbound owner = %q; err = %v", owner, err)
		}
	})
	t.Run("identifies a disabled inbound as reserved", func(t *testing.T) {
		setupConflictDB(t)
		seedInboundConflict(t, "disabled-443", "", 443, model.VLESS, "", "{}")
		if err := database.GetDB().Model(&model.Inbound{}).Where("tag = ?", "disabled-443").
			Update("enable", false).Error; err != nil {
			t.Fatal(err)
		}
		if owner, err := telemtWebInboundPort443Owner(TelemtWebProxyState{}); err != nil ||
			!strings.Contains(owner, "disabled-443") || !strings.Contains(owner, "отключено") {
			t.Fatalf("disabled reservation was not identified: %q, %v", owner, err)
		}
	})
	t.Run("ignores remote node inbounds", func(t *testing.T) {
		setupConflictDB(t)
		nodeID := 11
		seedInboundConflictNode(t, "remote-443", "", 443, model.VLESS, "", "{}", &nodeID)
		if owner, err := telemtWebInboundPort443Owner(TelemtWebProxyState{}); err != nil || owner != "" {
			t.Fatalf("remote inbound blocked local port: %q, %v", owner, err)
		}
	})
	t.Run("ignores the WEB proxy's own managed inbound", func(t *testing.T) {
		setupConflictDB(t)
		web := `{"webProxy":{"enabled":true,"domain":"web.example.com"}}`
		seedInboundConflict(t, "mtproto-owned", "", 443, model.MTProto, "", web)
		var inbound model.Inbound
		if err := database.GetDB().Where("tag = ?", "mtproto-owned").First(&inbound).Error; err != nil {
			t.Fatal(err)
		}
		state := TelemtWebProxyState{ManagedInboundID: inbound.Id}
		if owner, err := telemtWebInboundPort443Owner(state); err != nil || owner != "" {
			t.Fatalf("own managed WEB inbound blocked its frontend: %q, %v", owner, err)
		}
		if owner, err := telemtWebInboundPort443Owner(TelemtWebProxyState{}); err != nil ||
			!strings.Contains(owner, "mtproto-owned") {
			t.Fatalf("unmanaged WEB inbound was not detected: %q, %v", owner, err)
		}
	})
	t.Run("ignores UDP-only port 443", func(t *testing.T) {
		setupConflictDB(t)
		seedInboundConflict(t, "hysteria-udp", "", 443, model.Hysteria, "", "{}")
		if owner, err := telemtWebInboundPort443Owner(TelemtWebProxyState{}); err != nil || owner != "" {
			t.Fatalf("UDP-only listener blocked TCP 443: %q, %v", owner, err)
		}
	})
}
