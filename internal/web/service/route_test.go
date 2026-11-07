package service

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	routercommand "github.com/xtls/xray-core/app/router/command"
	xraynet "github.com/xtls/xray-core/common/net"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// RouteTestRequest describes a synthetic connection. The Xray API evaluates
// routing rules without making a connection to the requested destination.
type RouteTestRequest struct {
	Domain     string `json:"domain"`
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	Network    string `json:"network"`
	InboundTag string `json:"inboundTag"`
	Protocol   string `json:"protocol"`
}

type RouteTestResult struct {
	Matched     bool     `json:"matched"`
	OutboundTag string   `json:"outboundTag"`
	GroupTags   []string `json:"groupTags"`
}

func buildXrayRouteContext(in RouteTestRequest) (*routercommand.RoutingContext, error) {
	if in.Port < 0 || in.Port > 65535 {
		return nil, fmt.Errorf("invalid destination port")
	}
	var network xraynet.Network
	switch strings.ToLower(strings.TrimSpace(in.Network)) {
	case "tcp":
		network = xraynet.Network_TCP
	case "udp":
		network = xraynet.Network_UDP
	default:
		return nil, fmt.Errorf("network must be tcp or udp")
	}
	domain := strings.TrimSpace(in.Domain)
	address := strings.TrimSpace(in.IP)
	if domain != "" && address != "" {
		return nil, fmt.Errorf("specify a domain or an IP address, not both")
	}
	if domain == "" && address == "" {
		return nil, fmt.Errorf("destination is required")
	}
	if len(domain) > 253 || strings.ContainsAny(domain, "/\\\\ \t\r\n") {
		return nil, fmt.Errorf("invalid destination domain")
	}
	request := &routercommand.RoutingContext{
		Network:      network,
		TargetDomain: domain,
		TargetPort:   uint32(in.Port),
		InboundTag:   strings.TrimSpace(in.InboundTag),
		Protocol:     strings.TrimSpace(in.Protocol),
	}
	if address != "" {
		ip := net.ParseIP(strings.Trim(address, "[]"))
		if ip == nil {
			return nil, fmt.Errorf("invalid destination IP")
		}
		if ipv4 := ip.To4(); ipv4 != nil {
			ip = ipv4
		}
		request.TargetIPs = [][]byte{[]byte(ip)}
	}
	return request, nil
}

// TestXrayRoute queries the live Xray RoutingService; it does not perform a
// network reachability probe and does not start another Xray instance.
func TestXrayRoute(ctx context.Context, input RouteTestRequest) (*RouteTestResult, error) {
	routingContext, err := buildXrayRouteContext(input)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	api := net.JoinHostPort("127.0.0.1", strconv.Itoa(reservedAPIPort()))
	//nolint:staticcheck // A bounded blocking dial ensures an offline core is reported.
	conn, err := grpc.DialContext(ctx, api, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("Xray routing API unavailable: %w", err)
	}
	defer conn.Close()
	reply, err := routercommand.NewRoutingServiceClient(conn).TestRoute(ctx, &routercommand.TestRouteRequest{
		RoutingContext: routingContext,
		PublishResult:  false,
	})
	if err != nil {
		return nil, fmt.Errorf("Xray route test failed: %w", err)
	}
	return &RouteTestResult{
		Matched:     reply.GetOutboundTag() != "",
		OutboundTag: reply.GetOutboundTag(),
		GroupTags:   reply.GetOutboundGroupTags(),
	}, nil
}
