// Package snell contains the panel's Snell credential and version contract.
package snell

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

type Settings struct {
	Version  int            `json:"version"`
	PSK      string         `json:"psk"`
	Mode     string         `json:"mode"`
	ObfsMode string         `json:"obfsMode"`
	ObfsHost string         `json:"obfsHost"`
	QuicProxy bool          `json:"quicProxy,omitempty"`
	Clients  []model.Client `json:"clients"`
}

// Reject control characters in credentials and HTTP Host values before they
// reach the native core or are exported to subscription URLs.
func containsControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

func Secret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func Parse(raw string) (Settings, error) {
	var s Settings
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return s, err
	}
	if s.Version != 5 && s.Version != 6 {
		return s, fmt.Errorf("Snell server version must be 5 or 6")
	}
	if s.PSK == "" || containsControl(s.PSK) {
		return s, fmt.Errorf("Snell requires a PSK without control characters")
	}
	if s.Version == 6 {
		if s.QuicProxy {
			return s, fmt.Errorf("Snell QUIC Proxy is only supported by v5")
		}
		if len(s.PSK) < 12 || len(s.PSK) > 255 {
			return s, fmt.Errorf("Snell v6 PSK must be 12..255 bytes")
		}
		if s.Mode != "" && s.Mode != "default" && s.Mode != "unshaped" {
			return s, fmt.Errorf("Snell v6 mode must be default or unshaped")
		}
		if s.ObfsMode != "" && s.ObfsMode != "none" {
			return s, fmt.Errorf("Snell v6 uses mode instead of obfsMode")
		}
	} else {
		if s.ObfsMode != "" && s.ObfsMode != "none" && s.ObfsMode != "http" {
			return s, fmt.Errorf("Snell v5 obfsMode must be none or http")
		}
		if s.Mode != "" && s.Mode != "default" {
			return s, fmt.Errorf("Snell v5 does not support traffic shaping modes")
		}
	}
	// obfs_host is a client-side HTTP Host value (not a server listen option).
	// Validate it even on the inbound so exported links cannot contain
	// control characters; keep legacy non-HTTP hosts harmlessly stored.
	if containsControl(s.ObfsHost) {
		return s, fmt.Errorf("Snell HTTP obfuscation host contains control characters")
	}
	seenKeys := map[string]bool{}
	seenNames := map[string]bool{}
	for _, c := range s.Clients {
		if !c.Enable {
			continue
		}
		name := strings.TrimSpace(c.Email)
		if name == "" || c.Password == "" {
			return s, fmt.Errorf("Snell clients require a name and user key")
		}
		if containsControl(name) || containsControl(c.Password) {
			return s, fmt.Errorf("Snell user names and keys cannot contain control characters")
		}
		if len(c.Password) > 255 {
			return s, fmt.Errorf("Snell user key must be at most 255 bytes")
		}
		if seenNames[strings.ToLower(name)] {
			return s, fmt.Errorf("Snell client names must be unique")
		}
		if seenKeys[c.Password] {
			return s, fmt.Errorf("Snell user keys must be unique")
		}
		seenNames[strings.ToLower(name)] = true
		seenKeys[c.Password] = true
	}
	return s, nil
}

// Prepare seeds credentials only when missing, keeping existing subscriptions valid.
func Prepare(ib *model.Inbound, previous string) error {
	if ib.Protocol != model.Snell {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil {
		return err
	}
	if raw == nil {
		raw = map[string]any{}
	}
	var old Settings
	_ = json.Unmarshal([]byte(previous), &old)
	if raw["version"] == nil {
		raw["version"] = 6
	}
	if raw["psk"] == nil || raw["psk"] == "" {
		secret := old.PSK
		if secret == "" {
			var err error
			secret, err = Secret()
			if err != nil {
				return err
			}
		}
		raw["psk"] = secret
	}
	previousKeys := map[string]string{}
	for _, c := range old.Clients {
		previousKeys[c.Email] = c.Password
	}
	if clients, ok := raw["clients"].([]any); ok {
		for _, entry := range clients {
			c, ok := entry.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid Snell client")
			}
			if c["password"] == nil || c["password"] == "" {
				email, _ := c["email"].(string)
				secret := previousKeys[email]
				if secret == "" {
					var err error
					secret, err = Secret()
					if err != nil {
						return err
					}
				}
				c["password"] = secret
			}
		}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	// Reject unsupported combinations before removing inactive defaults.
	s, err := Parse(string(data))
	if err != nil {
		return err
	}
	if s.Version == 6 {
		delete(raw, "quicProxy")
		delete(raw, "obfsMode")
		delete(raw, "obfsHost")
		if s.Mode == "" || s.Mode == "default" {
			delete(raw, "mode")
		}
	} else {
		delete(raw, "mode")
		if !s.QuicProxy {
			delete(raw, "quicProxy")
		}
		if s.ObfsMode == "" || s.ObfsMode == "none" {
			delete(raw, "obfsMode")
			delete(raw, "obfsHost")
		}
	}
	canonical, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	ib.Settings = string(canonical)
	return nil
}

// sing-box deliberately calls the v5-compatible outbound version 4.
func (s Settings) ClientVersion() int {
	if s.Version == 5 {
		if s.QuicProxy {
			return 5
		}
		return 4
	}
	return 6
}

func (s Settings) Outbound(address string, port int, client model.Client) map[string]any {
	out := map[string]any{"type": "snell", "server": address, "server_port": port, "version": s.ClientVersion(), "psk": s.PSK, "userkey": client.Password}
	if s.Version == 6 {
		if s.Mode != "" {
			out["mode"] = s.Mode
		}
	} else {
		if s.ObfsMode != "" {
			out["obfs_mode"] = s.ObfsMode
		}
		if s.ObfsMode == "http" && s.ObfsHost != "" {
			out["obfs_host"] = s.ObfsHost
		}
	}
	return out
}
