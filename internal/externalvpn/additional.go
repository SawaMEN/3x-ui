package externalvpn

import (
	"bytes"
	"compress/flate"
	"crypto/md5" // FPTN's token format requires MD5 certificate fingerprints.
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

const DefaultFPTNImage = "fptnvpn/fptn-vpn-server:0.4.4"

var (
	imagePattern      = regexp.MustCompile(`^fptnvpn/fptn-vpn-server:(?:[0-9]+\.[0-9]+\.[0-9]+)(?:[-a-zA-Z0-9.]+)?$`)
	metricsKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	domainPattern     = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?$`)
)

// A stable ASCII username avoids the upstream users.list whitespace restriction
// without forcing panel users to rename their existing email identities.
func FPTNUsername(email string) string {
	sum := sha256.Sum256([]byte(email))
	return "u" + hex.EncodeToString(sum[:16])
}

func PrepareAdditional(ib *model.Inbound, previous string) error {
	var raw map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil {
		return err
	}
	if raw == nil {
		raw = map[string]any{}
	}
	var old Settings
	_ = json.Unmarshal([]byte(previous), &old)
	if ib.Protocol == model.FPTN {
		if raw["image"] == nil || raw["image"] == "" {
			raw["image"] = DefaultFPTNImage
		}
		if raw["mtu"] == nil {
			raw["mtu"] = 1400
		}
		if raw["maxSessions"] == nil {
			raw["maxSessions"] = 3
		}
		if raw["hostname"] == nil || raw["hostname"] == "" {
			raw["hostname"] = "fptn.local"
		}
		if raw["metricsKey"] == nil || raw["metricsKey"] == "" {
			key := old.MetricsKey
			if key == "" {
				var err error
				key, err = GenerateSecret()
				if err != nil {
					return err
				}
			}
			raw["metricsKey"] = key
		}

		if raw["certificate"] == nil || raw["certificate"] == "" {
			cert, _ := raw["certificatePEM"].(string)
			key, _ := raw["privateKeyPEM"].(string)
			if cert == "" && old.CertificatePEM != "" {
				cert, key = old.CertificatePEM, old.PrivateKeyPEM
			}
			host, _ := raw["hostname"].(string)
			valid := false
			if pair, err := tls.X509KeyPair([]byte(cert), []byte(key)); err == nil && len(pair.Certificate) > 0 {
				if leaf, err := x509.ParseCertificate(pair.Certificate[0]); err == nil {
					_, rsaKey := pair.PrivateKey.(*rsa.PrivateKey)
					valid = rsaKey && !time.Now().Before(leaf.NotBefore) && leaf.VerifyHostname(host) == nil && time.Until(leaf.NotAfter) > 30*24*time.Hour
				}
			}
			if !valid {
				var err error
				cert, key, err = newFPTNCertificate(host)
				if err != nil {
					return err
				}
			}
			raw["certificatePEM"], raw["privateKeyPEM"] = cert, key
		}
	} else {
		if ib.Protocol == model.OpenFlux {
			if raw["transports"] == nil {
				raw["transports"] = []OpenFluxTransport{{Type: "direct", Priority: 100}}
			}
			if raw["context"] == nil || raw["context"] == "" {
				context := old.Context
				if context == "" && previous != "" {
					// Freeze the old effective KDF context before editing carriers.
					context = "http://#"
					best := -1
					for i, transport := range old.Transports {
						if transport.Type != "direct" && transport.URL != "" && (best < 0 || transport.Priority > old.Transports[best].Priority) {
							best, context = i, transport.URL
						}
					}
				}
				if context == "" {
					var err error
					context, err = GenerateSecret()
					if err != nil {
						return err
					}
				}
				raw["context"] = context
			}
		}
	}
	keys := map[string]string{}
	for _, c := range old.Clients {
		keys[c.Email] = c.Password
	}
	if clients, ok := raw["clients"].([]any); ok {
		for _, item := range clients {
			c, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid VPN client")
			}
			if c["password"] == nil || c["password"] == "" {
				email, _ := c["email"].(string)
				secret := keys[email]
				if secret == "" {
					var err error
					secret, err = GenerateSecret()
					if err != nil {
						return err
					}
				}
				c["password"] = secret
			}
		}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	ib.Settings = string(data)
	_, err = FromInbound(ib)
	return err
}

func (inst Instance) validateAdditional() error {
	s := inst.Settings
	if inst.Port < 1 || inst.Port > 65535 {
		return fmt.Errorf("%s needs a port in 1..65535", inst.Protocol)
	}
	if inst.Listen != "" && net.ParseIP(strings.Trim(inst.Listen, "[]")) == nil {
		return fmt.Errorf("listen must be an IP")
	}
	if inst.Protocol == model.FPTN {
		if s.AdsFilter {
			return fmt.Errorf("FPTN server image does not support ad filtering; use routing adblock")
		}
		if !imagePattern.MatchString(s.Image) {
			return fmt.Errorf("FPTN requires a pinned fptnvpn/fptn-vpn-server image version")
		}
		if s.MTU < 576 || s.MTU > 9000 {
			return fmt.Errorf("FPTN MTU must be 576..9000")
		}
		if s.MaxSessions < 1 || s.MaxSessions > 1000 || s.Bandwidth < 0 || s.Bandwidth > 2000 {
			return fmt.Errorf("invalid FPTN session or bandwidth limit")
		}
		if !domainPattern.MatchString(s.Hostname) {
			return fmt.Errorf("FPTN needs a certificate hostname")
		}
		if len(s.MetricsKey) < 32 || !metricsKeyPattern.MatchString(s.MetricsKey) {
			return fmt.Errorf("invalid FPTN metrics key")
		}
		if (s.Certificate == "") != (s.PrivateKey == "") {
			return fmt.Errorf("provide both certificate and private key")
		}
		for _, domain := range strings.Split(s.AllowedSNI, ",") {
			if domain != "" && !domainPattern.MatchString(domain) {
				return fmt.Errorf("invalid FPTN SNI domain")
			}
		}
	} else {
		// The upstream exit replaces its session when another client joins. Never
		// advertise independent accounts that the server cannot actually isolate.
		if len(s.Clients) > 1 {
			return fmt.Errorf("OpenFlux supports one client per inbound; create a separate inbound for each user")
		}
		if len(s.Transports) < 1 || len(s.Transports) > 9 {
			return fmt.Errorf("OpenFlux requires 1..9 distinct transports")
		}
		if s.MaxPacketSize != 0 && (s.MaxPacketSize < 1280 || s.MaxPacketSize > 65000) {
			return fmt.Errorf("OpenFlux packet size must be automatic (0) or 1280..65000")
		}
		if s.Debug < 0 || s.Debug > 2 {
			return fmt.Errorf("OpenFlux debug level must be 0..2")
		}
		seen := map[string]bool{}
		for _, t := range s.Transports {
			if seen[t.Type] {
				return fmt.Errorf("duplicate OpenFlux transport")
			}
			seen[t.Type] = true
			if t.Priority < 0 || t.Priority > 1000 {
				return fmt.Errorf("invalid transport priority")
			}
			switch t.Type {
			case "direct":
			case "oneme":
				if !validOpenFluxMAXToken(t.Token) {
					return fmt.Errorf("MAX requires a Web MAX token without whitespace, control characters or INI comments")
				}
				if t.UID != "" && !validOpenFluxMAXUID(t.UID) {
					return fmt.Errorf("MAX account ID must be a positive 64-bit decimal integer")
				}
			case "cupsonline":
				if !validOpenFluxCupsRooms(t.URL) {
					return fmt.Errorf("Cups.online needs a base64url room list from the official exit, not a plain HTTPS page")
				}
			case "yandex", "vyandex", "boards", "mailru", "avito", "telemost":
				u, err := url.Parse(t.URL)
				if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
					return fmt.Errorf("transport requires a HTTPS channel URL")
				}
			default:
				return fmt.Errorf("unsupported shareable OpenFlux transport %q", t.Type)
			}
			if strings.TrimSpace(t.URL) != t.URL || strings.ContainsAny(t.URL, "\r\n\x00#;") {
				return fmt.Errorf("URL cannot contain INI comments or control characters")
			}
		}
		if len(s.Context) > 4096 || strings.TrimSpace(s.Context) != s.Context || strings.ContainsAny(s.Context, "\r\n\x00") {
			return fmt.Errorf("invalid OpenFlux encryption context")
		}
	}
	seen := map[string]bool{}
	for _, c := range s.Clients {
		if c.Email == "" || strings.ContainsAny(c.Email, "\r\n\x00") || seen[c.Email] {
			return fmt.Errorf("VPN clients require unique names")
		}
		seen[c.Email] = true
		if inst.Protocol == model.OpenFlux && c.Enable && strings.TrimSpace(c.Password) != c.Password {
			return fmt.Errorf("OpenFlux secret cannot begin or end with whitespace")
		}
		if c.Enable {
			length, minimum := len(c.Password), 32
			if inst.Protocol == model.OpenFlux {
				// Match official CLI, Android and desktop UTF-16 length checks.
				length, minimum = len(utf16.Encode([]rune(c.Password))), 16
			}
			if length < minimum || strings.ContainsAny(c.Password, "\r\n\x00") {
				return fmt.Errorf("VPN client secret must have at least %d characters", minimum)
			}
		}
	}
	return nil
}

// ExportAdditionalLink uses the actual upstream wire formats. In particular,
// OpenFlux uses raw DEFLATE + base64url, whereas FPTN uses raw standard base64.
func ExportAdditionalLink(inst Instance, email, address string, port int, remark string) (string, error) {
	if inst.Protocol == model.VetroffProxy {
		return exportVetroffLink(inst, email, address, port)
	}
	if err := inst.Validate(); err != nil {
		return "", err
	}
	address = strings.Trim(address, "[]")
	if address == "" || port < 1 || port > 65535 {
		return "", fmt.Errorf("public endpoint required")
	}
	if ip := net.ParseIP(address); ip != nil && ip.IsUnspecified() {
		return "", fmt.Errorf("cannot advertise an unspecified address")
	}
	var client Client
	for _, c := range inst.Settings.Clients {
		if c.Enable && c.Email == email {
			client = c
			break
		}
	}
	if client.Email == "" {
		return "", fmt.Errorf("client is disabled or absent")
	}
	if inst.Protocol == model.FPTN {
		cert := inst.Settings.Certificate
		if cert == "" {
			cert = filepath.Join(directory(), fmt.Sprint(inst.ID), "cert.pem")
		}
		data := []byte(inst.Settings.CertificatePEM)
		if inst.Settings.Certificate != "" || len(data) == 0 {
			var err error
			data, err = os.ReadFile(cert)
			if err != nil {
				return "", err
			}
		}
		block, _ := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" {
			return "", fmt.Errorf("invalid FPTN certificate")
		}
		sum := md5.Sum(block.Bytes)
		token := map[string]any{"version": 1, "service_name": remark, "username": FPTNUsername(client.Email), "password": client.Password, "servers": []map[string]any{{"name": remark, "host": address, "port": port, "md5_fingerprint": hex.EncodeToString(sum[:])}}, "censored_zone_servers": []any{}}
		raw, err := json.Marshal(token)
		if err != nil {
			return "", err
		}
		return "fptn:" + base64.RawStdEncoding.EncodeToString(raw), nil
	}
	// MAX credentials belong to the server account, never to subscribers.
	// Upstream shareConfig also omits MAX: each client needs its own login.
	transports := make([]OpenFluxTransport, 0, len(inst.Settings.Transports))
	for _, carrier := range inst.Settings.Transports {
		if carrier.Type == "oneme" {
			continue
		}
		carrier.Token, carrier.UID = "", ""
		transports = append(transports, carrier)
	}
	if len(transports) == 0 {
		return "", fmt.Errorf("MAX uses a client configuration, not an openflux:// link; prepare it in the inbound's MAX client setup")
	}
	for i := range transports {
		if transports[i].Type == "direct" {
			transports[i].Dial = net.JoinHostPort(address, fmt.Sprint(port))
		}
	}
	raw, err := json.Marshal(map[string]any{"name": remark, "negotiate": true, "codec": "batched", "secret": client.Password, "context": inst.Settings.Context, "transports": transports})
	if err != nil {
		return "", err
	}
	var packed bytes.Buffer
	w, err := flate.NewWriter(&packed, flate.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(raw); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return "openflux://v1/" + base64.RawURLEncoding.EncodeToString(packed.Bytes()), nil
}

// Match the INI parser and signed int64 account identifiers used by upstream.
func validOpenFluxMAXToken(token string) bool {
	if token == "" || len(token) > 16384 {
		return false
	}
	for _, r := range token {
		if r < 33 || r > 126 || r == '#' || r == ';' {
			return false
		}
	}
	return true
}

func validOpenFluxMAXUID(uid string) bool {
	if len(uid) == 0 || len(uid) > 19 {
		return false
	}
	for _, r := range uid {
		if r < '0' || r > '9' {
			return false
		}
	}
	n, err := strconv.ParseInt(uid, 10, 64)
	return err == nil && n > 0
}

// Official Cups.online transport accepts a base64url-encoded JSON list of
// room UUIDs (or an HTTPS link with ?rooms= / ?room=), not a room page URL.
func validOpenFluxCupsRooms(raw string) bool {
	if raw == "" || len(raw) > 8192 || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n\x00#;") {
		return false
	}
	packed := raw
	if strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || u.User != nil {
			return false
		}
		if single := u.Query().Get("room"); single != "" {
			return validOpenFluxCupsRoomID(single)
		}
		packed = u.Query().Get("rooms")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(packed)
	if err != nil {
		return false
	}
	var ids []string
	if err := json.Unmarshal(decoded, &ids); err != nil || len(ids) == 0 || len(ids) > 64 {
		return false
	}
	for _, id := range ids {
		if !validOpenFluxCupsRoomID(id) {
			return false
		}
	}
	return true
}

var openFluxRoomIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-(?:[a-fA-F0-9]{4}-){3}[a-fA-F0-9]{12}package externalvpn

import (
	"bytes"
	"compress/flate"
	"crypto/md5" // FPTN's token format requires MD5 certificate fingerprints.
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

const DefaultFPTNImage = "fptnvpn/fptn-vpn-server:0.4.4"

var (
	imagePattern      = regexp.MustCompile(`^fptnvpn/fptn-vpn-server:(?:[0-9]+\.[0-9]+\.[0-9]+)(?:[-a-zA-Z0-9.]+)?$`)
	metricsKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	domainPattern     = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?$`)
)

// A stable ASCII username avoids the upstream users.list whitespace restriction
// without forcing panel users to rename their existing email identities.
func FPTNUsername(email string) string {
	sum := sha256.Sum256([]byte(email))
	return "u" + hex.EncodeToString(sum[:16])
}

func PrepareAdditional(ib *model.Inbound, previous string) error {
	var raw map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil {
		return err
	}
	if raw == nil {
		raw = map[string]any{}
	}
	var old Settings
	_ = json.Unmarshal([]byte(previous), &old)
	if ib.Protocol == model.FPTN {
		if raw["image"] == nil || raw["image"] == "" {
			raw["image"] = DefaultFPTNImage
		}
		if raw["mtu"] == nil {
			raw["mtu"] = 1400
		}
		if raw["maxSessions"] == nil {
			raw["maxSessions"] = 3
		}
		if raw["hostname"] == nil || raw["hostname"] == "" {
			raw["hostname"] = "fptn.local"
		}
		if raw["metricsKey"] == nil || raw["metricsKey"] == "" {
			key := old.MetricsKey
			if key == "" {
				var err error
				key, err = GenerateSecret()
				if err != nil {
					return err
				}
			}
			raw["metricsKey"] = key
		}

		if raw["certificate"] == nil || raw["certificate"] == "" {
			cert, _ := raw["certificatePEM"].(string)
			key, _ := raw["privateKeyPEM"].(string)
			if cert == "" && old.CertificatePEM != "" {
				cert, key = old.CertificatePEM, old.PrivateKeyPEM
			}
			host, _ := raw["hostname"].(string)
			valid := false
			if pair, err := tls.X509KeyPair([]byte(cert), []byte(key)); err == nil && len(pair.Certificate) > 0 {
				if leaf, err := x509.ParseCertificate(pair.Certificate[0]); err == nil {
					_, rsaKey := pair.PrivateKey.(*rsa.PrivateKey)
					valid = rsaKey && !time.Now().Before(leaf.NotBefore) && leaf.VerifyHostname(host) == nil && time.Until(leaf.NotAfter) > 30*24*time.Hour
				}
			}
			if !valid {
				var err error
				cert, key, err = newFPTNCertificate(host)
				if err != nil {
					return err
				}
			}
			raw["certificatePEM"], raw["privateKeyPEM"] = cert, key
		}
	} else {
		if ib.Protocol == model.OpenFlux {
			if raw["transports"] == nil {
				raw["transports"] = []OpenFluxTransport{{Type: "direct", Priority: 100}}
			}
			if raw["context"] == nil || raw["context"] == "" {
				context := old.Context
				if context == "" && previous != "" {
					// Freeze the old effective KDF context before editing carriers.
					context = "http://#"
					best := -1
					for i, transport := range old.Transports {
						if transport.Type != "direct" && transport.URL != "" && (best < 0 || transport.Priority > old.Transports[best].Priority) {
							best, context = i, transport.URL
						}
					}
				}
				if context == "" {
					var err error
					context, err = GenerateSecret()
					if err != nil {
						return err
					}
				}
				raw["context"] = context
			}
		}
	}
	keys := map[string]string{}
	for _, c := range old.Clients {
		keys[c.Email] = c.Password
	}
	if clients, ok := raw["clients"].([]any); ok {
		for _, item := range clients {
			c, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid VPN client")
			}
			if c["password"] == nil || c["password"] == "" {
				email, _ := c["email"].(string)
				secret := keys[email]
				if secret == "" {
					var err error
					secret, err = GenerateSecret()
					if err != nil {
						return err
					}
				}
				c["password"] = secret
			}
		}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	ib.Settings = string(data)
	_, err = FromInbound(ib)
	return err
}

func (inst Instance) validateAdditional() error {
	s := inst.Settings
	if inst.Port < 1 || inst.Port > 65535 {
		return fmt.Errorf("%s needs a port in 1..65535", inst.Protocol)
	}
	if inst.Listen != "" && net.ParseIP(strings.Trim(inst.Listen, "[]")) == nil {
		return fmt.Errorf("listen must be an IP")
	}
	if inst.Protocol == model.FPTN {
		if s.AdsFilter {
			return fmt.Errorf("FPTN server image does not support ad filtering; use routing adblock")
		}
		if !imagePattern.MatchString(s.Image) {
			return fmt.Errorf("FPTN requires a pinned fptnvpn/fptn-vpn-server image version")
		}
		if s.MTU < 576 || s.MTU > 9000 {
			return fmt.Errorf("FPTN MTU must be 576..9000")
		}
		if s.MaxSessions < 1 || s.MaxSessions > 1000 || s.Bandwidth < 0 || s.Bandwidth > 2000 {
			return fmt.Errorf("invalid FPTN session or bandwidth limit")
		}
		if !domainPattern.MatchString(s.Hostname) {
			return fmt.Errorf("FPTN needs a certificate hostname")
		}
		if len(s.MetricsKey) < 32 || !metricsKeyPattern.MatchString(s.MetricsKey) {
			return fmt.Errorf("invalid FPTN metrics key")
		}
		if (s.Certificate == "") != (s.PrivateKey == "") {
			return fmt.Errorf("provide both certificate and private key")
		}
		for _, domain := range strings.Split(s.AllowedSNI, ",") {
			if domain != "" && !domainPattern.MatchString(domain) {
				return fmt.Errorf("invalid FPTN SNI domain")
			}
		}
	} else {
		// The upstream exit replaces its session when another client joins. Never
		// advertise independent accounts that the server cannot actually isolate.
		if len(s.Clients) > 1 {
			return fmt.Errorf("OpenFlux supports one client per inbound; create a separate inbound for each user")
		}
		if len(s.Transports) < 1 || len(s.Transports) > 9 {
			return fmt.Errorf("OpenFlux requires 1..9 distinct transports")
		}
		if s.MaxPacketSize != 0 && (s.MaxPacketSize < 1280 || s.MaxPacketSize > 65000) {
			return fmt.Errorf("OpenFlux packet size must be automatic (0) or 1280..65000")
		}
		if s.Debug < 0 || s.Debug > 2 {
			return fmt.Errorf("OpenFlux debug level must be 0..2")
		}
		seen := map[string]bool{}
		for _, t := range s.Transports {
			if seen[t.Type] {
				return fmt.Errorf("duplicate OpenFlux transport")
			}
			seen[t.Type] = true
			if t.Priority < 0 || t.Priority > 1000 {
				return fmt.Errorf("invalid transport priority")
			}
			switch t.Type {
			case "direct":
			case "oneme":
				if !validOpenFluxMAXToken(t.Token) {
					return fmt.Errorf("MAX requires a Web MAX token without whitespace, control characters or INI comments")
				}
				if t.UID != "" && !validOpenFluxMAXUID(t.UID) {
					return fmt.Errorf("MAX account ID must be a positive 64-bit decimal integer")
				}
			case "yandex", "vyandex", "boards", "mailru", "cupsonline", "avito", "telemost":
				u, err := url.Parse(t.URL)
				if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
					return fmt.Errorf("transport requires a HTTPS channel URL")
				}
			default:
				return fmt.Errorf("unsupported shareable OpenFlux transport %q", t.Type)
			}
			if strings.TrimSpace(t.URL) != t.URL || strings.ContainsAny(t.URL, "\r\n\x00#;") {
				return fmt.Errorf("URL cannot contain INI comments or control characters")
			}
		}
		if len(s.Context) > 4096 || strings.TrimSpace(s.Context) != s.Context || strings.ContainsAny(s.Context, "\r\n\x00") {
			return fmt.Errorf("invalid OpenFlux encryption context")
		}
	}
	seen := map[string]bool{}
	for _, c := range s.Clients {
		if c.Email == "" || strings.ContainsAny(c.Email, "\r\n\x00") || seen[c.Email] {
			return fmt.Errorf("VPN clients require unique names")
		}
		seen[c.Email] = true
		if inst.Protocol == model.OpenFlux && c.Enable && strings.TrimSpace(c.Password) != c.Password {
			return fmt.Errorf("OpenFlux secret cannot begin or end with whitespace")
		}
		if c.Enable {
			length, minimum := len(c.Password), 32
			if inst.Protocol == model.OpenFlux {
				// Match official CLI, Android and desktop UTF-16 length checks.
				length, minimum = len(utf16.Encode([]rune(c.Password))), 16
			}
			if length < minimum || strings.ContainsAny(c.Password, "\r\n\x00") {
				return fmt.Errorf("VPN client secret must have at least %d characters", minimum)
			}
		}
	}
	return nil
}

// ExportAdditionalLink uses the actual upstream wire formats. In particular,
// OpenFlux uses raw DEFLATE + base64url, whereas FPTN uses raw standard base64.
func ExportAdditionalLink(inst Instance, email, address string, port int, remark string) (string, error) {
	if inst.Protocol == model.VetroffProxy {
		return exportVetroffLink(inst, email, address, port)
	}
	if err := inst.Validate(); err != nil {
		return "", err
	}
	address = strings.Trim(address, "[]")
	if address == "" || port < 1 || port > 65535 {
		return "", fmt.Errorf("public endpoint required")
	}
	if ip := net.ParseIP(address); ip != nil && ip.IsUnspecified() {
		return "", fmt.Errorf("cannot advertise an unspecified address")
	}
	var client Client
	for _, c := range inst.Settings.Clients {
		if c.Enable && c.Email == email {
			client = c
			break
		}
	}
	if client.Email == "" {
		return "", fmt.Errorf("client is disabled or absent")
	}
	if inst.Protocol == model.FPTN {
		cert := inst.Settings.Certificate
		if cert == "" {
			cert = filepath.Join(directory(), fmt.Sprint(inst.ID), "cert.pem")
		}
		data := []byte(inst.Settings.CertificatePEM)
		if inst.Settings.Certificate != "" || len(data) == 0 {
			var err error
			data, err = os.ReadFile(cert)
			if err != nil {
				return "", err
			}
		}
		block, _ := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" {
			return "", fmt.Errorf("invalid FPTN certificate")
		}
		sum := md5.Sum(block.Bytes)
		token := map[string]any{"version": 1, "service_name": remark, "username": FPTNUsername(client.Email), "password": client.Password, "servers": []map[string]any{{"name": remark, "host": address, "port": port, "md5_fingerprint": hex.EncodeToString(sum[:])}}, "censored_zone_servers": []any{}}
		raw, err := json.Marshal(token)
		if err != nil {
			return "", err
		}
		return "fptn:" + base64.RawStdEncoding.EncodeToString(raw), nil
	}
	// MAX credentials belong to the server account, never to subscribers.
	// Upstream shareConfig also omits MAX: each client needs its own login.
	transports := make([]OpenFluxTransport, 0, len(inst.Settings.Transports))
	for _, carrier := range inst.Settings.Transports {
		if carrier.Type == "oneme" {
			continue
		}
		carrier.Token, carrier.UID = "", ""
		transports = append(transports, carrier)
	}
	if len(transports) == 0 {
		return "", fmt.Errorf("MAX uses a client configuration, not an openflux:// link; prepare it in the inbound's MAX client setup")
	}
	for i := range transports {
		if transports[i].Type == "direct" {
			transports[i].Dial = net.JoinHostPort(address, fmt.Sprint(port))
		}
	}
	raw, err := json.Marshal(map[string]any{"name": remark, "negotiate": true, "codec": "batched", "secret": client.Password, "context": inst.Settings.Context, "transports": transports})
	if err != nil {
		return "", err
	}
	var packed bytes.Buffer
	w, err := flate.NewWriter(&packed, flate.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(raw); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return "openflux://v1/" + base64.RawURLEncoding.EncodeToString(packed.Bytes()), nil
}

// Match the INI parser and signed int64 account identifiers used by upstream.
func validOpenFluxMAXToken(token string) bool {
	if token == "" || len(token) > 16384 {
		return false
	}
	for _, r := range token {
		if r < 33 || r > 126 || r == '#' || r == ';' {
			return false
		}
	}
	return true
}

func validOpenFluxMAXUID(uid string) bool {
	if len(uid) == 0 || len(uid) > 19 {
		return false
	}
	for _, r := range uid {
		if r < '0' || r > '9' {
			return false
		}
	}
	n, err := strconv.ParseInt(uid, 10, 64)
	return err == nil && n > 0
}

)

func validOpenFluxCupsRoomID(id string) bool {
	return openFluxRoomIDPattern.MatchString(id)
}

// Write credentials only to a private runtime config, never to process arguments.
func writeOpenFluxTransport(conf *strings.Builder, carrier OpenFluxTransport, endpoint string, exit bool) {
	fmt.Fprintf(conf, "\n[Transport \"%s\"]\nType = %s\nPriority = %d\n", carrier.Type, carrier.Type, carrier.Priority)
	switch carrier.Type {
	case "direct":
		key := "Dial"
		if exit {
			key = "Listen"
		}
		fmt.Fprintf(conf, "%s = %s\n", key, endpoint)
	case "oneme":
		fmt.Fprintf(conf, "Token = %s\nUID = %s\n", carrier.Token, carrier.UID)
	default:
		fmt.Fprintf(conf, "URL = %s\n", carrier.URL)
	}
}
