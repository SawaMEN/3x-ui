package service

import (
	"context"
	"encoding/json"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
)

func TestTelemtVPNMigrationCandidate(t *testing.T) {
	base := model.Inbound{
		Id: 1, Protocol: model.VLESS, Enable: true, Listen: "0.0.0.0", Port: 443,
		Settings: "{}", StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"vpn.example.com"}}`,
	}
	item, relevant := telemtVPNMigrationCandidate(base, "web.example.com", []string{"web.example.com"})
	if !relevant || item.Reason != "" || item.SNI != "vpn.example.com" || item.Public != "vpn.example.com:443" || item.NewListen != "127.0.0.1" {
		t.Fatalf("unexpected proposal: %+v, relevant=%v", item, relevant)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*model.Inbound)
		reason string
	}{
		{"disabled", func(i *model.Inbound) { i.Enable = false }, "disabled"},
		{"fallback", func(i *model.Inbound) { i.Settings = `{"fallbacks":[{"dest":80}]}` }, "dependent-listener"},
		{"specific address", func(i *model.Inbound) { i.Listen = "192.0.2.1" }, "specific-address"},
		{"native sidecar", func(i *model.Inbound) { i.Protocol = model.Snell }, "unsupported-protocol"},
		{"no TLS", func(i *model.Inbound) { i.StreamSettings = `{"network":"tcp","security":"none"}` }, "tls-sni"},
		{"web SNI", func(i *model.Inbound) {
			i.StreamSettings = `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"web.example.com"}}`
		}, "web-sni"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inbound := base
			tc.mutate(&inbound)
			proposal, ok := telemtVPNMigrationCandidate(inbound, "web.example.com", []string{"web.example.com"})
			if !ok || proposal.Reason != tc.reason {
				t.Fatalf("got %+v, want %q", proposal, tc.reason)
			}
		})
	}
	udp := base
	udp.Protocol = model.Hysteria
	if _, relevant := telemtVPNMigrationCandidate(udp, "web.example.com", nil); relevant {
		t.Fatal("UDP-only listener should not block TCP 443")
	}
}

func TestTelemtVPNMigrationPreviewDoesNotChangeInbounds(t *testing.T) {
	setupConflictDB(t)
	inbound := model.Inbound{
		Protocol: model.VLESS, Enable: true, Tag: "vpn-original", Port: 443, Listen: "0.0.0.0",
		Settings: "{}", StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"vpn.example.com"}}`,
	}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	seedInboundConflict(t, "reserved-migration-port", "127.0.0.1", telemtVPNMigrationFirstPort, model.VLESS, "{}", "{}")
	state := TelemtWebProxyState{Domain: "web.example.com", SharedIngress: &TelemtWebIngress{Enabled: true}}
	items, err := planTelemtVPNMigrations(context.Background(), state)
	if err != nil || len(items) != 1 {
		t.Fatalf("migration plan: %+v %v", items, err)
	}
	if items[0].Reason == "" && (items[0].NewPort <= telemtVPNMigrationFirstPort || items[0].NewPort > telemtVPNMigrationLastPort) {
		t.Fatalf("allocator used reserved or invalid port: %+v", items[0])
	}
	var stored model.Inbound
	if err := database.GetDB().First(&stored, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Port != inbound.Port || stored.Listen != inbound.Listen || stored.Tag != inbound.Tag || stored.Settings != inbound.Settings || stored.StreamSettings != inbound.StreamSettings {
		t.Fatal("preview mutated original inbound")
	}
}

func TestTelemtVPNMigrationAtomicStageAndRestore(t *testing.T) {
	setupConflictDB(t)
	inbound := model.Inbound{
		Protocol: model.VLESS, Enable: true, Tag: "stable-tag", Listen: "0.0.0.0", Port: 443,
		Settings:       `{"clients":[{"email":"preserve","id":"abc"}]}`,
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"vpn.example.com"}}`,
	}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	move, ok := telemtVPNMigrationCandidate(inbound, "web.example.com", nil)
	if !ok || move.Reason != "" {
		t.Fatalf("unexpected candidate: %+v", move)
	}
	move.NewPort = telemtVPNMigrationFirstPort
	stale := move
	stale.Revision = "stale"
	if err := stageTelemtVPNMigration([]telemtVPNMigrationPlan{stale}); err == nil {
		t.Fatal("accepted a stale revision")
	}
	check := func(port int, listen string) {
		t.Helper()
		var stored model.Inbound
		if err := database.GetDB().First(&stored, inbound.Id).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Port != port || stored.Listen != listen || stored.Tag != inbound.Tag || stored.Settings != inbound.Settings || stored.StreamSettings != inbound.StreamSettings {
			t.Fatalf("unexpected socket or changed credentials: %+v", stored)
		}
	}
	check(443, "0.0.0.0")
	if err := stageTelemtVPNMigration([]telemtVPNMigrationPlan{move, stale}); err == nil {
		t.Fatal("accepted a stale second entry")
	}
	check(443, "0.0.0.0")
	if journal, err := loadTelemtVPNMigration(database.GetDB()); err != nil || journal != nil {
		t.Fatalf("failed stage left a journal: %+v %v", journal, err)
	}
	if err := stageTelemtVPNMigration([]telemtVPNMigrationPlan{move}); err != nil {
		t.Fatal(err)
	}
	check(move.NewPort, move.NewListen)
	journal, err := loadTelemtVPNMigration(database.GetDB())
	if err != nil || journal == nil || journal.Committed {
		t.Fatalf("pending journal: %+v %v", journal, err)
	}
	if err := markTelemtVPNMigrationCommitted(true); err != nil {
		t.Fatal(err)
	}
	if err := markTelemtVPNMigrationCommitted(false); err != nil {
		t.Fatal(err)
	}
	if err := restoreTelemtVPNMigrationRows(); err != nil {
		t.Fatal(err)
	}
	check(443, "0.0.0.0")
	if err := restoreTelemtVPNMigrationRows(); err != nil {
		t.Fatal(err)
	}
	if err := clearTelemtVPNMigration(); err != nil {
		t.Fatal(err)
	}
}

func TestTelemtVPNMigrationNativeTLSProtocols(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol model.Protocol
		settings string
		stream   string
		reason   string
	}{
		{"ShadowTLS fixed SNI", model.ShadowTLS, `{"handshake":{"server":"vpn.example.com"},"version":3}`, "", ""},
		{"ShadowTLS wildcard", model.ShadowTLS, `{"handshake":{"server":"vpn.example.com"},"wildcardSni":"all"}`, "", "tls-sni"},
		{"TrustTunnel split bind", model.TrustTunnel, `{"hostname":"vpn.example.com"}`, "", ""},
		{"TrustTunnel TCP only", model.TrustTunnel, `{"hostname":"vpn.example.com","trustTunnelTcpOnly":true}`, "", ""},
		{"AnyTLS", model.AnyTLS, `{"tls":{"enabled":true,"serverName":"vpn.example.com"}}`, "", ""},
		{"Naive TCP", model.NaiveProxy, `{"network":"tcp","tls":{"serverName":"vpn.example.com"}}`, "", ""},
		{"Naive mixed", model.NaiveProxy, `{"tls":{"serverName":"vpn.example.com"}}`, "", "udp-or-mixed"},
		{"MASQUE TCP", model.MASQUE, `{"version":[1,2],"tls":{"enabled":true,"serverName":"vpn.example.com"}}`, "", ""},
		{"MASQUE mixed", model.MASQUE, `{"version":[2,3],"tls":{"enabled":true,"serverName":"vpn.example.com"}}`, "", "udp-or-mixed"},
		{"Shadowsocks TCP TLS", model.Shadowsocks, `{"network":"tcp"}`, `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"vpn.example.com"}}`, ""},
		{"VLESS REALITY", model.VLESS, "{}", `{"network":"tcp","security":"reality","realitySettings":{"serverNames":["vpn.example.com"]}}`, ""},
		{"Trojan REALITY", model.Trojan, "{}", `{"network":"tcp","security":"reality","realitySettings":{"serverNames":["vpn.example.com"]}}`, "reality-verification"},
		{"VetrOFF TCP and H3", model.VetroffProxy, `{"hostname":"vpn.example.com","vetroff":{"http3":true}}`, "", ""},
		{"VetrOFF TCP only", model.VetroffProxy, `{"hostname":"vpn.example.com","vetroff":{"http3":false}}`, "", ""},
		{"VetrOFF H3 transport", model.VetroffProxy, `{"hostname":"vpn.example.com","vetroff":{"http3":true,"transport":"h3-connect"}}`, "", "tls-sni"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inbound := model.Inbound{Id: 1, Protocol: tc.protocol, Enable: true, Port: 443, Listen: "0.0.0.0", Settings: tc.settings, StreamSettings: tc.stream}
			move, relevant := telemtVPNMigrationCandidate(inbound, "web.example.com", nil)
			if !relevant || move.Reason != tc.reason {
				t.Fatalf("candidate = %+v, relevant = %v; want %q", move, relevant, tc.reason)
			}
		})
	}
}

func TestTelemtVPNMigrationListenerOwnership(t *testing.T) {
	paths := map[string]string{"100": "/opt/x-ui/bin/sing-box", "101": "/opt/x-ui/bin/caddy-naive", "102": "/usr/bin/sing-box"}
	resolve := func(pid string) (string, error) { return paths[pid], nil }
	allowed := []string{paths["100"], paths["101"]}
	for _, tc := range []struct {
		name  string
		lines []string
		ok    bool
	}{
		{"core", []string{`LISTEN users:(("sing-box",pid=100,fd=1))`}, true},
		{"managed naive", []string{`LISTEN users:(("caddy-naive",pid=101,fd=1))`}, true},
		{"foreign same name", []string{`LISTEN users:(("sing-box",pid=102,fd=1))`}, false},
		{"mixed owner", []string{`LISTEN users:(("sing-box",pid=100,fd=1),("sing-box",pid=102,fd=2))`}, false},
		{"unknown", []string{"LISTEN"}, false},
		{"empty", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTelemtVPNListenerOwners(tc.lines, allowed, resolve)
			if (err == nil) != tc.ok {
				t.Fatalf("ownership verification: %v, want success=%v", err, tc.ok)
			}
		})
	}
}

func TestTelemtVetroffH3MigrationPreservesBind(t *testing.T) {
	for _, tc := range []struct {
		name     string
		listen   string
		settings string
		bind     string
		pin      bool
	}{
		{"inherited wildcard", "", `{"hostname":"vpn.example.com","vetroff":{"http3":true,"transport":"h2-connect"},"clients":[{"email":"preserve"}]}`, "0.0.0.0:443", true},
		{"inherited IPv6", "::", `{"hostname":"vpn.example.com","vetroff":{"http3":true,"h3Listen":""}}`, "[::]:443", true},
		{"explicit separate", "0.0.0.0", `{"hostname":"vpn.example.com","vetroff":{"http3":true,"h3Listen":"0.0.0.0:8443"}}`, "0.0.0.0:8443", false},
		{"disabled", "0.0.0.0", `{"hostname":"vpn.example.com","vetroff":{"http3":false}}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inbound := model.Inbound{Id: 1, Protocol: model.VetroffProxy, Enable: true, Port: 443, Listen: tc.listen, Settings: tc.settings}
			move, ok := telemtVPNMigrationCandidate(inbound, "web.example.com", nil)
			if !ok || move.Reason != "" || move.VetroffH3Bind != tc.bind || move.VetroffPinH3 != tc.pin {
				t.Fatalf("unexpected migration: %+v", move)
			}
			updated, err := patchTelemtVetroffH3(inbound.Settings, move, false)
			if err != nil {
				t.Fatal(err)
			}
			inbound.Settings, inbound.Listen, inbound.Port = updated, "127.0.0.1", 18200
			if h3 := vetroffH3Listener(&inbound); tc.bind != "" && (h3 == nil || net.JoinHostPort(h3.Listen, strconv.Itoa(h3.Port)) != tc.bind) {
				t.Fatalf("HTTP/3 bind was changed: %+v", h3)
			}
			restored, err := patchTelemtVetroffH3(updated, move, true)
			if err != nil {
				t.Fatal(err)
			}
			var original, result any
			if err := json.Unmarshal([]byte(tc.settings), &original); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(restored), &result); err != nil || !reflect.DeepEqual(original, result) {
				t.Fatalf("settings were not restored: %s", restored)
			}
		})
	}
}

func TestTelemtVetroffMigrationJournalRestoresInheritedH3(t *testing.T) {
	setupConflictDB(t)
	inbound := model.Inbound{
		Protocol: model.VetroffProxy, Enable: true, Tag: "vetroff-h3-migration",
		Port: 443, Listen: "0.0.0.0",
		Settings: `{"hostname":"vpn.example.com","vetroff":{"http3":true,"h3Listen":"","transport":"h2-connect"},"clients":[{"email":"original","password":"not-a-real-credential"}]}`,
	}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	move, ok := telemtVPNMigrationCandidate(inbound, "web.example.com", nil)
	if !ok || move.Reason != "" || !move.VetroffPinH3 {
		t.Fatalf("unexpected migration plan: %+v", move)
	}
	move.NewPort = telemtVPNMigrationFirstPort
	if err := stageTelemtVPNMigration([]telemtVPNMigrationPlan{move}); err != nil {
		t.Fatal(err)
	}
	var stored model.Inbound
	if err := database.GetDB().First(&stored, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if h3 := vetroffH3Listener(&stored); h3 == nil || h3.Listen != "0.0.0.0" || h3.Port != 443 || stored.Port != move.NewPort || stored.Listen != move.NewListen {
		t.Fatalf("TCP/UDP migration mismatch: %+v", stored)
	}
	journal, err := loadTelemtVPNMigration(database.GetDB())
	if err != nil || journal == nil || !journal.Moves[0].VetroffPinH3 {
		t.Fatalf("missing HTTP/3 recovery journal: %+v %v", journal, err)
	}
	raw, err := json.Marshal(journal)
	if err != nil || strings.Contains(string(raw), "not-a-real-credential") {
		t.Fatal("journal contains client settings")
	}
	if err := restoreTelemtVPNMigrationRows(); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().First(&stored, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	var original, restored any
	if err := json.Unmarshal([]byte(inbound.Settings), &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(stored.Settings), &restored); err != nil || !reflect.DeepEqual(original, restored) || stored.Port != inbound.Port || stored.Listen != inbound.Listen {
		t.Fatalf("failed to restore original VetrOFF configuration: %+v", stored)
	}
}

func TestTelemtRealityMigrationProbePreservesSecurity(t *testing.T) {
	profile := map[string]any{
		"type": "vless", "tag": "original", "uuid": "test-identity", "server": "public.example.com", "server_port": 443, "flow": "xtls-rprx-vision",
		"tls": map[string]any{"enabled": true, "server_name": "vpn.example.com", "reality": map[string]any{"enabled": true, "public_key": "test-key", "short_id": "abcd"}},
	}
	if err := validateTelemtRealityProbeProfile(profile, "vpn.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := validateTelemtRealityProbeProfile(profile, "different.example.com"); err == nil {
		t.Fatal("accepted mismatched SNI")
	}
	raw, err := telemtRealityProbeOutbound(profile, "127.0.0.1", 18200)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	settings := out[0]["settings"].(map[string]any)
	if settings["server"] != "127.0.0.1" || settings["server_port"] != float64(18200) || settings["flow"] != profile["flow"] || !reflect.DeepEqual(settings["tls"], profile["tls"]) || profile["server"] != "public.example.com" {
		t.Fatal("probe mutated original security settings or selected the wrong endpoint")
	}
	if _, err := telemtRealityProbeOutbound(profile, "192.0.2.1", 443); err == nil {
		t.Fatal("accepted a non-loopback probe destination")
	}
}

func TestTelemtRealityOriginalProbeHost(t *testing.T) {
	for _, tc := range []struct {
		listen string
		want   string
	}{
		{"", "127.0.0.1"},
		{"0.0.0.0", "127.0.0.1"},
		{"::", "::1"},
		{"::0", "::1"},
		{"127.0.0.2", "127.0.0.2"},
		{"::1", "::1"},
	} {
		if got := telemtRealityOriginalProbeHost(tc.listen); got != tc.want {
			t.Errorf("probe host for %q = %q; want %q", tc.listen, got, tc.want)
		}
	}
}

func TestTelemtRealityProbeRejectsMissingIdentity(t *testing.T) {
	profile := map[string]any{
		"type": "vless", "uuid": "test-identity",
		"tls": map[string]any{
			"enabled": true, "server_name": "VPN.Example.com",
			"reality": map[string]any{"enabled": true, "public_key": "test-key"},
		},
	}
	if err := validateTelemtRealityProbeProfile(profile, "vpn.example.com"); err != nil {
		t.Fatal("DNS names should compare case-insensitively:", err)
	}
	for _, identity := range []any{nil, "", " ", 42} {
		profile["uuid"] = identity
		if err := validateTelemtRealityProbeProfile(profile, "vpn.example.com"); err == nil {
			t.Errorf("accepted invalid identity of type %T", identity)
		}
	}
	profile["uuid"] = "test-identity"
	profile["tls"].(map[string]any)["reality"].(map[string]any)["public_key"] = " "
	if err := validateTelemtRealityProbeProfile(profile, "vpn.example.com"); err == nil {
		t.Fatal("accepted a blank public key")
	}
}

func TestTelemtShadowTLSMigrationProbeChain(t *testing.T) {
	profile := telemtShadowTLSProbe{SNI: "vpn.example.com", OuterPassword: "outer-test", InnerPassword: "inner-test"}
	items, all, err := telemtShadowTLSProbeOutbounds(profile, "::1", 18200)
	if err != nil {
		t.Fatal(err)
	}
	var roots, chain []map[string]any
	if err := json.Unmarshal(items, &roots); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(all, &chain); err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || len(chain) != 2 || !reflect.DeepEqual(roots[0], chain[0]) {
		t.Fatal("probe must test the authenticated inner proxy only")
	}
	inner := chain[0]["settings"].(map[string]any)
	outer := chain[1]["settings"].(map[string]any)
	tls := outer["tls"].(map[string]any)
	if inner["detour"] != chain[1]["tag"] || inner["password"] != profile.InnerPassword || outer["password"] != profile.OuterPassword ||
		outer["server"] != "::1" || outer["server_port"] != float64(18200) || tls["server_name"] != profile.SNI ||
		chain[0]["protocol"] != "singbox:shadowsocks" || chain[1]["protocol"] != "singbox:shadowtls" {
		t.Fatal("probe lost the authentication layers or loopback destination")
	}
	if _, _, err := telemtShadowTLSProbeOutbounds(profile, "192.0.2.1", 443); err == nil {
		t.Fatal("accepted a public probe destination")
	}
}

func TestTelemtTrustTunnelNoUDP(t *testing.T) {
	paths := map[string]string{"100": "/opt/x-ui/bin/trusttunnel_endpoint", "101": "/opt/x-ui/bin/sing-box"}
	resolve := func(pid string) (string, error) { return paths[pid], nil }
	for _, tc := range []struct {
		output string
		ok     bool
	}{
		{"", true},
		{`UNCONN users:(("sing-box",pid=101,fd=1))`, true},
		{`UNCONN users:(("trusttunnel_endpoint",pid=100,fd=1))`, false},
		{"UNCONN", false},
	} {
		if err := validateTelemtTrustTunnelNoUDP(tc.output, resolve); (err == nil) != tc.ok {
			t.Fatalf("unexpected UDP check result: %v, want %v", err, tc.ok)
		}
	}
}

func TestTelemtTrustTunnelSplitJournalAndPortReservation(t *testing.T) {
	setupConflictDB(t)
	inbound := model.Inbound{
		Protocol: model.TrustTunnel, Enable: true, Tag: "trusttunnel-split",
		Listen: "0.0.0.0", Port: 443,
		Settings: `{"hostname":"vpn.example.com","clients":[{"email":"fixture","password":"test-secret","enable":true}]}`,
	}
	db := database.GetDB()
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	move, ok := telemtVPNMigrationCandidate(inbound, "web.example.com", nil)
	if !ok || move.Reason != "" || move.TrustTunnelQUICBind != "0.0.0.0:443" {
		t.Fatalf("unexpected split plan: %+v", move)
	}
	move.NewPort = telemtVPNMigrationFirstPort
	if err := stageTelemtVPNMigration([]telemtVPNMigrationPlan{move}); err != nil {
		t.Fatal(err)
	}
	var stored model.Inbound
	if err := db.First(&stored, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	inst, err := externalvpn.FromInbound(&stored)
	if err != nil || inst.TrustTunnelQUICBind != move.TrustTunnelQUICBind || inst.Settings.TrustTunnelTCPOnly || inst.Bind() != "127.0.0.1:18200" {
		t.Fatalf("runtime split was not projected from saved settings: %v", err)
	}
	udp := model.Inbound{Protocol: model.Hysteria, Listen: "0.0.0.0", Port: 443}
	if conflict, err := checkPortConflictTx(db, &udp, 0); err != nil || conflict == nil || conflict.InboundID != inbound.Id || conflict.Transports != transportUDP {
		t.Fatalf("UDP 443 reservation missing: %v %v", conflict, err)
	}
	tcp := model.Inbound{Protocol: model.VLESS, Listen: "0.0.0.0", Port: 443}
	if conflict, err := checkPortConflictTx(db, &tcp, 0); err != nil || conflict != nil {
		t.Fatalf("split still reserves public TCP 443: %v %v", conflict, err)
	}
	if !telemtInboundUsesUDP443(&stored) {
		t.Fatal("UDP status lost split reservation")
	}
	journal, err := loadTelemtVPNMigration(db)
	if err != nil || journal == nil || journal.Moves[0].TrustTunnelQUICBind != move.TrustTunnelQUICBind {
		t.Fatalf("missing split recovery journal: %v", err)
	}
	raw, err := json.Marshal(journal)
	if err != nil || strings.Contains(string(raw), "test-secret") {
		t.Fatal("journal exposed client credentials")
	}
	// Older editors omit the runtime-owned key; saving other settings must
	// preserve it rather than reverting to a combined private listener.
	edited := stored
	edited.Settings = inbound.Settings
	if err := prepareExternalVPN(&edited, stored.Settings); err != nil {
		t.Fatal(err)
	}
	if udp := trustTunnelSeparateUDPListener(&edited); udp == nil || udp.Port != 443 {
		t.Fatal("editor dropped the QUIC reservation")
	}
	edited.Settings = `{"hostname":"vpn.example.com","trustTunnelTcpOnly":true}`
	if err := prepareExternalVPN(&edited, stored.Settings); err == nil {
		t.Fatal("editor disabled QUIC while split migration active")
	}
	if err := restoreTelemtVPNMigrationRows(); err != nil {
		t.Fatal(err)
	}
	if err := restoreTelemtVPNMigrationRows(); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&stored, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	var original, restored any
	if err := json.Unmarshal([]byte(inbound.Settings), &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(stored.Settings), &restored); err != nil || !reflect.DeepEqual(original, restored) ||
		stored.Port != 443 || stored.Listen != inbound.Listen || trustTunnelSeparateUDPListener(&stored) != nil {
		t.Fatal("original combined listener was not restored")
	}
}

func TestTelemtTrustTunnelQUICSocketRequiresExactPID(t *testing.T) {
	for _, tc := range []struct {
		output string
		bind   string
		ok     bool
	}{
		{`UNCONN 0 0 0.0.0.0:443 0.0.0.0:* users:(("trusttunnel_endpoint",pid=123,fd=4))`, "0.0.0.0:443", true},
		{`UNCONN 0 0 [::]:443 [::]:* users:(("trusttunnel_endpoint",pid=123,fd=4))`, "[::]:443", true},
		{`UNCONN 0 0 0.0.0.0:443 0.0.0.0:* users:(("trusttunnel_endpoint",pid=124,fd=4))`, "0.0.0.0:443", false},
		{`UNCONN 0 0 127.0.0.1:443 0.0.0.0:* users:(("trusttunnel_endpoint",pid=123,fd=4))`, "0.0.0.0:443", false},
		{"UNCONN 0 0 0.0.0.0:443 0.0.0.0:*", "0.0.0.0:443", false},
		{"", "0.0.0.0:443", false},
	} {
		if err := validateTelemtTrustTunnelQUICSocket(tc.output, tc.bind, 123); (err == nil) != tc.ok {
			t.Fatalf("unexpected QUIC verification result for %q: %v", tc.output, err)
		}
	}
}

func TestTelemtVPNMigrationRejectsDuplicateJournalEntries(t *testing.T) {
	setupConflictDB(t)
	first := telemtVPNMigrationPlan{ID: 1, Protocol: string(model.VLESS), OldPort: 443, NewListen: "127.0.0.1", NewPort: 18200}
	for _, second := range []telemtVPNMigrationPlan{
		{ID: 1, Protocol: string(model.VMESS), OldPort: 443, NewListen: "127.0.0.1", NewPort: 18201},
		{ID: 2, Protocol: string(model.VMESS), OldPort: 443, NewListen: "127.0.0.1", NewPort: 18200},
	} {
		if err := saveTelemtVPNMigration(database.GetDB(), &telemtVPNMigrationJournal{Moves: []telemtVPNMigrationPlan{first, second}}); err != nil {
			t.Fatal(err)
		}
		if _, err := loadTelemtVPNMigration(database.GetDB()); err == nil {
			t.Fatal("duplicate migration journal accepted")
		}
	}
}
