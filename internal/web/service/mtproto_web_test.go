package service

import (
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestValidateMtprotoWebInboundSingleOwner(t *testing.T) {
	setupConflictDB(t)
	settings := `{"webProxy":{"enabled":true,"domain":"web.example.com","frontend":"nginx"},"clients":[{"email":"alice","enable":true,"secret":"00112233445566778899aabbccddeeff"}]}`
	first := &model.Inbound{Protocol: model.MTProto, Enable: true, Settings: settings}
	if err := ValidateMtprotoWebInbound(first); err != nil {
		t.Fatalf("first WEB inbound: %v", err)
	}
	seedInboundConflict(t, "owned-web", "", 46041, model.MTProto, "", settings)
	owner := loadInboundByTag(t, "owned-web")
	owner.Enable = true
	// The fixture is saved enabled by seedInboundConflict.
	contender := &model.Inbound{Protocol: model.MTProto, Enable: true, Settings: settings}
	if err := ValidateMtprotoWebInbound(contender); err == nil {
		t.Fatal("second WEB inbound must not own port 443")
	}
	contender.Id = owner.Id
	if err := ValidateMtprotoWebInbound(contender); err != nil {
		t.Fatalf("editing the WEB owner should be allowed: %v", err)
	}
	contender.Id = 0
	contender.Enable = false
	if err := ValidateMtprotoWebInbound(contender); err != nil {
		t.Fatalf("disabled WEB inbound should remain editable: %v", err)
	}
}

func TestManagedMtprotoWebProfilesUseClientCredentials(t *testing.T) {
	setupConflictDB(t)
	settings := `{"clients":[{"email":"alice","enable":true,"secret":"00112233445566778899aabbccddeeff","totalGB":1024,"limitIp":2},{"email":"bob","enable":true,"secret":"11223344556677889900aabbccddeeff"}]}`
	seedInboundConflict(t, "mt-web-users", "", 46042, model.MTProto, "", settings)
	inbound := loadInboundByTag(t, "mt-web-users")
	base := `[access.users]
xui = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

[[upstreams]]
type = "direct"
`
	state := TelemtWebProxyState{
		Enabled: true, ManagedInboundID: inbound.Id, Domain: "web.example.com",
		Secret:     "0123456789abcdef0123456789abcdef",
		PublicAddr: "203.0.113.10:443", ListenPort: 15080,
	}
	config, err := appendTelemtWebProxyConfig(base, state)
	if err != nil {
		t.Fatal(err)
	}
	alice := mtprotoWebUsername(inbound.Id, "alice")
	bob := mtprotoWebUsername(inbound.Id, "bob")
	for _, required := range []string{alice, bob, `secret_mode = "dd"`, `[access.user_data_quota]`, `[access.user_max_unique_ips]`} {
		if !strings.Contains(config, required) {
			t.Errorf("managed WEB config missing %q", required)
		}
	}
	if strings.Contains(config, `user = "webproxy"`) {
		t.Fatal("managed WEB proxy must not export the legacy shared credential")
	}
	// Generating the config again must not duplicate or preserve deleted WEB users.
	second, err := appendTelemtWebProxyConfig(config, TelemtWebProxyState{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(second, alice+` = "`) {
		t.Fatal("stale WEB account survived after transport was disabled")
	}
}

func TestValidateMtprotoWebCanonicalEndpoint(t *testing.T) {
	setupConflictDB(t)
	ib := &model.Inbound{Protocol: model.MTProto, Enable: true, Port: 46099, Listen: "127.0.0.2", Settings: `{"webProxy":{"enabled":true,"domain":" HTTPS://Web.Example.COM/ "},"clients":[],"custom":"preserve"}`}
	if err := ValidateMtprotoWebInbound(ib); err != nil {
		t.Fatal(err)
	}
	opts, err := mtprotoWebForInbound(ib)
	if err != nil || opts.Domain != "web.example.com" || ib.Port != 443 || ib.Listen != "" {
		t.Fatalf("incorrect effective WEB endpoint: %+v opts=%+v err=%v", ib, opts, err)
	}
	if !strings.Contains(ib.Settings, `"custom":"preserve"`) {
		t.Fatal("unrelated setting lost")
	}
	if strings.Contains(ib.Settings, `"carriers":null`) {
		t.Fatal("absent carriers must not become null: frontend rejects null arrays")
	}
}

func TestCheckPortConflictMtprotoWebEffectivePort(t *testing.T) {
	const web = `{"webProxy":{"enabled":true,"domain":"web.example.com"}}`
	t.Run("existing legacy WEB reserves 443 on every interface", func(t *testing.T) {
		setupConflictDB(t)
		seedInboundConflict(t, "legacy-web", "127.0.0.2", 46099, model.MTProto, "", web)
		for _, tc := range []struct {
			port     int
			protocol model.Protocol
			conflict bool
		}{
			{443, model.VLESS, true}, {46099, model.VLESS, false}, {443, model.Hysteria, false},
		} {
			got, err := (&InboundService{}).checkPortConflict(&model.Inbound{Protocol: tc.protocol, Port: tc.port, Listen: "127.0.0.3"}, 0)
			if err != nil || (got != nil) != tc.conflict {
				t.Fatalf("port=%d protocol=%s conflict=%+v err=%v", tc.port, tc.protocol, got, err)
			}
		}
	})
	t.Run("new WEB conflicts with existing TCP 443", func(t *testing.T) {
		setupConflictDB(t)
		seedInboundConflict(t, "vless-443", "127.0.0.3", 443, model.VLESS, "", "{}")
		got, err := (&InboundService{}).checkPortConflict(&model.Inbound{Protocol: model.MTProto, Port: 46099, Listen: "127.0.0.2", Settings: web}, 0)
		if err != nil || got == nil || got.Port != 443 {
			t.Fatalf("conflict=%+v err=%v", got, err)
		}
	})
}
