package externalvpn

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"unicode/utf8"

	"github.com/SawaMEN/3x-ui/v3/internal/sudoku"
)

// The official Sudoku client exposes a wildcard-bound mixed HTTP/SOCKS port.
// Run it only in an isolated container and publish the bridge on host loopback.
// Never load the short link's local port or PAC rules for a managed node route.
type sudokuLinkPayload struct {
	Host            string   `json:"h"`
	Port            int      `json:"p"`
	Key             string   `json:"k"`
	ASCII           string   `json:"a"`
	AEAD            string   `json:"e"`
	MixedPort       int      `json:"m"`
	Packed          bool     `json:"x"`
	CustomTable     string   `json:"t"`
	CustomTables    []string `json:"ts"`
	DisableHTTPMask bool     `json:"hd"`
	HTTPMaskMode    string   `json:"hm"`
	HTTPMaskTLS     bool     `json:"ht"`
	HTTPMaskHost    string   `json:"hh"`
	HTTPMaskMux     string   `json:"hx"`
	HTTPMaskPath    string   `json:"hy"`
	Name            string   `json:"n"`
}

func validSudokuHost(host string) bool {
	if host == "" || len(host) > 253 || strings.TrimSpace(host) != host {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, part := range strings.Split(host, ".") {
		if len(part) == 0 || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for _, r := range part {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

func sudokuOutboundConfig(link string) ([]byte, error) {
	const invalid = "invalid Sudoku outbound connection link"
	if len(link) > 16<<10 || !strings.HasPrefix(link, "sudoku://") {
		return nil, fmt.Errorf("%s", invalid)
	}
	encoded := strings.TrimPrefix(link, "sudoku://")
	if encoded == "" || strings.ContainsAny(encoded, "/?# \t\r\n") {
		return nil, fmt.Errorf("%s", invalid)
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || !utf8.Valid(raw) {
		return nil, fmt.Errorf("%s", invalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var p sudokuLinkPayload
	if decoder.Decode(&p) != nil {
		return nil, fmt.Errorf("%s", invalid)
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, fmt.Errorf("%s", invalid)
	}
	if !validSudokuHost(p.Host) || p.Port < 1 || p.Port > 65535 {
		return nil, fmt.Errorf("%s: invalid server address", invalid)
	}
	key, err := hex.DecodeString(p.Key)
	if err != nil || (len(key) != 32 && len(key) != 64) {
		return nil, fmt.Errorf("%s: invalid private key", invalid)
	}
	if p.AEAD == "" {
		p.AEAD = "chacha20-poly1305"
	}
	switch p.AEAD {
	case "chacha20-poly1305", "aes-128-gcm", "none":
	default:
		return nil, fmt.Errorf("%s: unsupported AEAD", invalid)
	}
	switch p.ASCII {
	case "", "entropy", "prefer_entropy":
		p.ASCII = "prefer_entropy"
	case "ascii", "prefer_ascii":
		p.ASCII = "prefer_ascii"
	case "up_ascii_down_entropy", "up_entropy_down_ascii":
	default:
		return nil, fmt.Errorf("%s: unsupported ASCII mode", invalid)
	}
	switch p.HTTPMaskMode {
	case "", "legacy", "auto", "ws", "stream", "poll":
	default:
		return nil, fmt.Errorf("%s: unsupported HTTP mask", invalid)
	}
	switch p.HTTPMaskMux {
	case "", "off", "auto", "on":
	default:
		return nil, fmt.Errorf("%s: unsupported multiplexing", invalid)
	}
	if strings.ContainsAny(p.HTTPMaskHost, "\r\n\x00") || len(p.HTTPMaskHost) > 253 ||
		strings.ContainsAny(p.HTTPMaskPath, "\r\n\x00") || len(p.HTTPMaskPath) > 1024 ||
		len(p.CustomTable) > 256 || len(p.CustomTables) > 16 {
		return nil, fmt.Errorf("%s: invalid client settings", invalid)
	}
	for _, table := range p.CustomTables {
		if len(table) > 256 {
			return nil, fmt.Errorf("%s: invalid custom table", invalid)
		}
	}
	cfg := sudoku.Config{
		Mode: "client", Transport: "tcp", LocalPort: 1080,
		ServerAddress: net.JoinHostPort(p.Host, fmt.Sprint(p.Port)),
		Key: p.Key, AEAD: p.AEAD, ASCII: p.ASCII,
		PaddingMin: 5, PaddingMax: 15,
		CustomTable: p.CustomTable, CustomTables: p.CustomTables,
		EnablePureDownlink: !p.Packed, Multiplex: p.HTTPMaskMux,
		HTTPMask: sudoku.HTTPMaskConfig{
			Disable: p.DisableHTTPMask, Mode: p.HTTPMaskMode,
			TLS: p.HTTPMaskTLS, Host: p.HTTPMaskHost, PathRoot: p.HTTPMaskPath,
			Multiplex: p.HTTPMaskMux,
		},
	}
	// The official client may use PAC or DIRECT for imported links. Managed
	// routes must have no bypass of the selected node, even for private IPs.
	return json.Marshal(struct {
		sudoku.Config
		ProxyMode string   `json:"proxy_mode"`
		RuleURLs  []string `json:"rule_urls"`
	}{Config: cfg, ProxyMode: "global", RuleURLs: []string{}})
}

func validateSudokuOutbound(s OutboundSettings) error {
	if s.EnableUDP || s.UDPStream {
		return fmt.Errorf("Sudoku managed node routes do not support UDP")
	}
	_, err := sudokuOutboundConfig(s.Link)
	return err
}
