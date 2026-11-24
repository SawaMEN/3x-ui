package externalvpn

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

func TestTrustTunnelListenProtocols(t *testing.T) {
	for _, tc := range []struct {
		settings string
		wantQUIC bool
	}{
		{"{}", true},
		{`{"trustTunnelTcpOnly":false}`, true},
		{`{"trustTunnelTcpOnly":true}`, false},
	} {
		var settings Settings
		if err := json.Unmarshal([]byte(tc.settings), &settings); err != nil {
			t.Fatal(err)
		}
		output := trustTunnelListenProtocols(settings.TrustTunnelTCPOnly)
		if !strings.Contains(output, "[listen_protocols.http1]") || !strings.Contains(output, "[listen_protocols.http2]") ||
			strings.Contains(output, "[listen_protocols.quic]") != tc.wantQUIC {
			t.Fatalf("unexpected listen protocols for %s: %s", tc.settings, output)
		}
	}
}

func TestTrustTunnelSplitPreservesPublicQUIC(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	for _, tc := range []struct {
		listen string
		want   string
	}{
		{"", "0.0.0.0:443"},
		{"0.0.0.0", "0.0.0.0:443"},
		{"::", "[::]:443"},
		{"192.0.2.10", "192.0.2.10:443"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			inst := Instance{
				ID: 7, Protocol: model.TrustTunnel, Listen: tc.listen, Port: 443,
				Settings: Settings{Hostname: "vpn.example.com", Clients: []Client{{Email: "fixture", Password: "fixture-only", Enable: true}}},
			}
			plan, err := planTrustTunnelSplit(inst, 18200, "127.0.0.1:31201", "127.0.0.1:31202")
			if err != nil {
				t.Fatal(err)
			}
			if plan.TCPBind != "127.0.0.1:18200" || plan.QUICBind != tc.want ||
				!strings.Contains(plan.TCPConfig, "[listen_protocols.http2]") ||
				strings.Contains(plan.TCPConfig, "[listen_protocols.quic]") ||
				strings.Contains(plan.QUICConfig, "[listen_protocols.http1]") ||
				strings.Contains(plan.QUICConfig, "[listen_protocols.http2]") ||
				!strings.Contains(plan.QUICConfig, "[listen_protocols.quic]") {
				t.Fatal("split lost original UDP endpoint or mixed transport modes")
			}
			for _, cfg := range []string{plan.TCPConfig, plan.QUICConfig} {
				if !strings.Contains(cfg, "credentials.toml") || !strings.Contains(cfg, tomlString(sidecaregress.Address(inst.ID))) ||
					!strings.Contains(cfg, "allow_private_network_connections = false") || !strings.Contains(cfg, "per_client_metrics = true") ||
					strings.Contains(cfg, "fixture-only") {
					t.Fatal("split changed authentication/egress or leaked secrets")
				}
			}
			if !strings.Contains(plan.TCPConfig, "127.0.0.1:31201") || !strings.Contains(plan.QUICConfig, "127.0.0.1:31202") {
				t.Fatal("metrics endpoints are not independent")
			}
			if _, err := planTrustTunnelSplit(inst, 18200, "127.0.0.1:31201", "127.0.0.1:31201"); err == nil {
				t.Fatal("accepted duplicate metrics sockets")
			}
			if _, err := planTrustTunnelSplit(inst, 18200, "0.0.0.0:31201", "127.0.0.1:31202"); err == nil {
				t.Fatal("accepted public metrics socket")
			}
			if _, err := planTrustTunnelSplit(inst, 443, "127.0.0.1:31201", "127.0.0.1:31202"); err == nil {
				t.Fatal("accepted original public TCP port")
			}
		})
	}
}
