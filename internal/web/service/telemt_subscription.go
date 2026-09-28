package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

var telemtSubscriptionProfileMu sync.Mutex

// telemtSubscriptionUsername maps a subscription identifier to a stable Telemt
// username without exposing the subscription token itself in telemt.toml.
func telemtSubscriptionUsername(subID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(subID)))
	return "sub_" + hex.EncodeToString(sum[:12])
}

// EnsureSubscriptionProxy returns the stable Telemt proxy assigned to subID,
// creating it on first use. Repeated calls keep the same username and secret.
func (TelemtService) EnsureSubscriptionProxy(subID, host string) (TelemtProxy, error) {
	subID = strings.TrimSpace(subID)
	host = strings.TrimSpace(host)
	if subID == "" {
		return TelemtProxy{}, errors.New("telemt: subscription id is required")
	}
	if host == "" || strings.ContainsAny(host, " \t\n\r\"") {
		return TelemtProxy{}, errors.New("telemt: public host is required")
	}
	if _, err := os.Stat(telemtBinaryPath); err != nil {
		return TelemtProxy{}, fmt.Errorf("telemt: proxy engine is not installed: %w", err)
	}

	telemtSubscriptionProfileMu.Lock()
	defer telemtSubscriptionProfileMu.Unlock()

	if err := ensureTelemtConfig(); err != nil {
		return TelemtProxy{}, err
	}
	original, err := os.ReadFile(telemtConfigPath)
	if err != nil {
		return TelemtProxy{}, err
	}

	var raw struct {
		Server struct {
			Port int `toml:"port"`
		} `toml:"server"`
		General struct {
			Modes struct {
				TLS bool `toml:"tls"`
			} `toml:"modes"`
		} `toml:"general"`
		Access struct {
			Users map[string]string `toml:"users"`
		} `toml:"access"`
	}
	if err := toml.Unmarshal(original, &raw); err != nil {
		return TelemtProxy{}, fmt.Errorf("telemt: parse config: %w", err)
	}

	username := telemtSubscriptionUsername(subID)
	secret := strings.TrimSpace(raw.Access.Users[username])
	created := false
	if secret == "" {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			return TelemtProxy{}, fmt.Errorf("telemt: generate subscription secret: %w", err)
		}
		secret = hex.EncodeToString(buf)

		text := string(original)
		section := "[access.users]"
		idx := strings.Index(text, section)
		if idx < 0 {
			return TelemtProxy{}, errors.New("telemt: access.users section is missing")
		}
		insertAt := len(text)
		if next := strings.Index(text[idx+len(section):], "\n["); next >= 0 {
			insertAt = idx + len(section) + next + 1
		}
		entry := fmt.Sprintf("%s = \"%s\"\n", username, secret)
		text = text[:insertAt] + entry + text[insertAt:]
		if err := os.WriteFile(telemtConfigPath, []byte(text), 0o600); err != nil {
			return TelemtProxy{}, err
		}
		created = true
	}

	rollback := func() {
		if created {
			_ = os.WriteFile(telemtConfigPath, original, 0o600)
			_ = systemctl("daemon-reload")
		}
	}

	if created {
		if err := systemctl("daemon-reload"); err != nil {
			rollback()
			return TelemtProxy{}, err
		}
		if systemctl("is-active", "--quiet", telemtServiceName) == nil {
			if err := systemctl("restart", telemtServiceName); err != nil {
				rollback()
				_ = systemctl("restart", telemtServiceName)
				return TelemtProxy{}, fmt.Errorf("telemt: subscription profile was rejected: %w", err)
			}
		} else if err := systemctl("start", telemtServiceName); err != nil {
			rollback()
			return TelemtProxy{}, fmt.Errorf("telemt: failed to start service: %w", err)
		}
	} else if systemctl("is-active", "--quiet", telemtServiceName) != nil {
		if err := systemctl("start", telemtServiceName); err != nil {
			return TelemtProxy{}, fmt.Errorf("telemt: failed to start service: %w", err)
		}
	}

	link, err := telemtGeneratedLink(username, raw.General.Modes.TLS)
	if err != nil {
		if created {
			rollback()
			_ = systemctl("restart", telemtServiceName)
		}
		return TelemtProxy{}, fmt.Errorf("telemt: failed to obtain subscription link: %w", err)
	}
	if parsed, err := url.Parse(link); err == nil {
		query := parsed.Query()
		query.Set("server", host)
		parsed.RawQuery = query.Encode()
		link = parsed.String()
	}

	return TelemtProxy{
		Name:   username,
		Secret: secret,
		Host:   host,
		Port:   raw.Server.Port,
		TLS:    raw.General.Modes.TLS,
		Link:   link,
	}, nil
}
