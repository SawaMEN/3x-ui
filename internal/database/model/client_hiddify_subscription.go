package model

import (
	"net/url"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

// NormalizeHiddifySubURI keeps imported subscription bases on HTTPS/443.
// Invalid stored values are ignored without modifying the database.
func NormalizeHiddifySubURI(value string) string {
	raw := strings.TrimSpace(value)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.ForceQuery || parsed.RawQuery != "" || strings.Contains(raw, "#") {
		return ""
	}
	host, err := netsafe.NormalizeHost(strings.ToLower(parsed.Hostname()))
	if err != nil {
		return ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	path := strings.Trim(parsed.Path, "/")
	if path == "" || len(path) > 128 {
		return ""
	}
	for _, r := range path {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return ""
		}
	}
	return "https://" + host + "/" + path + "/"
}

// AfterFind normalizes legacy Hiddify URLs as records leave the database. This
// covers existing installations without coupling imported users to the current
// regular subscription port and keeps every consumer (UI, Happ, Telegram and
// exports) on the same standard-443 URL.
func (r *ClientRecord) AfterFind(_ *gorm.DB) error {
	if r.KeepAliveLegacy != 0 {
		r.KeepAlive = KeepAliveValue(strconv.Itoa(r.KeepAliveLegacy))
	} else if r.KeepAliveRange != "" {
		r.KeepAlive = r.KeepAliveRange
	} else {
		r.KeepAlive = ""
	}

	if r.HiddifySubURI != "" {
		r.HiddifySubURI = NormalizeHiddifySubURI(r.HiddifySubURI)
	}
	return nil
}
