package externalvpn

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

type Client struct {
	ExpiryTime int64  `json:"expiryTime,omitempty"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	Enable     bool   `json:"enable"`
}

type OpenFluxTransport struct {
	Token    string `json:"token,omitempty"`
	UID      string `json:"uid,omitempty"`
	Type     string `json:"type"`
	URL      string `json:"url,omitempty"`
	Priority int    `json:"priority,omitempty"`
	Dial     string `json:"dial,omitempty"`
}

type Settings struct {
	Vetroff         *VetroffSettings    `json:"vetroff,omitempty"`
	CertificatePEM  string              `json:"certificatePEM"`
	PrivateKeyPEM   string              `json:"privateKeyPEM"`
	Image           string              `json:"image"`
	MTU             int                 `json:"mtu"`
	MaxSessions     int                 `json:"maxSessions"`
	Bandwidth       int                 `json:"bandwidth"`
	DetectProbing   bool                `json:"detectProbing"`
	AllowedSNI      string              `json:"allowedSni"`
	AdsFilter       bool                `json:"adsFilter"`
	TorrentFilter   bool                `json:"torrentFilter"`
	SpamFilter      bool                `json:"spamFilter"`
	MetricsKey      string              `json:"metricsKey"`
	MaxPacketSize   int                 `json:"maxPacketSize,omitempty"`
	RefusePeerLogin bool                `json:"refusePeerLogin,omitempty"`
	Debug           int                 `json:"debug,omitempty"`
	Context         string              `json:"context"`
	Transports      []OpenFluxTransport `json:"transports"`

	Password       string   `json:"password"`
	Key            int      `json:"key"` // Legacy v2, migrated on save
	Encrypt        string   `json:"encrypt"`
	EncryptKey     string   `json:"encryptKey"`
	MaxConn        int      `json:"maxConn"`
	ConnectTimeout int      `json:"connectTimeout"`
	Forward        string   `json:"forward"`
	Congestion     string   `json:"congestion"`
	Hostname       string   `json:"hostname"`
	Certificate    string   `json:"certificate"`
	PrivateKey     string   `json:"privateKey"`
	Clients        []Client `json:"clients"`
}

type Instance struct {
	Revision       string `json:"-"`
	ClientRevision string `json:"-"`
	ID             int
	Tag            string
	Protocol       model.Protocol
	Listen         string
	Port           int
	Settings       Settings
}

func FromInbound(ib *model.Inbound) (Instance, error) {
	if ib == nil || !IsManaged(ib.Protocol) {
		return Instance{}, fmt.Errorf("unsupported external VPN inbound")
	}
	if ib.Protocol == model.VetroffProxy {
		if err := validateVetroffPanelLimits(ib); err != nil {
			return Instance{}, err
		}
	}
	var settings Settings
	if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
		return Instance{}, fmt.Errorf("%s settings: %w", ib.Protocol, err)
	}
	inst := Instance{Revision: sidecartraffic.RevisionOf(ib), ClientRevision: ib.RuntimeClientRevision, ID: ib.Id, Tag: ib.Tag, Protocol: ib.Protocol, Listen: ib.Listen, Port: ib.Port, Settings: settings}
	return inst, inst.Validate()
}

func (inst Instance) Validate() error {
	if inst.Protocol == model.VetroffProxy {
		return validateVetroff(inst)
	}
	s := inst.Settings
	if inst.Protocol == model.FPTN || inst.Protocol == model.OpenFlux {
		return inst.validateAdditional()
	}
	if inst.Protocol == model.Pingtunnel {
		if inst.Port != 0 {
			return fmt.Errorf("Pingtunnel uses ICMP and requires port 0")
		}
		if len(inst.PingtunnelPassword()) < 8 {
			return fmt.Errorf("Pingtunnel 3.x requires a shared password of at least 8 characters; save the inbound to generate one")
		}
		if s.Encrypt != "" && s.Encrypt != "chacha20" && s.Encrypt != "aes256" && s.Encrypt != "aes128" {
			return fmt.Errorf("Pingtunnel 3.x encryption must be chacha20, aes256 or aes128")
		}
		if s.MaxConn < 0 || s.ConnectTimeout < 0 {
			return fmt.Errorf("Pingtunnel connection limit and timeout must be nonnegative")
		}
		if s.Congestion != "" && s.Congestion != "bb" && s.Congestion != "none" {
			return fmt.Errorf("Pingtunnel congestion must be bb or none")
		}
		if inst.Listen != "" && net.ParseIP(inst.Listen) == nil {
			return fmt.Errorf("Pingtunnel listen must be an IP address")
		}
		return nil
	}
	if inst.Port < 1 || inst.Port > 65535 {
		return fmt.Errorf("TrustTunnel requires a TCP port")
	}
	if inst.Listen != "" && net.ParseIP(inst.Listen) == nil {
		return fmt.Errorf("TrustTunnel listen must be an IP address")
	}
	host := strings.TrimSpace(s.Hostname)
	if host == "" || strings.ContainsAny(host, " \t\r\n/:\\\"") {
		return fmt.Errorf("TrustTunnel hostname is required")
	}
	if (s.Certificate == "") != (s.PrivateKey == "") {
		return fmt.Errorf("TrustTunnel certificate and privateKey must both be provided")
	}
	seen := make(map[string]bool)
	for _, c := range s.Clients {
		if !c.Enable {
			continue
		}
		if c.Email == "" || strings.ContainsAny(c.Email, "\r\n\x00") || c.Password == "" || strings.ContainsAny(c.Password, "\r\n\x00") {
			return fmt.Errorf("TrustTunnel clients need an email and password without control characters")
		}
		if seen[c.Email] {
			return fmt.Errorf("duplicate TrustTunnel client %q", c.Email)
		}
		seen[c.Email] = true
	}
	return nil
}

// PingtunnelPassword keeps existing v2 inbounds functional when their
// encryption key meets the v3 shared-secret requirement.
// prepareExternalVPN removes legacy fields on the next save.
func (inst Instance) PingtunnelPassword() string {
	if inst.Settings.Password != "" {
		return inst.Settings.Password
	}
	return inst.Settings.EncryptKey
}

func (inst Instance) Bind() string {
	addr := strings.Trim(inst.Listen, "[]")
	if addr == "" {
		addr = "0.0.0.0"
	}
	return net.JoinHostPort(addr, strconv.Itoa(inst.Port))
}

func GeneratePingtunnelKey() (int, error) {
	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return 0, err
	}
	return int((uint32(bytes[0])<<24|uint32(bytes[1])<<16|uint32(bytes[2])<<8|uint32(bytes[3]))%2147483647) + 1, nil
}

func GenerateSecret() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	const alphabet = "0123456789abcdef"
	var out [64]byte
	for i, b := range bytes {
		out[i*2], out[i*2+1] = alphabet[b>>4], alphabet[b&15]
	}
	return string(out[:]), nil
}

func IsManaged(p model.Protocol) bool {
	return p == model.VetroffProxy || p == model.Pingtunnel || p == model.TrustTunnel || p == model.FPTN || p == model.OpenFlux
}
