package service

import (
	"context"
	"net"
	"sort"
	"strings"
)

// GatewayDiscovery is read-only. Suggestions never modify routes, interface
// addresses, nftables or the currently running core.
type GatewayInterfaceAddress struct {
	IP      string `json:"ip"`
	Prefix  int    `json:"prefix"`
	Private bool   `json:"private"`
}

type GatewayInterfaceInfo struct {
	Name         string                    `json:"name"`
	Addresses    []GatewayInterfaceAddress `json:"addresses"`
	DefaultRoute bool                      `json:"defaultRoute"`
}

type GatewayDiscovery struct {
	Interfaces []GatewayInterfaceInfo `json:"interfaces"`
	DefaultWAN string                 `json:"defaultWAN,omitempty"`
	Suggested  *GatewayNetworkConfig  `json:"suggested,omitempty"`
	Note       string                 `json:"note"`
}

// gatewayDefaultRouteInterface never treats a route without an explicit dev
// field as a usable WAN interface. Ambiguous default routes return no guess.
func gatewayDefaultRouteInterface(output string) string {
	found := ""
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "default" {
			continue
		}
		for i := 1; i+1 < len(fields); i++ {
			if fields[i] != "dev" {
				continue
			}
			name := fields[i+1]
			if !gatewayInterfaceName.MatchString(name) {
				return ""
			}
			if found != "" && found != name {
				return ""
			}
			found = name
			break
		}
	}
	return found
}

func gatewayInterfaceFromAddresses(iface net.Interface, addresses []net.Addr, defaultWAN string) GatewayInterfaceInfo {
	info := GatewayInterfaceInfo{Name: iface.Name, Addresses: []GatewayInterfaceAddress{}, DefaultRoute: iface.Name == defaultWAN}
	for _, addr := range addresses {
		ip, network, err := net.ParseCIDR(addr.String())
		if err != nil || network == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			continue
		}
		prefix, bits := network.Mask.Size()
		if bits != 32 || prefix < 1 || prefix > 32 {
			continue
		}
		info.Addresses = append(info.Addresses, GatewayInterfaceAddress{
			IP: ip.To4().String(), Prefix: prefix, Private: ip.IsPrivate(),
		})
	}
	sort.Slice(info.Addresses, func(i, j int) bool {
		return info.Addresses[i].IP < info.Addresses[j].IP
	})
	return info
}

// Only a single private subnet on an UP, non-default-route interface is
// suggested. Public addresses and the host's default WAN are never proposed
// for transparent interception automatically.
func gatewaySuggestNetwork(interfaces []GatewayInterfaceInfo, defaultWAN string) (*GatewayNetworkConfig, string) {
	// Missing default-route data means we cannot distinguish LAN from WAN.
	// Never guess a transparent interception interface in that case.
	if defaultWAN == "" {
		return nil, "Не удалось надёжно определить основной маршрут. Укажите LAN и WAN вручную."
	}
	var possibilities []GatewayNetworkConfig
	for _, iface := range interfaces {
		if iface.DefaultRoute || iface.Name == defaultWAN {
			continue
		}
		for _, addr := range iface.Addresses {
			if addr.Private && addr.Prefix >= 8 && addr.Prefix <= 30 {
				possibilities = append(possibilities, GatewayNetworkConfig{
					LANInterface: iface.Name, LANIP: addr.IP, LANPrefix: addr.Prefix, WANInterface: defaultWAN,
				})
			}
		}
	}
	if len(possibilities) != 1 {
		return nil, "Выберите LAN-интерфейс и его адрес вручную: безопасный вариант не определён однозначно."
	}
	return &possibilities[0], "Найден один частный LAN-адрес вне интерфейса основного маршрута. Проверьте, что клиенты используют этот сервер как шлюз."
}

func (s *GatewayNetworkService) Discover(ctx context.Context) (GatewayDiscovery, error) {
	result := GatewayDiscovery{Interfaces: []GatewayInterfaceInfo{}}
	interfaces := s.interfaces
	if interfaces == nil {
		interfaces = net.Interfaces
	}
	all, err := interfaces()
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	output, err := s.command(ctx, "ip", "-4", "route", "show", "default")
	if err == nil {
		result.DefaultWAN = gatewayDefaultRouteInterface(output)
	}
	for _, iface := range all {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || !gatewayInterfaceName.MatchString(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		info := gatewayInterfaceFromAddresses(iface, addrs, result.DefaultWAN)
		if len(info.Addresses) > 0 {
			result.Interfaces = append(result.Interfaces, info)
		}
	}
	sort.Slice(result.Interfaces, func(i, j int) bool { return result.Interfaces[i].Name < result.Interfaces[j].Name })
	result.Suggested, result.Note = gatewaySuggestNetwork(result.Interfaces, result.DefaultWAN)
	return result, nil
}
