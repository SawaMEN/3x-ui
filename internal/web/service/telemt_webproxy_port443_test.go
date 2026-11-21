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
		owner, err := telemtWebInboundPort443Owner(0)
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
		if owner, err := telemtWebInboundPort443Owner(0); err != nil ||
			!strings.Contains(owner, "disabled-443") || !strings.Contains(owner, "отключено") {
			t.Fatalf("disabled reservation was not identified: %q, %v", owner, err)
		}
	})
	t.Run("ignores remote node inbounds", func(t *testing.T) {
		setupConflictDB(t)
		nodeID := 11
		seedInboundConflictNode(t, "remote-443", "", 443, model.VLESS, "", "{}", &nodeID)
		if owner, err := telemtWebInboundPort443Owner(0); err != nil || owner != "" {
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
		if owner, err := telemtWebInboundPort443Owner(inbound.Id); err != nil || owner != "" {
			t.Fatalf("own managed WEB inbound blocked its frontend: %q, %v", owner, err)
		}
		if owner, err := telemtWebInboundPort443Owner(0); err != nil ||
			!strings.Contains(owner, "mtproto-owned") {
			t.Fatalf("unmanaged WEB inbound was not detected: %q, %v", owner, err)
		}
	})
	t.Run("ignores UDP-only port 443", func(t *testing.T) {
		setupConflictDB(t)
		seedInboundConflict(t, "hysteria-udp", "", 443, model.Hysteria, "", "{}")
		if owner, err := telemtWebInboundPort443Owner(0); err != nil || owner != "" {
			t.Fatalf("UDP-only listener blocked TCP 443: %q, %v", owner, err)
		}
	})
}

func TestTelemtWebPort443DuringInboundEdit(t *testing.T) {
	setupConflictDB(t)
	seedInboundConflict(t, "in-443-tcp", "", 443, model.MTProto, "", "{}")
	var edited model.Inbound
	if err := database.GetDB().Where("tag = ?", "in-443-tcp").First(&edited).Error; err != nil {
		t.Fatal(err)
	}

	state := TelemtWebProxyState{}
	if owner, err := telemtWebPanelPort443Owner(state, 0); err != nil || !strings.Contains(owner, "in-443-tcp") {
		t.Fatalf("standalone setup must see the reservation: owner=%q err=%v", owner, err)
	}
	if owner, err := telemtWebPanelPort443Owner(state, edited.Id); err != nil || owner != "" {
		t.Fatalf("the editor must ignore its own reservation before deployment: owner=%q err=%v", owner, err)
	}

	if err := database.GetDB().Model(&edited).Update("protocol", model.VLESS).Error; err != nil {
		t.Fatal(err)
	}
	if owner, err := telemtWebPanelPort443Owner(state, edited.Id); err != nil || !strings.Contains(owner, "vless") {
		t.Fatalf("the editor must still report a non-MTProto reservation: owner=%q err=%v", owner, err)
	}
}

func TestTelemtWebInboundEditDoesNotIgnoreAnExistingManagedOwner(t *testing.T) {
	setupConflictDB(t)
	seedInboundConflict(t, "managed-443", "", 443, model.MTProto, "", `{"webProxy":{"enabled":true}}`)
	seedInboundConflict(t, "another-mtproto", "", 444, model.MTProto, "", "{}")
	var managed, edited model.Inbound
	if err := database.GetDB().Where("tag = ?", "managed-443").First(&managed).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Where("tag = ?", "another-mtproto").First(&edited).Error; err != nil {
		t.Fatal(err)
	}

	state := TelemtWebProxyState{ManagedInboundID: managed.Id}
	if owner, err := telemtWebPanelPort443Owner(state, edited.Id); err != nil || !strings.Contains(owner, "managed-443") {
		t.Fatalf("another inbound's WEB reservation must still conflict: owner=%q err=%v", owner, err)
	}
}

func TestTelemtWebPort443HonorsPanelEnvironmentOverride(t *testing.T) {
	setupConflictDB(t)
	s := &SettingService{}
	if err := s.setString("webPort", "2053"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XUI_PORT", "443")
	owner, err := telemtWebPanelPort443Owner(TelemtWebProxyState{}, 0)
	if err != nil || !strings.Contains(owner, "XUI_PORT=443") {
		t.Fatalf("effective panel listener was missed: owner=%q err=%v", owner, err)
	}
}

func TestTelemtWebPanelPort443ReservesDomainFront(t *testing.T) {
	setupConflictDB(t)
	settings := &SettingService{}
	config := `{"enabled":true,"listen":"0.0.0.0","httpPort":80,"httpsPort":443,"publicPort":443,"domains":[{"name":"vpn.example.com","mode":"sni_only","enabled":true}]}`
	if err := settings.setString("domainFrontConfig", config); err != nil {
		t.Fatal(err)
	}
	owner, err := telemtWebPanelPort443Owner(TelemtWebProxyState{}, 0)
	if err != nil || !strings.Contains(owner, "Domain Front HTTPS") {
		t.Fatalf("Telemt must detect the saved domain gateway reservation: owner=%q err=%v", owner, err)
	}
}
