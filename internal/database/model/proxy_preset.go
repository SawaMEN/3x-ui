package model

import "time"

// ProxyPreset stores reusable subscription-endpoint overrides. The JSON config
// deliberately uses pointer fields: nil means "leave the Host value unchanged",
// while a non-nil zero/false/empty value explicitly clears or disables it.
type ProxyPreset struct {
	Id          int       `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId      int       `json:"-" gorm:"index;not null"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description"`
	Config      string    `json:"-" gorm:"type:text;not null"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (ProxyPreset) TableName() string { return "proxy_presets" }

// HostProxyPreset binds one logical Host group to a preset. Keeping this in a
// separate table preserves the existing hosts schema and all legacy Host API
// payloads: a group with no binding behaves exactly as before.
type HostProxyPreset struct {
	GroupId   string    `json:"groupId" gorm:"primaryKey;column:group_id"`
	UserId    int       `json:"-" gorm:"index;not null;column:user_id"`
	PresetId  int       `json:"presetId" gorm:"index;not null;column:preset_id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (HostProxyPreset) TableName() string { return "host_proxy_presets" }

// ProxyPresetConfig contains only fields that are safe and meaningful to reuse
// across Host endpoints. Identity/routing fields (group/inbound/address/remark,
// enabled state, node selection and tags) intentionally stay owned by Host.
type ProxyPresetConfig struct {
	Port                   *int      `json:"port,omitempty"`
	ServerDescription      *string   `json:"serverDescription,omitempty"`
	Security               *string   `json:"security,omitempty"`
	Sni                    *string   `json:"sni,omitempty"`
	HostHeader             *string   `json:"hostHeader,omitempty"`
	Path                   *string   `json:"path,omitempty"`
	Alpn                   *[]string `json:"alpn,omitempty"`
	Fingerprint            *string   `json:"fingerprint,omitempty"`
	CipherSuites           *string   `json:"cipherSuites,omitempty"`
	OverrideSniFromAddress *bool     `json:"overrideSniFromAddress,omitempty"`
	KeepSniBlank           *bool     `json:"keepSniBlank,omitempty"`
	PinnedPeerCertSha256   *[]string `json:"pinnedPeerCertSha256,omitempty"`
	VerifyPeerCertByName   *string   `json:"verifyPeerCertByName,omitempty"`
	AllowInsecure          *bool     `json:"allowInsecure,omitempty"`
	EchConfigList          *string   `json:"echConfigList,omitempty"`
	MuxParams              *string   `json:"muxParams,omitempty"`
	SockoptParams          *string   `json:"sockoptParams,omitempty"`
	FinalMask              *string   `json:"finalMask,omitempty"`
	VlessRoute             *string   `json:"vlessRoute,omitempty"`
	ExcludeFromSubTypes    *[]string `json:"excludeFromSubTypes,omitempty"`
	MihomoIpVersion        *string   `json:"mihomoIpVersion,omitempty"`
	MihomoX25519           *bool     `json:"mihomoX25519,omitempty"`
	ShuffleHost            *bool     `json:"shuffleHost,omitempty"`
}

// ApplyProxyPresetConfig overlays only explicitly present preset fields onto an
// in-memory Host. Callers must use a Host loaded specifically for subscription
// rendering; the normal Hosts API deliberately returns the stored base values.
func ApplyProxyPresetConfig(h *Host, cfg ProxyPresetConfig) {
	if h == nil {
		return
	}
	if cfg.Port != nil {
		h.Port = *cfg.Port
	}
	if cfg.ServerDescription != nil {
		h.ServerDescription = *cfg.ServerDescription
	}
	if cfg.Security != nil {
		h.Security = *cfg.Security
	}
	if cfg.Sni != nil {
		h.Sni = *cfg.Sni
	}
	if cfg.HostHeader != nil {
		h.HostHeader = *cfg.HostHeader
	}
	if cfg.Path != nil {
		h.Path = *cfg.Path
	}
	if cfg.Alpn != nil {
		h.Alpn = append([]string(nil), (*cfg.Alpn)...)
	}
	if cfg.Fingerprint != nil {
		h.Fingerprint = *cfg.Fingerprint
	}
	if cfg.CipherSuites != nil {
		h.CipherSuites = *cfg.CipherSuites
	}
	if cfg.OverrideSniFromAddress != nil {
		h.OverrideSniFromAddress = *cfg.OverrideSniFromAddress
	}
	if cfg.KeepSniBlank != nil {
		h.KeepSniBlank = *cfg.KeepSniBlank
	}
	if cfg.PinnedPeerCertSha256 != nil {
		h.PinnedPeerCertSha256 = append([]string(nil), (*cfg.PinnedPeerCertSha256)...)
	}
	if cfg.VerifyPeerCertByName != nil {
		h.VerifyPeerCertByName = *cfg.VerifyPeerCertByName
	}
	if cfg.AllowInsecure != nil {
		h.AllowInsecure = *cfg.AllowInsecure
	}
	if cfg.EchConfigList != nil {
		h.EchConfigList = *cfg.EchConfigList
	}
	if cfg.MuxParams != nil {
		h.MuxParams = *cfg.MuxParams
	}
	if cfg.SockoptParams != nil {
		h.SockoptParams = *cfg.SockoptParams
	}
	if cfg.FinalMask != nil {
		h.FinalMask = *cfg.FinalMask
	}
	if cfg.VlessRoute != nil {
		h.VlessRoute = *cfg.VlessRoute
	}
	if cfg.ExcludeFromSubTypes != nil {
		h.ExcludeFromSubTypes = append([]string(nil), (*cfg.ExcludeFromSubTypes)...)
	}
	if cfg.MihomoIpVersion != nil {
		h.MihomoIpVersion = *cfg.MihomoIpVersion
	}
	if cfg.MihomoX25519 != nil {
		h.MihomoX25519 = *cfg.MihomoX25519
	}
	if cfg.ShuffleHost != nil {
		h.ShuffleHost = *cfg.ShuffleHost
	}
}
