package service

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type TelemtConnection struct {
	IP          string  `json:"ip"`
	City        string  `json:"city"`
	Region      string  `json:"region"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

type telemtGeoCacheEntry struct {
	value   TelemtConnection
	expires time.Time
}

var telemtGeoCache struct {
	sync.RWMutex
	items map[string]telemtGeoCacheEntry
}

func (TelemtService) ConnectedClients() ([]TelemtConnection, error) {
	if systemctl("is-active", "--quiet", telemtServiceName) != nil {
		return []TelemtConnection{}, nil
	}

	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get("http://127.0.0.1:9091/v1/stats/users/active-ips")
	if err != nil {
		return nil, fmt.Errorf("telemt: active client API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telemt: active client API returned HTTP %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ActiveIPs map[string][]string `json:"active_ips"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("telemt: decode active clients: %w", err)
	}
	if !envelope.OK {
		return nil, fmt.Errorf("telemt: active client API rejected request")
	}

	seen := make(map[string]struct{})
	ips := make([]string, 0)
	for _, values := range envelope.Data.ActiveIPs {
		for _, rawIP := range values {
			ip := strings.TrimSpace(rawIP)
			if ip == "" {
				continue
			}
			if net.ParseIP(ip) == nil {
				if host, _, err := net.SplitHostPort(ip); err == nil {
					ip = host
				}
			}
			if net.ParseIP(ip) == nil {
				continue
			}
			if _, ok := seen[ip]; ok {
				continue
			}
			seen[ip] = struct{}{}
			ips = append(ips, ip)
		}
	}
	sort.Strings(ips)

	out := make([]TelemtConnection, 0, len(ips))
	for _, ip := range ips {
		out = append(out, telemtGeoLookup(client, ip))
	}
	return out, nil
}

func telemtGeoLookup(client *http.Client, ip string) TelemtConnection {
	if parsed := net.ParseIP(ip); parsed != nil && (parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocal() || parsed.IsUnspecified()) {
		return TelemtConnection{IP: ip, City: "Локальная сеть", Country: "—"}
	}

	now := time.Now()
	telemtGeoCache.RLock()
	cached, ok := telemtGeoCache.items[ip]
	telemtGeoCache.RUnlock()
	if ok && cached.expires.After(now) {
		return cached.value
	}

	result := TelemtConnection{IP: ip, City: "Неизвестно", Country: "—"}
	endpoint := "https://ipwho.is/" + url.PathEscape(ip)
	resp, err := client.Get(endpoint)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var geo struct {
				Success     bool    `json:"success"`
				City        string  `json:"city"`
				Region      string  `json:"region"`
				Country     string  `json:"country"`
				CountryCode string  `json:"country_code"`
				Latitude    float64 `json:"latitude"`
				Longitude   float64 `json:"longitude"`
			}
			if json.NewDecoder(resp.Body).Decode(&geo) == nil && geo.Success {
				result.City = strings.TrimSpace(geo.City)
				result.Region = strings.TrimSpace(geo.Region)
				result.Country = strings.TrimSpace(geo.Country)
				result.CountryCode = strings.TrimSpace(geo.CountryCode)
				result.Latitude = geo.Latitude
				result.Longitude = geo.Longitude
				if result.City == "" {
					result.City = "Неизвестно"
				}
				if result.Country == "" {
					result.Country = "—"
				}
			}
		}
	}

	telemtGeoCache.Lock()
	if telemtGeoCache.items == nil {
		telemtGeoCache.items = make(map[string]telemtGeoCacheEntry)
	}
	telemtGeoCache.items[ip] = telemtGeoCacheEntry{value: result, expires: now.Add(30 * time.Minute)}
	telemtGeoCache.Unlock()
	return result
}
