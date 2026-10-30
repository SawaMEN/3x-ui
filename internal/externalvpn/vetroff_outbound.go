package externalvpn

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The upstream CLI is the source of truth for full profile validation. This
// preflight rejects malformed/untrusted links without logging credentials or
// requiring a downloaded sidecar just to save an outbound configuration.
func validateVetroffOutbound(s OutboundSettings) error {
	const bad = "invalid VetroffProxy outbound connection link"
	if s.UDPStream && !s.EnableUDP {
		return fmt.Errorf("VetroffProxy UDP streaming requires enableUDP")
	}
	if len(s.Link) > 16<<10 || !strings.HasPrefix(s.Link, "vetroff://") || strings.ContainsAny(s.Link, "/?# \t\r\n") {
		// The two slashes immediately after the URI scheme are expected.
		if !strings.HasPrefix(s.Link, "vetroff://") || strings.ContainsAny(strings.TrimPrefix(s.Link, "vetroff://"), "/?# \t\r\n") || len(s.Link) > 16<<10 {
			return fmt.Errorf("%s", bad)
		}
	}
	encoded := strings.TrimPrefix(s.Link, "vetroff://")
	if encoded == "" {
		return fmt.Errorf("%s", bad)
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || !utf8.Valid(raw) {
		return fmt.Errorf("%s", bad)
	}
	// Duplicate JSON fields are rejected by the official profile parser,
	// including case variants. Do the same before any data reaches the CLI.
	fields := json.NewDecoder(bytes.NewReader(raw))
	start, err := fields.Token()
	if err != nil || start != json.Delim('{') {
		return fmt.Errorf("%s", bad)
	}
	seen := make(map[string]bool)
	for fields.More() {
		key, err := fields.Token()
		name, ok := key.(string)
		if err != nil || !ok || seen[strings.ToLower(name)] {
			return fmt.Errorf("%s", bad)
		}
		seen[strings.ToLower(name)] = true
		var value json.RawMessage
		if fields.Decode(&value) != nil {
			return fmt.Errorf("%s", bad)
		}
	}
	end, err := fields.Token()
	if err != nil || end != json.Delim('}') {
		return fmt.Errorf("%s", bad)
	}
	var extra any
	if fields.Decode(&extra) != io.EOF {
		return fmt.Errorf("%s", bad)
	}
	var profile struct {
		Version        int      `json:"v"`
		Servers        []string `json:"servers"`
		Username       string   `json:"username"`
		Password       string   `json:"password"`
		Transport      string   `json:"transport"`
		TLSBackend     string   `json:"tls"`
		SNI            string   `json:"sni"`
		Pin            string   `json:"pin_sha256"`
		Encryption     string   `json:"encryption"`
		Balance        string   `json:"balance"`
		Bond           bool     `json:"bond"`
		Discover       bool     `json:"discover"`
		Pipeline       string   `json:"pipeline"`
		Padding        bool     `json:"padding"`
		ConnectTimeout string   `json:"connect_timeout"`
		AttemptTimeout string   `json:"attempt_timeout"`
		ProtocolPolicy string   `json:"protocol_policy"`
		XHTTPPath      string   `json:"xhttp_path"`
		XHTTPHost      string   `json:"xhttp_host"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&profile) != nil || decoder.Decode(&extra) != io.EOF {
		return fmt.Errorf("%s", bad)
	}
	if profile.Version != 1 || len(profile.Servers) == 0 || len(profile.Servers) > 8 || profile.Username == "" || profile.Password == "" {
		return fmt.Errorf("%s", bad)
	}
	origins := make(map[string]bool)
	for _, server := range profile.Servers {
		u, err := url.Parse(server)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
			u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
			return fmt.Errorf("%s", bad)
		}
		if port := u.Port(); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("%s", bad)
			}
		}
		origin := strings.ToLower(u.Hostname())
		if u.Port() != "" && u.Port() != "443" {
			origin += ":" + u.Port()
		}
		if origins[origin] {
			return fmt.Errorf("%s", bad)
		}
		origins[origin] = true
	}
	switch profile.Transport {
	case "", "h2-connect", "h2-post", "h3-connect", "h3-post", "wss", "xhttp-stream-one", "xhttp-stream-up", "xhttp-packet-up":
	default:
		return fmt.Errorf("%s", bad)
	}
	switch profile.TLSBackend {
	case "", "auto", "go", "nss":
	default:
		return fmt.Errorf("%s", bad)
	}
	switch profile.Encryption {
	case "", "tls13":
	default:
		return fmt.Errorf("%s", bad)
	}
	switch profile.ProtocolPolicy {
	case "", "builtin", "strict":
	default:
		return fmt.Errorf("%s", bad)
	}
	switch profile.Balance {
	case "", "failover", "round-robin", "least-active", "adaptive":
	default:
		return fmt.Errorf("%s", bad)
	}
	switch profile.Pipeline {
	case "", "auto", "4", "8":
	default:
		return fmt.Errorf("%s", bad)
	}
	if (strings.HasPrefix(profile.Transport, "h3-") && profile.TLSBackend == "nss") ||
		(profile.ProtocolPolicy == "strict" && (profile.Bond || profile.Discover)) ||
		(profile.Discover && (!profile.Bond || len(profile.Servers) != 1)) ||
		(profile.Bond && (!profile.Discover && len(profile.Servers) < 2 || profile.Padding || profile.Balance == "adaptive" || s.EnableUDP)) {
		return fmt.Errorf("%s", bad)
	}
	if profile.Pin != "" {
		if !strings.HasPrefix(profile.Pin, "sha256/") {
			return fmt.Errorf("%s", bad)
		}
		pin, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(profile.Pin, "sha256/"))
		if err != nil || len(pin) != 32 {
			return fmt.Errorf("%s", bad)
		}
	}
	for _, item := range []struct {
		value string
		max   time.Duration
	}{{profile.ConnectTimeout, 2 * time.Minute}, {profile.AttemptTimeout, 30 * time.Second}} {
		if item.value == "" {
			continue
		}
		duration, err := time.ParseDuration(item.value)
		if err != nil || duration <= 0 || duration > item.max {
			return fmt.Errorf("%s", bad)
		}
	}
	return nil
}
