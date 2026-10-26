package sub

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/amneziawg"
	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	wgutil "github.com/SawaMEN/3x-ui/v3/internal/util/wireguard"
)

// These extensions are registered by the pinned SawaMEN Hiddify core. They
// must not use the stock sing-box compatibility gate or silently disappear
// when AnyTLS switches a mixed subscription to native JSON.
func hiddifyExtendedProtocol(protocol model.Protocol) bool {
	switch protocol {
	case model.Mieru, model.Sudoku, model.TrustTunnel, model.FPTN, model.OpenFlux, model.AmneziaWG:
		return true
	}
	return false
}

func (s *SubJsonService) genHiddifyExtended(subReq *SubService, ib *model.Inbound, client model.Client) []map[string]any {
	var result []map[string]any
	for _, ep := range subReq.shareEndpointsForInbound(ib) {
		ob := s.genHiddifyExtendedEndpoint(subReq, ib, client, ep)
		if ob == nil {
			continue
		}
		ob["tag"] = fmt.Sprintf("%s · %s", ib.Protocol, subReq.endpointRemark(ib, client.Email, ep.ep, ""))
		result = append(result, ob)
	}
	return result
}

func (s *SubJsonService) genHiddifyExtendedEndpoint(subReq *SubService, ib *model.Inbound, client model.Client, ep ShareEndpoint) map[string]any {
	settings := subReq.linkSettings(ib)
	ob := map[string]any{"type": string(ib.Protocol), "server": strings.Trim(ep.Address, "[]"), "server_port": ep.Port}
	switch ib.Protocol {
	case model.Mieru:
		if client.Password == "" {
			return nil
		}
		entries := mieruShareEntries(settings, ib.Port)
		bindings := []map[string]any{}
		seen := map[string]bool{}
		for _, entry := range entries {
			port := entry.port
			if ep.ep != nil {
				port = strconv.Itoa(ep.Port)
			}
			key := entry.protocol + ":" + port
			if seen[key] {
				continue
			}
			seen[key] = true
			binding := map[string]any{"protocol": entry.protocol}
			if strings.Contains(port, "-") {
				binding["portRange"] = port
			} else {
				n, err := strconv.Atoi(port)
				if err != nil {
					return nil
				}
				binding["port"] = n
			}
			bindings = append(bindings, binding)
		}
		if len(bindings) == 0 {
			return nil
		}
		delete(ob, "server_port") // Mieru represents every TCP/UDP port in portBindings.
		ob["username"], ob["password"], ob["portBindings"] = client.Email, client.Password, bindings
		multiplexing, _ := settings["multiplexing"].(string)
		if multiplexing == "" {
			multiplexing = "MULTIPLEXING_LOW"
		}
		handshake, _ := settings["handshakeMode"].(string)
		if handshake == "" {
			handshake = "HANDSHAKE_STANDARD"
		}
		ob["multiplexing"], ob["handshake_mode"] = multiplexing, handshake
		if mtu, ok := settings["mtu"]; ok {
			ob["mtu"] = mtu
		}
	case model.Sudoku:
		// Reuse the canonical short-link payload to preserve defaults, client keys,
		// HTTP masking and endpoint overrides in both export formats.
		clone := *ib
		stream := unmarshalStreamSettings(ib.StreamSettings)
		stream = cloneStreamForExternalProxy(stream)
		if stream == nil {
			stream = map[string]any{}
		}
		stream["externalProxy"] = []any{map[string]any{"dest": ep.Address, "port": ep.Port, "forceTls": ep.ForceTls}}
		raw, _ := json.Marshal(stream)
		clone.StreamSettings = string(raw)
		delete(subReq.streamSettingsByInbound, ib.Id)
		link := subReq.genSudokuLink(&clone, client.Email)
		delete(subReq.streamSettingsByInbound, ib.Id)
		payloadRaw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(link, "sudoku://"))
		if err != nil {
			return nil
		}
		var payload map[string]any
		if json.Unmarshal(payloadRaw, &payload) != nil {
			return nil
		}
		for source, target := range map[string]string{"k": "key", "a": "ascii", "e": "aead", "ts": "custom_tables"} {
			if v, ok := payload[source]; ok {
				ob[target] = v
			}
		}
		if table, ok := payload["t"]; ok {
			ob["custom_tables"] = []any{table}
		}
		ob["enable_pure_downlink"] = payload["x"] != true
		for source, target := range map[string]string{"paddingMin": "padding_min", "paddingMax": "padding_max"} {
			if v, ok := settings[source]; ok {
				ob[target] = v
			}
		}
		mask := map[string]any{}
		for source, target := range map[string]string{"hd": "disable", "hm": "mode", "ht": "tls", "hh": "host", "hy": "path_root", "hx": "multiplex"} {
			if v, ok := payload[source]; ok {
				mask[target] = v
			}
		}
		ob["httpmask"] = mask
	case model.TrustTunnel:
		if ep.ForceTls == "none" {
			return nil
		}
		// TrustTunnel's listener authenticates its settings-owned user/password.
		clients, err := subReq.inboundService.GetClients(ib)
		if err != nil {
			return nil
		}
		password := ""
		for _, c := range clients {
			if strings.EqualFold(c.Email, client.Email) && c.Enable {
				password = c.Password
				break
			}
		}
		if password == "" {
			return nil
		}
		ob["username"], ob["password"] = client.Email, password
		sni, _ := settings["hostname"].(string)
		if override, ok := externalProxySNI(ep.ep); ok {
			sni = override
		}
		tls := map[string]any{"enabled": true, "server_name": sni}
		applyNativeTLSHostOptions(tls, ep.ep)
		if certificate := trustTunnelSubscriptionCertificate(ib, settings); certificate != "" {
			tls["certificate"] = []string{certificate}
		}
		ob["tls"] = tls
	case model.FPTN, model.OpenFlux:
		inst, err := externalvpn.FromInbound(ib)
		if err != nil {
			return nil
		}
		link, err := externalvpn.ExportAdditionalLink(inst, client.Email, ep.Address, ep.Port, ib.Remark)
		if err != nil {
			return nil
		}
		if ib.Protocol == model.FPTN {
			if ep.ForceTls == "none" {
				return nil
			}
			token, err := externalvpn.DecodeFPTNToken(link)
			if err != nil || len(token.Servers) == 0 {
				return nil
			}
			ob["username"], ob["password"], ob["md5_fingerprint"] = token.Username, token.Password, token.Servers[0].Fingerprint
			ob["mtu"] = 1400
		} else {
			cfg, err := externalvpn.DecodeOpenFluxLink(link)
			if err != nil {
				return nil
			}
			delete(ob, "server")
			delete(ob, "server_port")
			ob["secret"], ob["context"], ob["codec"], ob["negotiate"], ob["transports"] = cfg.Secret, cfg.Context, cfg.Codec, cfg.Negotiate, cfg.Transports
		}
	case model.AmneziaWG:
		var parsed amneziawg.InboundSettings
		if json.Unmarshal([]byte(ib.Settings), &parsed) != nil || parsed.Server == nil || client.PrivateKey == "" {
			return nil
		}
		adjusted, err := amneziawg.ClientServerSettings(*parsed.Server, client.AWGClientOptions)
		if err != nil {
			return nil
		}
		server := &adjusted
		pub := server.PublicKey
		if pub == "" {
			pub, err = wgutil.PublicKeyFromPrivate(server.PrivateKey)
			if err != nil {
				return nil
			}
		}
		addresses := amneziaWGClientAddresses(parsed.Clients, client)
		if len(addresses) == 0 {
			return nil
		}
		raw, _ := json.Marshal(server)
		var source map[string]any
		_ = json.Unmarshal(raw, &source)
		awg := map[string]any{}
		for _, key := range []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "i1", "i2", "i3", "i4", "i5"} {
			if v, ok := source[key]; ok {
				awg[key] = v
			}
		}
		for i, key := range []string{"h1", "h2", "h3", "h4"} {
			value, _ := awg[key].(string)
			awg[key] = amneziaWGHeaderOrDefault(value, strconv.Itoa(i+1))
		}
		for src, dst := range map[string]string{"headerProtectionKey": "header_protection_key", "contentPaddingAddition": "content_padding_addition", "rekeyAfterTime": "rekey_after_time", "rekeyTimeout": "rekey_timeout", "rejectAfterTime": "reject_after_time", "keepaliveTimeout": "keepalive_timeout", "maxHandshakeAttempts": "max_handshake_attempts", "randomTrailers": "random_trailers", "disableCookies": "disable_cookies"} {
			if v, ok := source[src]; ok {
				awg[dst] = v
			}
		}
		peer := map[string]any{"address": strings.Trim(ep.Address, "[]"), "port": ep.Port, "public_key": pub, "allowed_ips": []string{"0.0.0.0/0", "::/0"}}
		if client.PreSharedKey != "" {
			peer["preshared_key"] = client.PreSharedKey
		}
		if client.KeepAliveSeconds() > 0 {
			peer["persistent_keepalive_interval"] = client.KeepAliveSeconds()
		}
		ob = map[string]any{"type": "awg", "private_key": client.PrivateKey, "address": addresses, "mtu": amneziawg.EffectiveMTU(server.MTU, server.S4), "awg": awg, "peers": []any{peer}}
	default:
		return nil
	}
	return ob
}

// TrustTunnel creates a local self-signed certificate by default. Export its
// public certificate as the client's trust anchor without disabling verification.
// Remote node filesystem paths belong to that node and must not be read here.
func trustTunnelSubscriptionCertificate(ib *model.Inbound, settings map[string]any) string {
	data, _ := settings["certificatePEM"].(string)
	if data == "" && ib.NodeID == nil {
		path, _ := settings["certificate"].(string)
		if path == "" {
			path = filepath.Join(config.GetBinFolderPath(), "externalvpn", strconv.Itoa(ib.Id), "cert.pem")
		}
		raw, err := os.ReadFile(path)
		if err == nil {
			data = string(raw)
		}
	}
	var result strings.Builder
	for remaining := []byte(data); len(remaining) > 0; {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		remaining = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			continue
		}
		result.Write(pem.EncodeToMemory(block))
	}
	return result.String()
}
