package model

import "github.com/SawaMEN/3x-ui/v3/internal/core"

type InboundProtocolCapability struct {
	ClientAttachable bool   `json:"clientAttachable"`
	NodeEligible     bool   `json:"nodeEligible"`
	Xray             bool   `json:"xray"`
	Upstream         bool   `json:"upstream"`
	Native           bool   `json:"native"`
	SpeedLimit       bool   `json:"speedLimit"`
	Component        string `json:"component"`
}

var InboundProtocolCapabilities = map[Protocol]InboundProtocolCapability{
	VMESS:        {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: true, Component: ""},
	VLESS:        {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: true, Component: ""},
	Trojan:       {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: true, Component: ""},
	Shadowsocks:  {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: true, Component: ""},
	Hysteria:     {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: true, Component: ""},
	WireGuard:    {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: false, Component: ""},
	MTProto:      {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: true, Component: "telemt"},
	AmneziaWG:    {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: false, Component: ""},
	TUIC:         {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: true, Component: ""},
	TrustTunnel:  {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: false, Component: "trusttunnel_endpoint"},
	VetroffProxy: {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: false, Component: "vetroff"},
	FPTN:         {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: false, Component: "fptn"},
	OpenFlux:     {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: false, Component: "openflux"},
	MASQUE:       {ClientAttachable: true, NodeEligible: true, Xray: false, Upstream: false, Native: true, SpeedLimit: true, Component: ""},
	NaiveProxy:   {ClientAttachable: true, NodeEligible: true, Xray: false, Upstream: true, Native: true, SpeedLimit: true, Component: ""},
	Snell:        {ClientAttachable: true, NodeEligible: true, Xray: false, Upstream: false, Native: true, SpeedLimit: true, Component: ""},
	AnyTLS:       {ClientAttachable: true, NodeEligible: true, Xray: false, Upstream: true, Native: true, SpeedLimit: true, Component: ""},
	ShadowTLS:    {ClientAttachable: true, NodeEligible: true, Xray: false, Upstream: true, Native: true, SpeedLimit: true, Component: ""},
	Mieru:        {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: true, Component: "mita"},
	VKTurnProxy:  {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: false, Component: "vk-turn-proxy"},
	Sudoku:       {ClientAttachable: true, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: true, Component: "sudoku-tunnel"},
	HTTP:         {ClientAttachable: false, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: true, Component: ""},
	Mixed:        {ClientAttachable: false, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: true, Component: ""},
	Tunnel:       {ClientAttachable: false, NodeEligible: true, Xray: true, Upstream: false, Native: false, SpeedLimit: false, Component: ""},
	Tun:          {ClientAttachable: false, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: false, Component: ""},
	Pingtunnel:   {ClientAttachable: false, NodeEligible: true, Xray: true, Upstream: true, Native: true, SpeedLimit: false, Component: "pingtunnel"},
}

func (c InboundProtocolCapability) SupportsCore(kind core.Type) bool {
	switch kind {
	case core.Xray:
		return c.Xray
	case core.SingBox:
		return c.Native
	case core.SingBoxUpstream:
		return c.Upstream
	default:
		return false
	}
}

type ProtocolAvailability struct {
	Supported  bool   `json:"supported"`
	Ready      bool   `json:"ready"`
	Version    string `json:"version"`
	SpeedLimit bool   `json:"speedLimit"`
	Component  string `json:"component"`
}
