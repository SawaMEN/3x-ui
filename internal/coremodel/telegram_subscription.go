package coremodel

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
