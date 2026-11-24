package externalvpn

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

type trustTunnelListenerMode uint8

const (
	trustTunnelCombined trustTunnelListenerMode = iota
	trustTunnelTCP
	trustTunnelQUIC
)

func trustTunnelProtocolTables(mode trustTunnelListenerMode) string {
	var tables string
	if mode == trustTunnelCombined || mode == trustTunnelTCP {
		tables += "[listen_protocols.http1]\n[listen_protocols.http2]\n"
	}
	if mode == trustTunnelCombined || mode == trustTunnelQUIC {
		tables += "[listen_protocols.quic]\n"
	}
	return tables
}

// Both transport configurations retain the same authentication and egress policy.
func renderTrustTunnelEndpointConfig(bind, credentials, egress, metrics string, mode trustTunnelListenerMode) (string, error) {
	tables := trustTunnelProtocolTables(mode)
	if tables == "" {
		return "", errors.New("invalid TrustTunnel listener mode")
	}
	return fmt.Sprintf("listen_address = %s\ncredentials_file = %s\nallow_private_network_connections = false\n%s[forward_protocol.socks5]\naddress = %s\nextended_auth = false\n[metrics]\naddress = %s\nper_client_metrics = true\n",
		tomlString(bind), tomlString(credentials), tables, tomlString(egress), tomlString(metrics)), nil
}

type trustTunnelSplitConfigs struct {
	TCPBind, QUICBind     string
	TCPConfig, QUICConfig string
	HostsPath             string
}

// Prepare independent endpoint configurations without changing persistent
// state or launching processes. A supervisor must own both endpoints and
// independently account their counters before enabling this migration.
func planTrustTunnelSplit(inst Instance, tcpPort int, tcpMetrics, quicMetrics string) (trustTunnelSplitConfigs, error) {
	if inst.Protocol != model.TrustTunnel || inst.Settings.TrustTunnelTCPOnly || inst.ID <= 0 {
		return trustTunnelSplitConfigs{}, errors.New("split requires an existing combined TrustTunnel inbound")
	}
	if err := inst.Validate(); err != nil {
		return trustTunnelSplitConfigs{}, err
	}
	if tcpPort < 1 || tcpPort > 65535 || tcpPort == inst.Port {
		return trustTunnelSplitConfigs{}, errors.New("split requires a separate valid TCP port")
	}
	quicBind := inst.Bind()
	tcpBind := net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpPort))
	egress := sidecaregress.Address(inst.ID)
	_, egressPort, _ := net.SplitHostPort(egress)
	ports := map[int]bool{}
	for _, address := range []string{tcpMetrics, quicMetrics} {
		host, port, err := net.SplitHostPort(address)
		n, parseErr := strconv.Atoi(port)
		if err != nil || parseErr != nil || n < 1 || n > 65535 || host != "127.0.0.1" ||
			n == tcpPort || n == inst.Port || port == egressPort || ports[n] {
			return trustTunnelSplitConfigs{}, errors.New("split requires distinct loopback metrics endpoints")
		}
		ports[n] = true
	}
	folder := filepath.Join(directory(), strconv.Itoa(inst.ID))
	credentials := filepath.Join(folder, "credentials.toml")
	tcpConfig, err := renderTrustTunnelEndpointConfig(tcpBind, credentials, egress, tcpMetrics, trustTunnelTCP)
	if err != nil {
		return trustTunnelSplitConfigs{}, err
	}
	quicConfig, err := renderTrustTunnelEndpointConfig(quicBind, credentials, egress, quicMetrics, trustTunnelQUIC)
	if err != nil {
		return trustTunnelSplitConfigs{}, err
	}
	return trustTunnelSplitConfigs{TCPBind: tcpBind, QUICBind: quicBind, TCPConfig: tcpConfig, QUICConfig: quicConfig, HostsPath: filepath.Join(folder, "hosts.toml")}, nil
}
