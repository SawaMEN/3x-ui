package service

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	wgutil "github.com/SawaMEN/3x-ui/v3/internal/util/wireguard"
)

func managerEntrySource(e model.ManagerEntry, owner int) (*model.Inbound, error) {
	if e.Mode != "generate" {
		var ib model.Inbound
		if err := database.GetDB().Where("id = ? AND user_id = ?", e.TemplateID, owner).First(&ib).Error; err != nil {
			return nil, err
		}
		if strings.Contains(ib.Tag, "manager-") {
			return nil, fmt.Errorf("generated inbounds cannot be templates")
		}
		return &ib, nil
	}
	if e.Config == nil {
		return nil, fmt.Errorf("automatic generation requires a protocol configuration")
	}
	c := e.Config
	switch c.Protocol {
	case model.VLESS, model.VMESS, model.Trojan, model.Shadowsocks, model.Hysteria, model.WireGuard, model.MTProto, model.AmneziaWG, model.TUIC, model.TrustTunnel, model.VetroffProxy, model.FPTN, model.OpenFlux, model.MASQUE, model.NaiveProxy, model.Snell, model.AnyTLS, model.ShadowTLS, model.Mieru, model.VKTurnProxy, model.Sudoku:
	default:
		return nil, fmt.Errorf("protocol %q has no client generation support", c.Protocol)
	}
	for _, raw := range []string{c.Settings, c.StreamSettings, c.Sniffing} {
		if len(raw) > 1<<20 {
			return nil, fmt.Errorf("protocol configuration exceeds 1 MiB")
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var value map[string]any
		if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
			return nil, fmt.Errorf("protocol settings must be JSON objects")
		}
	}
	ib := &model.Inbound{UserId: owner, Protocol: c.Protocol, Settings: c.Settings, StreamSettings: c.StreamSettings, Sniffing: c.Sniffing, Listen: c.Listen, DisableFlow: c.DisableFlow, ShareAddrStrategy: c.ShareAddrStrategy, ShareAddr: c.ShareAddr, SubSortIndex: c.SubSortIndex}
	if strings.TrimSpace(ib.Settings) == "" {
		ib.Settings = "{}"
	}
	if e.NodeID != nil && *e.NodeID > 0 {
		ib.NodeID = e.NodeID
	}
	if err := validateInboundTLSCertificates(ib.StreamSettings); err != nil {
		return nil, err
	}
	return ib, nil
}

func managerRandomSecret(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func managerFreshServerKeys(ib *model.Inbound) error {
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
		return err
	}
	switch ib.Protocol {
	case model.WireGuard:
		key, _, err := wgutil.GenerateWireguardKeypair()
		if err != nil {
			return err
		}
		settings["secretKey"] = key
	case model.AmneziaWG:
		if server, ok := settings["server"].(map[string]any); ok {
			key, pub, err := wgutil.GenerateWireguardKeypair()
			if err != nil {
				return err
			}
			server["privateKey"], server["publicKey"] = key, pub
		}
	case model.Shadowsocks:
		size := 32
		if settings["method"] == "2022-blake3-aes-128-gcm" {
			size = 16
		}
		secret, err := managerRandomSecret(size)
		if err != nil {
			return err
		}
		settings["password"] = secret
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	ib.Settings = string(raw)
	if strings.TrimSpace(ib.StreamSettings) == "" {
		return nil
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(ib.StreamSettings), &stream); err != nil {
		return err
	}
	if stream["security"] != "reality" {
		return nil
	}
	reality, ok := stream["realitySettings"].(map[string]any)
	if !ok {
		return fmt.Errorf("Reality settings are required")
	}
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	reality["privateKey"] = base64.RawURLEncoding.EncodeToString(key.Bytes())
	client, ok := reality["settings"].(map[string]any)
	if !ok {
		client = map[string]any{}
		reality["settings"] = client
	}
	client["publicKey"] = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	short := make([]byte, 8)
	if _, err := rand.Read(short); err != nil {
		return err
	}
	reality["shortIds"] = []string{hex.EncodeToString(short)}
	raw, err = json.Marshal(stream)
	if err == nil {
		ib.StreamSettings = string(raw)
	}
	return err
}
