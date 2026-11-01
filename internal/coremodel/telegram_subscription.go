package coremodel

import (
	"net/url"
	"strconv"
	"strings"
)

// TelegramProxyProfile contains only the credentials intended for one subscriber.
type TelegramProxyProfile struct {
	Name string `json:"name,omitempty"`
	Host string `json:"host"`
	Port int    `json:"port"`
	TLS  bool   `json:"tls"`
	Link string `json:"link"`
}

type TelegramSubscription struct {
	Enabled         bool                   `json:"enabled"`
	Personal        *TelegramProxyProfile  `json:"personal,omitempty"`
	WebProxyEnabled bool                   `json:"webProxyEnabled"`
	WebProxy        string                 `json:"webProxy,omitempty"`
	Proxies         []TelegramProxyProfile `json:"proxies,omitempty"`
}

type RemoteTelegramProxy struct {
	NodeID   int                  `json:"nodeId"`
	NodeName string               `json:"nodeName"`
	Profile  TelegramProxyProfile `json:"profile"`
}

func ParseTelegramProxyLink(link, name string) (TelegramProxyProfile, bool) {
	u, err := url.Parse(link)
	if err != nil {
		return TelegramProxyProfile{}, false
	}
	web := u.Scheme == "tg" && u.Host == "webproxy"
	proxy := u.Scheme == "tg" && u.Host == "proxy"
	if !web && !proxy {
		return TelegramProxyProfile{}, false
	}
	q := u.Query()
	port, _ := strconv.Atoi(q.Get("port"))
	if q.Get("server") == "" || q.Get("secret") == "" || (proxy && (port < 1 || port > 65535)) {
		return TelegramProxyProfile{}, false
	}
	return TelegramProxyProfile{Name: name, Host: q.Get("server"), Port: port, TLS: strings.HasPrefix(q.Get("secret"), "ee"), Link: link}, true
}
