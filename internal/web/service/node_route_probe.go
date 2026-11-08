package service

import (
	"bytes"
	"context"
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
	TunnelChecked bool   `json:"tunnelChecked,omitempty"`
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

// nodeRouteNativeProbeWrapper converts a generated native connection into the
// same panel outbound shape used by ordinary authenticated HTTP probes. Its
// credentials stay server-side and are never included in the probe response.
func nodeRouteNativeProbeWrapper(native map[string]any) (map[string]any, bool) {
	kind, _ := native["type"].(string)
	tag, _ := native["tag"].(string)
	if tag == "" {
		return nil, false
	}
	switch kind {
	case "vless", "vmess", "trojan", "shadowsocks", "hysteria",
		"tuic", "anytls", "naive", "snell", "shadowtls", "hysteria2":
	default:
		// Additional VPN sidecars require their own protocol-specific test.
		return nil, false
	}
	settings := make(map[string]any, len(native))
	for key, value := range native {
		if key != "type" && key != "tag" {
			settings[key] = value
		}
	}
	return map[string]any{
		"protocol": "singbox:" + kind,
		"tag":      tag,
		"settings": settings,
	}, true
}

// probeNodeRouteTunnel actually sends HTTP traffic through the generated
// service-client outbound. A reachable port is never reported as a working
// tunnel. Only provisioned, supported protocols use the authenticated probe;
// other protocols continue to use transport-level diagnostics.
func probeNodeRouteTunnel(ctx context.Context, inbound *model.Inbound) (NodeRouteProbeResult, bool, error) {
	var count int64
	if err := database.GetDB().Model(&model.ClientRecord{}).
		Where("email = ?", nodeRouteClientEmail(inbound.Id)).Count(&count).Error; err != nil {
		return NodeRouteProbeResult{}, false, err
	}
	if count == 0 {
		return NodeRouteProbeResult{}, false, nil
	}
	core, err := (&SettingService{}).GetCoreType()
	if err != nil {
		return NodeRouteProbeResult{}, false, err
	}
	if core == "" {
		core = CoreTypeSingBox
	}
	result := NodeRouteProbeResult{
		Tag: nodeRouteTag(inbound.Id), CheckedTarget: true, Transport: "tcp",
	}
	if nodeRouteIsUDP(inbound) {
		result.Transport = "udp"
	}
	if err := ctx.Err(); err != nil {
		return result, true, err
	}
	native, err := resolveNodeRouteOutbound(inbound.Id, core)
	if err != nil {
		result.TunnelChecked = true
		result.Detail = "Не удалось подготовить исходящее подключение каскада: " + err.Error()
		return result, true, nil
	}
	var probeConfig map[string]any
	if core == CoreTypeXray {
		probeConfig = native
	} else {
		var supported bool
		probeConfig, supported = nodeRouteNativeProbeWrapper(native)
		if !supported {
			return result, false, nil
		}
	}
	result.TunnelChecked = true
	encoded, err := json.Marshal(probeConfig)
	if err != nil {
		return result, true, err
	}
	probe, err := (&outbound.OutboundService{CoreType: core}).
		TestOutbound(string(encoded), "", "", "http")
	if err != nil {
		result.Detail = "Проверка трафика через каскад завершилась ошибкой: " + err.Error()
		return result, true, nil
	}
	if probe == nil {
		result.Detail = "Проверка каскада не вернула результат"
		return result, true, nil
	}
	result.Reachable = probe.Success
	result.LatencyMs = probe.Delay
	if probe.Success {
		result.Detail = "HTTP-запрос через аутентифицированное исходящее подключение успешен. Проверка выполнена с сервера панели."
	} else {
		result.Detail = "Трафик через исходящее подключение не проходит: " + probe.Error
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
	verified, attempted, probeErr := probeNodeRouteTunnel(ctx, inbound)
	if probeErr != nil {
		return NodeRouteProbeResult{}, probeErr
	}
	if attempted {
		return verified, nil
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
		result.Detail = "Получен ответ QUIC от UDP-порта. Авторизация и передача трафика не проверялись. Создайте маршрут и повторите проверку для поддерживаемых протоколов."
	} else {
		duration, err = probeNodeRouteTCP(ctx, node.Address, inbound.Port)
		if err != nil {
			result.Detail = "Не удалось установить TCP-соединение с портом ноды: " + err.Error()
			return result, nil
		}
		result.Detail = "TCP-порт входящего подключения отвечает, но это не подтверждает работоспособность каскада. Для этого протокола сквозная проверка недоступна."
	}
	result.Reachable = true
	result.LatencyMs = duration.Milliseconds()
	if result.LatencyMs == 0 {
		result.LatencyMs = 1
	}
	return result, nil
}
