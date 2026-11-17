package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
)

const maxMieruProbePorts = 8

// mieruProbePlan uses the same listener bindings as mita and subscription links.
// Inbound.Port is only the legacy fallback, not necessarily an active Mieru port.
type mieruProbePlan struct {
	tcpPorts   []int
	udpPorts   []string
	portLabels []string
	partial    bool
}

func nodeRouteMieruProbePlan(inbound *model.Inbound) mieruProbePlan {
	var plan mieruProbePlan
	if inbound == nil {
		return plan
	}
	seenTCP := make(map[int]bool)
	for _, binding := range mieru.PortBindingsFromInbound(inbound) {
		first, last := binding.Port, binding.Port
		label := strconv.Itoa(first)
		if binding.PortRange != "" {
			parts := strings.Split(binding.PortRange, "-")
			if len(parts) != 2 {
				continue
			}
			var err1, err2 error
			first, err1 = strconv.Atoi(parts[0])
			last, err2 = strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil {
				continue
			}
			label = binding.PortRange
		}
		if first < 1 || last > 65535 || first > last {
			continue
		}
		switch binding.Protocol {
		case "UDP":
			plan.udpPorts = append(plan.udpPorts, label)
			plan.portLabels = append(plan.portLabels, "UDP "+label)
		case "TCP":
			plan.portLabels = append(plan.portLabels, "TCP "+label)
			// A range can have more active ports than we can reasonably test.
			// Never mark the whole range offline from a handful of samples.
			if first != last {
				plan.partial = true
			}
			candidates := []int{first}
			if inbound.Port >= first && inbound.Port <= last && inbound.Port != first {
				candidates = append(candidates, inbound.Port)
			}
			if last != first {
				candidates = append(candidates, first+(last-first)/2, last)
			}
			for _, port := range candidates {
				if seenTCP[port] {
					continue
				}
				if len(plan.tcpPorts) >= maxMieruProbePorts {
					plan.partial = true
					continue
				}
				seenTCP[port] = true
				plan.tcpPorts = append(plan.tcpPorts, port)
			}
		}
	}
	return plan
}

// probeNodeRouteMieru tests actual TCP bindings, not an unrelated inbound.Port.
// Mieru UDP is not QUIC: its silent encrypted handshake cannot be checked by a
// generic UDP dial or a QUIC version-negotiation packet.
func probeNodeRouteMieru(ctx context.Context, inbound *model.Inbound, address string) NodeRouteProbeResult {
	plan := nodeRouteMieruProbePlan(inbound)
	result := NodeRouteProbeResult{Tag: nodeRouteTag(inbound.Id), Transport: "tcp"}
	if len(plan.udpPorts) != 0 {
		result.Transport = "udp"
		if len(plan.tcpPorts) != 0 {
			result.Transport = "tcp+udp"
		}
	}
	if len(plan.tcpPorts) == 0 {
		result.Inconclusive = true
		if len(plan.udpPorts) > 0 {
			result.Detail = "Mieru использует UDP-порты " + strings.Join(plan.udpPorts, ", ") + ". UDP Mieru не является QUIC: проверка TCP или QUIC здесь неприменима. Доступность и авторизацию можно подтвердить только реальным подключением Mieru."
		} else {
			result.Detail = "У Mieru не найдено действующих TCP/UDP-портов. Проверьте настройки входящего подключения."
		}
		return result
	}

	type attempt struct {
		port     int
		duration time.Duration
		err      error
	}
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	attempts := make(chan attempt, len(plan.tcpPorts))
	for _, port := range plan.tcpPorts {
		go func(port int) {
			duration, err := probeNodeRouteTCP(probeCtx, address, port)
			attempts <- attempt{port: port, duration: duration, err: err}
		}(port)
	}
	result.CheckedTarget = true
	var firstErr error
	for range plan.tcpPorts {
		probe := <-attempts
		if probe.err == nil {
			result.Reachable = true
			result.Detail = fmt.Sprintf("TCP-порт Mieru %d отвечает. Проверка не подтверждает авторизацию и передачу VPN-трафика.", probe.port)
			if len(plan.udpPorts) > 0 {
				result.Detail += " UDP-порты Mieru отдельно не проверялись."
			}
			result.LatencyMs = probe.duration.Milliseconds()
			if result.LatencyMs < 1 {
				result.LatencyMs = 1
			}
			return result
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("порт %d: %w", probe.port, probe.err)
		}
	}
	result.Inconclusive = plan.partial || len(plan.udpPorts) > 0
	checked := make([]string, 0, len(plan.tcpPorts))
	for _, port := range plan.tcpPorts {
		checked = append(checked, strconv.Itoa(port))
	}
	result.Detail = "Нет TCP-ответа от проверенных портов Mieru (" + strings.Join(checked, ", ") + "): " + firstErr.Error()
	if plan.partial {
		result.Detail += ". Диапазон TCP проверен частично; недоступность остальных портов не доказана."
	}
	if len(plan.udpPorts) > 0 {
		result.Detail += ". UDP-порты (" + strings.Join(plan.udpPorts, ", ") + ") не проверялись: Mieru UDP не использует QUIC."
	}
	return result
}
