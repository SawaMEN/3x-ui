package externalvpn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func validateVetroff(inst Instance) error {
	s := inst.Settings
	if inst.Port < 1 || inst.Port > 65535 || (inst.Listen != "" && net.ParseIP(strings.Trim(inst.Listen, "[]")) == nil) {
		return fmt.Errorf("VetroffProxy requires an IP listen address and port 1..65535")
	}
	if !domainPattern.MatchString(s.Hostname) || !filepath.IsAbs(s.Certificate) || !filepath.IsAbs(s.PrivateKey) {
		return fmt.Errorf("VetroffProxy requires a hostname and absolute TLS certificate/private key paths")
	}
	if len(s.Clients) > 1024 {
		return fmt.Errorf("VetroffProxy supports at most 1024 users")
	}
	seen := map[string]bool{}
	for _, c := range s.Clients {
		if c.Email == "" || seen[c.Email] || len(c.Email) > 256 || strings.ContainsAny(c.Email, ":\r\n\x00") || len(c.Password) < 24 || len(c.Password) > 1024 || strings.ContainsAny(c.Password, "\r\n\x00") {
			return fmt.Errorf("VetroffProxy requires unique usernames and passwords of 24..1024 bytes")
		}
		seen[c.Email] = true
	}
	return nil
}

func vetroffFiles(inst Instance) ([]string, error) {
	folder := filepath.Join(directory(), strconv.Itoa(inst.ID))
	cert, key, err := certificate(inst, folder)
	if err != nil {
		return nil, err
	}
	users := make([]map[string]any, 0, len(inst.Settings.Clients))
	for _, c := range inst.Settings.Clients {
		users = append(users, map[string]any{"id": FPTNUsername(c.Email), "username": FPTNUsername(c.Email), "password": c.Password, "disabled": !c.Enable})
	}
	usersPath := filepath.Join(folder, "vetroff-users.json")
	raw, err := json.Marshal(map[string]any{"v": 1, "users": users})
	if err != nil {
		return nil, err
	}
	if err := writePrivate(usersPath, raw); err != nil {
		return nil, err
	}
	path := filepath.Join(folder, "vetroff-server.json")
	raw, err = json.Marshal(map[string]any{"v": 1, "listen": inst.Bind(), "h3_listen": inst.Bind(), "enable_udp": true, "cert": cert, "key": key, "users_file": usersPath, "users_state": filepath.Join(folder, "vetroff-usage.json"), "max_tunnels": 256, "max_connections": 512})
	if err != nil {
		return nil, err
	}
	if err := writePrivate(path, raw); err != nil {
		return nil, err
	}
	return []string{"server", "--config", path}, nil
}

func exportVetroffLink(inst Instance, email, address string, port int) (string, error) {
	if inst.Protocol != model.VetroffProxy {
		return "", fmt.Errorf("not a VetroffProxy inbound")
	}
	if err := inst.Validate(); err != nil {
		return "", err
	}
	address = strings.Trim(address, "[]")
	if address == "" || port < 1 || port > 65535 || strings.ContainsAny(address, "/?#@\r\n") {
		return "", fmt.Errorf("invalid VetroffProxy endpoint")
	}
	if net.ParseIP(address) == nil && !domainPattern.MatchString(address) {
		return "", fmt.Errorf("invalid VetroffProxy endpoint hostname")
	}
	if ip := net.ParseIP(address); ip != nil && ip.IsUnspecified() {
		return "", fmt.Errorf("unspecified endpoint")
	}
	for _, c := range inst.Settings.Clients {
		if c.Enable && c.Email == email {
			origin := (&url.URL{Scheme: "https", Host: net.JoinHostPort(address, strconv.Itoa(port))}).String()
			raw, err := json.Marshal(map[string]any{"v": 1, "servers": []string{origin}, "username": FPTNUsername(c.Email), "password": c.Password, "sni": inst.Settings.Hostname, "transport": "h2-connect", "tls": "auto"})
			if err != nil {
				return "", err
			}
			return "vetroff://" + base64.RawURLEncoding.EncodeToString(raw), nil
		}
	}
	return "", fmt.Errorf("VetroffProxy client disabled or absent")
}

func validateVetroffPanelLimits(ib *model.Inbound) error {
	var raw struct {
		Clients []model.Client `json:"clients"`
	}
	if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil {
		return err
	}
	if ib.Total > 0 {
		return fmt.Errorf("VetroffProxy panel traffic quotas are not supported")
	}
	for _, c := range raw.Clients {
		if c.TotalGB > 0 || c.LimitIP > 0 || c.ExpiryTime < 0 || (c.SpeedLimit != nil && c.SpeedLimit.Enabled) {
			return fmt.Errorf("VetroffProxy does not support panel traffic, IP, speed or first-use duration limits")
		}
	}
	return nil
}
