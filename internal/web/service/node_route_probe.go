package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// NodeRouteProbeResult distinguishes the target TCP listener from the node's
// control-plane TCP port. A successful dial does not authenticate a VPN client.
type NodeRouteProbeResult struct {
	Tag           string `json:"tag"`
	Reachable     bool   `json:"reachable"`
	CheckedTarget bool   `json:"checkedTarget"`
	Transport     string `json:"transport"`
	LatencyMs     int64  `json:"latencyMs,omitempty"`
	Detail        string `json:"detail"`
}

func nodeRouteIsUDP(inbound *model.Inbound) bool {
	switch inbound.Protocol {
	case model.Hysteria, model.TUIC:
		return true
	case model.VetroffProxy:
		var settings struct {
			Vetroff struct {
				Transport string `json:"transport"`
			} `json:"vetroff"`
		}
		if json.Unmarshal([]byte(inbound.Settings), &settings) == nil {
			return strings.HasPrefix(settings.Vetroff.Transport, "h3-")
		}
	}
	return false
}

func probeNodeRouteTCP(ctx context.Context, address string, port int) (time.Duration, error) {
	if address == "" || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid node probe endpoint")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	dialer := &net.Dialer{}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(strings.Trim(address, "[]"), strconv.Itoa(port)))
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	return time.Since(start), nil
}

// ProbeNodeRoute performs an active network probe from the panel server to a
// configured node destination, without exposing its stored API credentials.
// UDP-only protocols probe the node API's TCP port, clearly not the VPN port.
func ProbeNodeRoute(ctx context.Context, userID, inboundID int) (NodeRouteProbeResult, error) {
	inbound, node, err := nodeRouteSource(inboundID)
	if err != nil {
		return NodeRouteProbeResult{}, err
	}
	if inbound.UserId != userID {
		return NodeRouteProbeResult{}, fmt.Errorf("inbound is not available to this user")
	}
	result := NodeRouteProbeResult{
		Tag: nodeRouteTag(inboundID), CheckedTarget: true, Transport: "tcp",
	}
	port := inbound.Port
	if nodeRouteIsUDP(inbound) {
		result.CheckedTarget = false
		result.Transport = "udp"
		port = node.Port
	}
	duration, err := probeNodeRouteTCP(ctx, node.Address, port)
	if err != nil {
		if result.CheckedTarget {
			result.Detail = "Не удалось установить TCP-соединение с портом входящего подключения: " + err.Error()
		} else {
			result.Detail = "Не удалось установить TCP-соединение с API ноды; доступность UDP-подключения не определена: " + err.Error()
		}
		return result, nil
	}
	result.Reachable = true
	result.LatencyMs = duration.Milliseconds()
	if result.LatencyMs == 0 {
		result.LatencyMs = 1
	}
	if result.CheckedTarget {
		result.Detail = "TCP-порт входящего подключения отвечает. VPN-авторизация и передача трафика не проверялись."
	} else {
		result.Detail = "API ноды доступен по TCP. Это не подтверждает доступность UDP-порта или VPN-соединения."
	}
	return result, nil
}
