package model

import "fmt"

// Mbps are decimal megabits per second; zero leaves that direction unlimited.
type ClientSpeedLimit struct {
	Enabled      bool `json:"enabled"`
	UploadMbps   int  `json:"uploadMbps"`
	DownloadMbps int  `json:"downloadMbps"`
}

func (s *ClientSpeedLimit) Validate() error {
	if s == nil {
		return nil
	}
	if s.UploadMbps < 0 || s.DownloadMbps < 0 || s.UploadMbps > 100000 || s.DownloadMbps > 100000 {
		return fmt.Errorf("client speed limits must be between 0 and 100000 Mbps")
	}
	if s.Enabled && s.UploadMbps == 0 && s.DownloadMbps == 0 {
		return fmt.Errorf("an enabled client speed limit requires a positive upload or download rate")
	}
	return nil
}

func SupportsClientSpeedLimit(protocol Protocol) bool {
	switch protocol {
	case VMESS, VLESS, HTTP, Trojan, Shadowsocks, Mixed, Hysteria, TUIC, NaiveProxy, AnyTLS, Snell, ShadowTLS:
		return true
	default:
		return false
	}
}
