package service

import (
	"bytes"
	"context"
	"errors"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service/outbound"
	"gorm.io/gorm"
)

// NodeRouteProbeResult identifies the actual target transport. A TCP dial or
// QUIC version-negotiation response does not authenticate a VPN client.
type NodeRouteProbeResult struct {
	Tag           string `json:"tag"`
	Reachable     bool   `json:"reachable"`
	CheckedTarget bool   `json:"checkedTarget"`
	Transport     string `json:"transport"`
	LatencyMs     int64  `json:"latencyMs,omitempty"`
	Inconclusive  bool   `json:"inconclusive,omitempty"`
	TunnelChecked bool `json:"tunnelChecked,omitempty"`
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

// probeNodeRouteQUIC checks a QUIC listener with a version-negotiation packet.
// Unlike a UDP dial, a matching response proves that packets can return from
// the target UDP port. This is NOT a VPN handshake or an authentication check.
// Some QUIC servers decline version negotiation, so timeout is inconclusive.
func probeNodeRouteQUIC(ctx context.Context, address string, port int) (time.Duration, error) {
	if address == "" || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid UDP probe endpoint")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", net.JoinHostPort(strings.Trim(address, "[]"), strconv.Itoa(port)))
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return 0, err
		}
	}

	// Long-header Initial with an unsupported reserved version. A compliant
	// QUIC server replies with version=0 and the connection IDs swapped.
	packet := make([]byte, 1200)
	packet[0] = 0xc0
	binary.BigEndian.PutUint32(packet[1:5], 0x0a0a0a0a)
	packet[5] = 8
	packet[14] = 8
	if _, err := rand.Read(packet[6:14]); err != nil {
		return 0, err
	}
	if _, err := rand.Read(packet[15:23]); err != nil {
		return 0, err
	}
	start := time.Now()
	if _, err := conn.Write(packet); err != nil {
		return 0, err
	}
	response := make([]byte, 1500)
	for {
		n, err := conn.Read(response)
		if err != nil {
			return 0, err
		}
		if n < 27 || response[0]&0x80 == 0 || binary.BigEndian.Uint32(response[1:5]) != 0 {
			continue
		}
		if response[5] != 8 || !bytes.Equal(response[6:14], packet[15:23]) || response[14] != 8 || !bytes.Equal(response[15:23], packet[6:14]) {
			continue
		}
		return time.Since(start), nil
	}
}


// nodeRouteTUICProbeWrapper uses the same native TUIC settings as the runtime,
// but wraps them in the panel's singbox:<protocol> format for the shared
// outbound HTTP test. Never return these credentials to a web client.
func nodeRouteTUICProbeWrapper(native map[string]any) (map[string]any, error) {
	protocol, _ := native["type"].(string)
	tag, _ := native["tag"].(string)
	if protocol != "tuic" || tag == "" {
		return nil, fmt.Errorf("not a managed TUIC outbound")
	}
	settings := make(map[string]any, len(native))
	for key, value := range native {
		if key != "type" && key != "tag" {
			settings[key] = value
		}
	}
	return map[string]any{
		"protocol": "singbox:tuic",
		"tag":      tag,
		"settings": settings,
	}, nil
}

// For an already provisioned destination, a real HTTP request through the
// authenticated TUIC outbound is a much stronger check than QUIC version
// negotiation. It runs from this panel, not from another ingress node.
// An absent managed identity means the operator has not created a route yet:
// keep the existing transport-only probe for that case.
func probeNodeRouteTUIC(ctx context.Context, inboundID int) (NodeRouteProbeResult, bool, error) {
	var count int64
	if err := database.GetDB().Model(&model.ClientRecord{}).
		Where("email = ?", nodeRouteClientEmail(inboundID)).Count(&count).Error; err != nil {
		return NodeRouteProbeResult{}, false, err
	}
	if count == 0 {
		return NodeRouteProbeResult{}, false, nil
	}
	result := NodeRouteProbeResult{
		Tag: nodeRouteTag(inboundID), CheckedTarget: true,
		Transport: "udp", TunnelChecked: true,
	}
	if err := ctx.Err(); err != nil {
		return result, true, err
	}
	native, err := resolveNodeRouteOutbound(inboundID, "sing-box")
	if err != nil {
		result.Detail = "Не удалось построить исходящее TUIC-подключение: " + err.Error()
		return result, true, nil
	}
	wrapper, err := nodeRouteTUICProbeWrapper(native)
	if err != nil {
		result.Detail = "Некорректная конфигурация исходящего TUIC: " + err.Error()
		return result, true, nil
	}
	encoded, err := json.Marshal(wrapper)
	if err != nil {
		return result, true, err
	}
	probe, err := (&outbound.OutboundService{CoreType: "sing-box"}).
		TestOutbound(string(encoded), "", "", "http")
	if err != nil {
		result.Detail = "Проверка TUIC-трафика завершилась ошибкой: " + err.Error()
		return result, true, nil
	}
	if probe == nil {
		result.Detail = "Проверка TUIC-трафика не вернула результат"
		return result, true, nil
	}
	result.Reachable = probe.Success
	result.LatencyMs = probe.Delay
	if probe.Success {
		result.Detail = "HTTP-трафик через аутентифицированное TUIC-подключение проходит. Проверка выполнена с сервера панели, а не с другой входящей ноды."
	} else {
		result.Detail = "HTTP-трафик через TUIC не проходит. Проверьте TLS/SNI, учётные данные, UDP-порт и исходящий доступ зарубежной ноды: " + probe.Error
	}
	return result, true, nil
}

// ProbeNodeRoute actively checks the destination port from the panel server.
// QUIC UDP probes require a matching version-negotiation response; a timeout
// cannot be interpreted as proof that the service is unreachable.
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
	result.CheckedTarget = true
	if inbound.Protocol == model.TUIC {
		verified, attempted, probeErr := probeNodeRouteTUIC(ctx, inboundID)
		if probeErr != nil && !errors.Is(probeErr, gorm.ErrRecordNotFound) {
			return NodeRouteProbeResult{}, probeErr
		}
		if attempted {
			return verified, nil
		}
	}
	var duration time.Duration
	if inbound.Protocol == model.Mieru {
		return probeNodeRouteMieru(ctx, inbound, node.Address), nil
	} else if nodeRouteIsUDP(inbound) {
		result.Transport = "udp"
		duration, err = probeNodeRouteQUIC(ctx, node.Address, inbound.Port)
		if err != nil {
			result.Detail = "UDP/QUIC-ответ не получен: доступность UDP-порта не подтверждена (сервер может не отвечать на version negotiation). " + err.Error()
			return result, nil
		}
		result.Inconclusive = true
		result.Detail = "Получен ответ QUIC от UDP-порта. Авторизация и передача VPN-трафика не проверялись. Создайте маршрут и повторите проверку, чтобы проверить реальный TUIC-трафик."
	} else {
		duration, err = probeNodeRouteTCP(ctx, node.Address, inbound.Port)
		if err != nil {
			result.Detail = "Не удалось установить TCP-соединение с портом входящего подключения: " + err.Error()
			return result, nil
		}
		result.Detail = "TCP-порт входящего подключения отвечает. VPN-авторизация и передача трафика не проверялись."
	}
	result.Reachable = true
	result.LatencyMs = duration.Milliseconds()
	if result.LatencyMs == 0 {
		result.LatencyMs = 1
	}
	return result, nil
}
