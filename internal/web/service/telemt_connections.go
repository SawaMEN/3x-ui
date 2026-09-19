package service

import (
	"context"
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
	Users       []string `json:"users"`
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
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:9091/v1/stats/users/active-ips", nil)
	if err != nil {
		return nil, fmt.Errorf("telemt: create active client request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telemt: active client API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telemt: active client API returned HTTP %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data []struct {
			Username  string   `json:"username"`
			ActiveIPs []string `json:"active_ips"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("telemt: decode active clients: %w", err)
	}
	if !envelope.OK {
		return nil, fmt.Errorf("telemt: active client API rejected request")
	}

	usersByIP := make(map[string]map[string]struct{})
	for _, user := range envelope.Data {
		for _, rawIP := range user.ActiveIPs {
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
			if usersByIP[ip] == nil {
				usersByIP[ip] = make(map[string]struct{})
			}
			if user.Username != "" {
				usersByIP[ip][user.Username] = struct{}{}
			}
		}
	}

	ips := make([]string, 0, len(usersByIP))
	for ip := range usersByIP {
		ips = append(ips, ip)
	}
	sort.Strings(ips)

	out := make([]TelemtConnection, 0, len(ips))
	for _, ip := range ips {
		users := make([]string, 0, len(usersByIP[ip]))
		for username := range usersByIP[ip] {
			users = append(users, username)
		}
		sort.Strings(users)
		row := telemtGeoLookup(client, ip)
		row.Users = users
		out = append(out, row)
	}
	return out, nil
}

func telemtGeoLookup(client *http.Client, ip string) TelemtConnection {
	if parsed := net.ParseIP(ip); parsed != nil && (parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() || parsed.IsUnspecified()) {
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
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err == nil {
		resp, err := client.Do(req)
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
	}

	telemtGeoCache.Lock()
	if telemtGeoCache.items == nil {
		telemtGeoCache.items = make(map[string]telemtGeoCacheEntry)
	}
	telemtGeoCache.items[ip] = telemtGeoCacheEntry{value: result, expires: now.Add(30 * time.Minute)}
	telemtGeoCache.Unlock()
	return result
}
