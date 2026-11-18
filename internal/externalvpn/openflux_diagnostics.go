package externalvpn

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"golang.org/x/net/proxy"
)

// OpenFluxDiagnosticStep is a bounded observation, not a prediction about
// carrier reachability from the subscriber's own mobile network.
type OpenFluxDiagnosticStep struct {
	Stage     string `json:"stage"`
	Status    string `json:"status"`
	Detail    string `json:"detail"`
	ElapsedMS int64  `json:"elapsedMs,omitempty"`
}

type OpenFluxDiagnosticResult struct {
	Type   string                   `json:"type"`
	Scope  string                   `json:"scope"`
	Passed bool                     `json:"passed"`
	Steps  []OpenFluxDiagnosticStep `json:"steps"`
}

func OpenFluxDiagnosticTypeSupported(typ string) bool {
	if typ == "direct" {
		return true
	}
	_, ok := openFluxProbeHost(typ)
	return ok
}

// DiagnoseOpenFluxChannel launches an isolated OpenFlux *client* against the
// existing exit. It enables exactly the requested transport: a working direct
// fallback can never hide a broken document/room/MAX carrier.
//
// IMPORTANT: the official exit supports one negotiated client per inbound.
// A successful active probe can temporarily replace its real user's session.
// Callers MUST obtain explicit informed acknowledgement before invoking this.
func DiagnoseOpenFluxChannel(parent context.Context, inst Instance, typ, maxClientToken string) OpenFluxDiagnosticResult {
	result := OpenFluxDiagnosticResult{Type: typ, Scope: "local-openflux-e2e", Steps: []OpenFluxDiagnosticStep{}}
	add := func(stage, status, detail string, elapsed time.Duration) {
		step := OpenFluxDiagnosticStep{Stage: stage, Status: status, Detail: detail}
		if elapsed > 0 {
			step.ElapsedMS = elapsed.Milliseconds()
		}
		result.Steps = append(result.Steps, step)
	}
	if !OpenFluxDiagnosticTypeSupported(typ) {
		add("configuration", "failed", "Unsupported OpenFlux transport", 0)
		return result
	}
	if inst.Protocol != model.OpenFlux || inst.ID <= 0 {
		add("configuration", "failed", "Select a saved OpenFlux inbound", 0)
		return result
	}
	if err := inst.Validate(); err != nil {
		add("configuration", "failed", "Saved inbound configuration is invalid: "+err.Error(), 0)
		return result
	}
	if !OpenFluxBinaryStatus(parent, true).Installed {
		add("configuration", "failed", "OpenFlux binary is not installed on this node", 0)
		return result
	}
	var channel *OpenFluxTransport
	for i := range inst.Settings.Transports {
		if inst.Settings.Transports[i].Type == typ {
			channel = &inst.Settings.Transports[i]
			break
		}
	}
	if channel == nil {
		add("configuration", "failed", "Transport is absent from the saved inbound; save the form first", 0)
		return result
	}
	var secret string
	for _, client := range inst.Settings.Clients {
		if client.Enable && client.Password != "" {
			secret = client.Password
			break
		}
	}
	if secret == "" {
		add("configuration", "failed", "Saved inbound has no enabled OpenFlux client secret", 0)
		return result
	}
	carrier := *channel
	if typ == "oneme" {
		if !validOpenFluxMAXUID(carrier.UID) || !validOpenFluxMAXToken(maxClientToken) || maxClientToken == carrier.Token {
			add("configuration", "failed", "MAX requires a separate client-account Web MAX token and a valid server account ID", 0)
			return result
		}
		carrier.Token = maxClientToken
	} else if maxClientToken != "" {
		add("configuration", "failed", "A MAX token is not permitted for this transport", 0)
		return result
	}
	if typ == "direct" {
		host := inst.Listen
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		carrier.Dial = net.JoinHostPort(host, strconv.Itoa(inst.Port))
	}
	add("configuration", "passed", "Saved transport, active client secret and local OpenFlux executable are valid", 0)

	ctx, cancel := context.WithTimeout(parent, 50*time.Second)
	defer cancel()
	if typ == "direct" {
		add("provider", "skipped", "Direct uses a local TCP connection; this cannot prove the port is reachable from an external mobile network", 0)
	} else {
		begin := time.Now()
		probe, err := ProbeOpenFluxProvider(ctx, typ)
		if err != nil {
			add("provider", "failed", "Provider TLS check: "+err.Error(), time.Since(begin))
		} else {
			add("provider", "passed", "Verified TLS to "+probe.Host+" (not yet a working tunnel)", time.Since(begin))
		}
	}

	// Use only one transport. Never install client cookies into the running
	// inbound, and never put secrets on a process command line.
	raw := map[string]any{
		"tag":      "openflux-diagnostic",
		"protocol": "openflux",
		"settings": map[string]any{
			"secret":     secret,
			"context":    inst.Settings.Context,
			"transports": []OpenFluxTransport{carrier},
		},
	}
	begin := time.Now()
	bridge, cleanup, err := EnsureProbeOutboundContext(ctx, raw)
	if err != nil {
		add("session", "failed", "Cannot start isolated OpenFlux client: "+err.Error(), time.Since(begin))
		add("payload", "skipped", "No tunnel session to test", 0)
		return result
	}
	defer cleanup()

	// The exit's L4 backend dials the machine's own non-loopback IPv4.
	// A random challenge makes a SOCKS greeting or a local TCP listener
	// insufficient evidence of a working encrypted end-to-end session.
	host, err := openFluxDiagnosticIPv4()
	if err != nil {
		add("session", "failed", err.Error(), time.Since(begin))
		add("payload", "skipped", "No local echo target", 0)
		return result
	}
	var challenge [48]byte
	if _, err := rand.Read(challenge[:]); err != nil {
		add("payload", "failed", "Cannot generate random test payload", 0)
		return result
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		add("session", "failed", "Cannot open a temporary local echo target: "+err.Error(), time.Since(begin))
		add("payload", "skipped", "No local echo target", 0)
		return result
	}
	defer listener.Close()
	go openFluxDiagnosticEcho(ctx, listener, challenge)

	bridgeSettings, ok := bridge["settings"].(map[string]any)
	if !ok {
		add("session", "failed", "Invalid local probe bridge", time.Since(begin))
		return result
	}
	servers, ok := bridgeSettings["servers"].([]any)
	if !ok || len(servers) == 0 {
		add("session", "failed", "No SOCKS bridge reported by probe client", time.Since(begin))
		return result
	}
	server, ok := servers[0].(map[string]any)
	if !ok {
		add("session", "failed", "Invalid SOCKS bridge address", time.Since(begin))
		return result
	}
	socksAddr, ok := server["address"].(string)
	if !ok || socksAddr != "127.0.0.1" {
		add("session", "failed", "Diagnostic SOCKS bridge must listen on loopback", time.Since(begin))
		return result
	}
	port, ok := server["port"].(int)
	if !ok || port < 1 || port > 65535 {
		add("session", "failed", "Invalid SOCKS bridge port", time.Since(begin))
		return result
	}
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort(socksAddr, strconv.Itoa(port)), nil, &net.Dialer{Timeout: 12 * time.Second})
	if err != nil {
		add("session", "failed", "Cannot create diagnostic SOCKS dialer: "+err.Error(), time.Since(begin))
		return result
	}
	destination := listener.Addr().String()
	var conn net.Conn
	if cd, ok := dialer.(proxy.ContextDialer); ok {
		conn, err = cd.DialContext(ctx, "tcp", destination)
	} else {
		conn, err = dialer.Dial("tcp", destination)
	}
	if err != nil {
		add("session", "failed", "OpenFlux did not establish a SOCKS CONNECT through "+typ+": "+err.Error(), time.Since(begin))
		add("payload", "skipped", "Encrypted session/CONNECT failed; no fallback transport was enabled", 0)
		return result
	}
	defer conn.Close()
	add("session", "passed", "SOCKS CONNECT completed through the selected OpenFlux transport alone", time.Since(begin))

	_ = conn.SetDeadline(time.Now().Add(12 * time.Second))
	startPayload := time.Now()
	if _, err := conn.Write(challenge[:]); err != nil {
		add("payload", "failed", "Tunnel could not send test bytes: "+err.Error(), time.Since(startPayload))
		return result
	}
	var response [48]byte
	if _, err := io.ReadFull(conn, response[:]); err != nil {
		add("payload", "failed", "Tunnel returned no complete test payload: "+err.Error(), time.Since(startPayload))
		return result
	}
	if !bytes.Equal(challenge[:], response[:]) {
		add("payload", "failed", "Tunnel returned mismatched payload bytes", time.Since(startPayload))
		return result
	}
	add("payload", "passed", "Verified 48 random bytes round-trip over encrypted OpenFlux; no direct fallback", time.Since(startPayload))
	result.Passed = true
	return result
}

// A loopback address is deliberately not used as an echo destination: an
// OpenFlux gVisor exit may handle 127.0.0.0/8 internally instead of routing
// it to the host. Require an actual address owned by the local server.
func openFluxDiagnosticIPv4() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("cannot inspect local IPv4 addresses: %w", err)
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addresses {
			ip, _, err := net.ParseCIDR(a.String())
			if err == nil && ip.To4() != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
				return ip.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no non-loopback local IPv4 for a reliable OpenFlux L4 echo test")
}

func openFluxDiagnosticEcho(ctx context.Context, listener net.Listener, expected [48]byte) {
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(12 * time.Second))
			var data [48]byte
			if _, err := io.ReadFull(c, data[:]); err == nil && bytes.Equal(data[:], expected[:]) {
				_, _ = c.Write(data[:])
			}
		}(conn)
	}
}
