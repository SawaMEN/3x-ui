package service

import (
	"context"
	"net"
	"strconv"
	"reflect"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestMieruNodeRouteProbeUsesRealBindings(t *testing.T) {
	ib := &model.Inbound{Id: 42, Protocol: model.Mieru, Port: 8443, Settings: "{\"tcpPorts\":[\"2012-2022\",\"2027\"],\"udpPorts\":[\"24000-24020\"]}"}
	p := nodeRouteMieruProbePlan(ib)
	if !reflect.DeepEqual(p.tcpPorts, []int{2012, 2017, 2022, 2027}) {
		t.Fatalf("TCP probe ports: %v", p.tcpPorts)
	}
	if !reflect.DeepEqual(p.udpPorts, []string{"24000-24020"}) || !p.partial {
		t.Fatalf("UDP/range probe plan: %+v", p)
	}
	if strings.Contains(strings.Join(p.portLabels, ","), "8443") {
		t.Fatalf("incorrectly advertised an unbound inbound port: %+v", p)
	}
}

func TestMieruNodeRouteProbeUDPOnlyIsInconclusive(t *testing.T) {
	ib := &model.Inbound{Id: 17, Protocol: model.Mieru, Port: 8443, Settings: "{\"tcpPorts\":[],\"udpPorts\":[\"25000-25010\"]}"}
	result := probeNodeRouteMieru(context.Background(), ib, "127.0.0.1")
	if result.Reachable || !result.Inconclusive || result.CheckedTarget || result.Transport != "udp" {
		t.Fatalf("UDP-only Mieru must be inconclusive, not a failed TCP check: %+v", result)
	}
	if !strings.Contains(result.Detail, "25000-25010") || !strings.Contains(result.Detail, "QUIC") {
		t.Fatal(result.Detail)
	}
}

func TestMieruNodeRouteProbeTCPUsesConfiguredPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	ib := &model.Inbound{Id: 7, Protocol: model.Mieru, Port: 8443, Settings: "{\"tcpPorts\":[\"" + strconv.Itoa(port) + "\"],\"udpPorts\":[]}"}
	result := probeNodeRouteMieru(context.Background(), ib, "127.0.0.1")
	if !result.Reachable || result.Inconclusive || !result.CheckedTarget || result.Transport != "tcp" {
		t.Fatalf("expected configured TCP listener to answer: %+v", result)
	}
}

func TestMieruNodeRouteProbeMixedTransportDoesNotReportUDPDown(t *testing.T) {
	// Binding an ephemeral TCP socket then closing it makes the TCP dial fail,
	// without claiming that the separate Mieru UDP transport is unavailable.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	ib := &model.Inbound{Id: 19, Protocol: model.Mieru, Port: port, Settings: "{\"tcpPorts\":[\"" + strconv.Itoa(port) + "\"],\"udpPorts\":[\"25001\"]}"}
	result := probeNodeRouteMieru(context.Background(), ib, "127.0.0.1")
	if result.Reachable || !result.Inconclusive || !result.CheckedTarget || result.Transport != "tcp+udp" {
		t.Fatalf("TCP refusal cannot prove mixed transport unavailable: %+v", result)
	}
	if !strings.Contains(result.Detail, "25001") {
		t.Fatal(result.Detail)
	}
}
