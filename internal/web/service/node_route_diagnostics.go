package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service/outbound"
)

// nodeRouteDelayProber is the running core's authenticated, per-outbound
// HTTPS probe. Testing the active outbound avoids the false negatives that
// can arise when a second, isolated core has a different runtime environment.
type nodeRouteDelayProber interface {
	ProxyDelay(context.Context, string, string, time.Duration) (*singbox.ClashProxyDelay, error)
}

func probeNodeRouteAppliedHTTP(ctx context.Context, tag string, prober nodeRouteDelayProber) (*outbound.TestOutboundResult, error) {
	return probeNodeRouteHTTP(func(target string) (*outbound.TestOutboundResult, error) {
		delay, err := prober.ProxyDelay(ctx, tag, target, 5*time.Second)
		if err != nil {
			// An unavailable endpoint or a failed request is not a reason to
			// bypass the selected outbound. Try the second HTTPS endpoint
			// through exactly the same applied outbound.
			return &outbound.TestOutboundResult{Error: err.Error()}, nil
		}
		if delay == nil || delay.Delay <= 0 {
			return &outbound.TestOutboundResult{Error: "ядро не вернуло положительную задержку"}, nil
		}
		return &outbound.TestOutboundResult{Success: true, Delay: delay.Delay}, nil
	})
}

func probeNodeRouteAppliedIfAvailable(ctx context.Context, core, tag string) (*outbound.TestOutboundResult, bool) {
	if !IsNativeCore(core) || !NativeCore(core).IsRunning() {
		return nil, false
	}
	// Only test an outbound actually present in the applied configuration.
	// An unsaved route or the fail-closed block placeholder must not be
	// represented as a working VPN tunnel.
	kind := singbox.AppliedOutboundType(tag)
	if kind == "" || kind == "block" {
		return nil, false
	}
	result, err := probeNodeRouteAppliedHTTP(ctx, tag, singbox.NewClashStatsClient())
	if err != nil {
		return &outbound.TestOutboundResult{Error: err.Error()}, true
	}
	return result, true
}

// nodeRouteProbeFailureStage describes the failing layer without interpreting
// a generic SOCKS reply as proof that the configured VPN is unavailable.
func nodeRouteProbeFailureStage(reason string) (string, string) {
	s := strings.ToLower(reason)
	switch {
	case strings.Contains(s, "general socks server failure"):
		return "proxy", "SOCKS-проверка получила общий отказ. Это не доказывает недоступность самого подключения; точная причина отсутствует в ответе SOCKS."
	case strings.Contains(s, "certificate") || strings.Contains(s, "x509") || strings.Contains(s, "tls handshake"):
		return "tls", "Ошибка TLS или проверки сертификата при тестовом подключении."
	case strings.Contains(s, "authentication") || strings.Contains(s, "unauthorized") || strings.Contains(s, "invalid user"):
		return "auth", "Возможная ошибка аутентификации проверочного подключения."
	case strings.Contains(s, "lookup ") || strings.Contains(s, "no such host"):
		return "dns", "Не удалось разрешить DNS-имя при проверке."
	case strings.Contains(s, "connection refused"):
		return "tcp", "Проверочное TCP-соединение отклонено."
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded") || strings.Contains(s, "i/o timeout"):
		return "timeout", "Истекло время ожидания ответа при проверке."
	case strings.Contains(s, "proxy core") || strings.Contains(s, "test config") || strings.Contains(s, "test sing-box"):
		return "test-core", "Ошибка запуска или настройки временного проверочного ядра."
	default:
		return "unknown", "HTTP-проверка не удалась. По одному этому результату нельзя определить доступность VPN."
	}
}

// nodeRouteTransportEvidence is an independent diagnostic, not a fallback
// that sends any HTTP traffic directly. In particular, QUIC version
// negotiation can time out even when a healthy UDP VPN listener is running.
func nodeRouteTransportEvidence(ctx context.Context, inbound *model.Inbound, native map[string]any) (bool, string) {
	host, _ := native["server"].(string)
	port := 0
	switch v := native["server_port"].(type) {
	case int:
		port = v
	case float64:
		port = int(v)
	}
	if host == "" {
		_, node, err := nodeRouteSource(inbound.Id)
		if err != nil {
			return false, "Адрес ноды для отдельной транспортной проверки недоступен."
		}
		host = node.Address
	}
	if port < 1 || port > 65535 {
		port = inbound.Port
	}
	if port < 1 || port > 65535 {
		return false, "Невозможно определить порт для отдельной транспортной проверки."
	}
	if nodeRouteIsUDP(inbound) {
		_, err := probeNodeRouteQUIC(ctx, host, port)
		if err != nil {
			return false, "UDP/QUIC-ответ не получен; это не подтверждает и не опровергает работоспособность UDP-туннеля."
		}
		return true, "Получен ответ QUIC от удалённого порта. Авторизация VPN этим тестом не проверялась."
	}
	_, err := probeNodeRouteTCP(ctx, host, port)
	if err != nil {
		return false, fmt.Sprintf("Отдельная TCP-проверка порта не прошла: %v", err)
	}
	return true, "Удалённый TCP-порт отвечает. Авторизация и передача HTTP-трафика этим тестом не проверялись."
}
