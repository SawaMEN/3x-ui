package dnstt

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// DelegationCheck reports whether public recursive DNS sees the authoritative
// name server and an A record matching the server address. A failed check does
// not change a running tunnel or its cryptographic identity.
type DelegationCheck struct {
	Zone       string   `json:"zone"`
	NameServer string   `json:"nameServer"`
	ExpectedIP string   `json:"expectedIP,omitempty"`
	Addresses  []string `json:"addresses"`
	NSFound    bool     `json:"nsFound"`
	IPMatches  bool     `json:"ipMatches"`
	Ready      bool     `json:"ready"`
	Details    string   `json:"details"`
}

type dnsLookup interface {
	LookupNS(context.Context, string) ([]*net.NS, error)
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

func checkDelegation(ctx context.Context, cfg Config, resolver dnsLookup) (DelegationCheck, error) {
	normalize(&cfg)
	if err := Validate(cfg); err != nil {
		return DelegationCheck{}, err
	}
	parts := strings.Split(cfg.Domain, ".")
	ns := "ns." + strings.Join(parts[1:], ".")
	result := DelegationCheck{Zone: cfg.Domain, NameServer: ns, ExpectedIP: cfg.ServerIP, Addresses: []string{}}
	records, err := resolver.LookupNS(ctx, cfg.Domain)
	if err != nil {
		result.Details = "DNS-делегирование пока не найдено: " + err.Error()
		return result, nil
	}
	for _, record := range records {
		if record != nil && strings.EqualFold(strings.TrimSuffix(record.Host, "."), ns) {
			result.NSFound = true
			break
		}
	}
	if !result.NSFound {
		result.Details = "NS-запись зоны не указывает на " + ns
		return result, nil
	}
	addresses, err := resolver.LookupIPAddr(ctx, ns)
	if err != nil {
		result.Details = "NS-запись обнаружена, но IP её хоста не разрешается: " + err.Error()
		return result, nil
	}
	for _, ip := range addresses {
		if ip.IP.To4() == nil {
			continue
		}
		result.Addresses = append(result.Addresses, ip.IP.String())
		if cfg.ServerIP == "" || cfg.ServerIP == ip.IP.String() {
			result.IPMatches = true
		}
	}
	result.Ready = result.NSFound && result.IPMatches
	if result.Ready {
		result.Details = "NS и IPv4-адрес доступны через DNS. Это не гарантирует, что UDP/53 доступен извне."
	} else {
		result.Details = fmt.Sprintf("IP NS-сервера не совпадает с ожидаемым адресом %s", cfg.ServerIP)
	}
	return result, nil
}

func CheckDelegation(ctx context.Context) (DelegationCheck, error) {
	cfg, err := load()
	if err != nil {
		return DelegationCheck{}, err
	}
	if cfg.Domain == "" {
		return DelegationCheck{}, errors.New("сначала настройте домен DNSTT")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return checkDelegation(lookupCtx, cfg, net.DefaultResolver)
}
