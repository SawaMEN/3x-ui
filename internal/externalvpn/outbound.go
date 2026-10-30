package externalvpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

type OutboundSettings struct {
	Link       string              `json:"link"`
	EnableUDP  bool                `json:"enableUDP"`
	UDPStream  bool                `json:"udpStream"`
	Token      string              `json:"token"`
	MTU        int                 `json:"mtu"`
	SNI        string              `json:"sni"`
	Bypass     string              `json:"bypass"`
	Secret     string              `json:"secret"`
	Context    string              `json:"context"`
	Transports []OpenFluxTransport `json:"transports"`
}

type managedOutbound struct {
	probe       bool
	raw         map[string]any
	fingerprint string
	port        int
	password    string
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	done        chan struct{}
	container   string
	nextRetry   time.Time
	failures    int
}

var (
	outboundMu        sync.Mutex
	outboundProcesses = map[string]*managedOutbound{}
	outboundPlans     = map[string]*managedOutbound{}
)

func IsAdditionalOutbound(raw map[string]any) bool {
	p, _ := raw["protocol"].(string)
	return strings.EqualFold(p, "fptn") || strings.EqualFold(p, "openflux") || strings.EqualFold(p, "vetroff")
}

func ValidateAdditionalOutbound(raw map[string]any) error {
	data, err := json.Marshal(raw["settings"])
	if err != nil {
		return err
	}
	var s OutboundSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	p, _ := raw["protocol"].(string)
	if strings.EqualFold(p, "vetroff") {
		return validateVetroffOutbound(s)
	}
	if strings.EqualFold(p, "fptn") {
		if !strings.HasPrefix(s.Token, "fptn:") {
			return fmt.Errorf("FPTN outbound requires an upstream access token")
		}
		if s.MTU != 0 && (s.MTU < 576 || s.MTU > 9000) {
			return fmt.Errorf("invalid FPTN MTU")
		}
		if s.SNI != "" && !domainPattern.MatchString(s.SNI) {
			return fmt.Errorf("invalid FPTN SNI")
		}
		if s.Bypass != "" && s.Bypass != "obfuscation" && s.Bypass != "sni-spoofing" {
			return fmt.Errorf("unsupported FPTN bypass preset")
		}
		_, err := DecodeFPTNToken(s.Token)
		return err
	}
	inst := Instance{Protocol: model.OpenFlux, Port: 443, Settings: Settings{Context: s.Context, Transports: s.Transports, Clients: []Client{{Email: "outbound", Password: s.Secret, Enable: true}}}}
	if err := inst.Validate(); err != nil {
		return err
	}
	for _, t := range s.Transports {
		if t.Type == "direct" {
			host, port, err := net.SplitHostPort(t.Dial)
			n, e := strconv.Atoi(port)
			if err != nil || e != nil || host == "" || strings.ContainsAny(t.Dial, "\r\n\x00#; \t") || n < 1 || n > 65535 {
				return fmt.Errorf("OpenFlux direct outbound requires host:port")
			}
		}
	}
	return nil
}

// PlanOutbound allocates a stable bridge identity without starting/stopping a
// child or rewriting its files. Preview and candidate checks use this path.
func PlanOutbound(raw map[string]any) (map[string]any, error) {
	if err := ValidateAdditionalOutbound(raw); err != nil {
		return nil, err
	}
	tag, _ := raw["tag"].(string)
	if strings.TrimSpace(tag) == "" {
		return nil, fmt.Errorf("managed outbound requires tag")
	}
	outboundMu.Lock()
	defer outboundMu.Unlock()
	plan, err := planOutboundLocked(tag, raw)
	if err != nil {
		return nil, err
	}
	return outboundBridge(tag, plan), nil
}

func planOutboundLocked(tag string, raw map[string]any) (*managedOutbound, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(sum[:])
	if plan := outboundPlans[tag]; plan != nil && plan.fingerprint == fingerprint {
		if active := outboundProcesses[tag]; active != nil {
			plan.port, plan.password = active.port, active.password
		}
		return plan, nil
	}
	plan := &managedOutbound{fingerprint: fingerprint}
	if err := json.Unmarshal(data, &plan.raw); err != nil {
		return nil, err
	}
	old := outboundProcesses[tag]
	if old == nil {
		old = outboundPlans[tag]
	}
	if old != nil {
		plan.port, plan.password = old.port, old.password
	}
	if plan.port == 0 {
		listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		plan.port = listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
	}
	if plan.password == "" {
		plan.password = uuid.NewString()
	}
	outboundPlans[tag] = plan
	return plan, nil
}

// OutboundsApplied includes the sidecar configuration behind stable bridge
// addresses: identical native JSON does not imply identical VPN credentials.
func OutboundsApplied(raws []map[string]any) bool {
	outboundMu.Lock()
	defer outboundMu.Unlock()
	tags := map[string]bool{}
	for _, raw := range raws {
		tag, _ := raw["tag"].(string)
		tags[tag] = true
		data, err := json.Marshal(raw)
		if err != nil {
			return false
		}
		sum := sha256.Sum256(data)
		proc := outboundProcesses[tag]
		if proc == nil || proc.fingerprint != hex.EncodeToString(sum[:]) || proc.done == nil {
			return false
		}
		select {
		case <-proc.done:
			return false
		default:
		}
	}
	for tag, proc := range outboundProcesses {
		if !proc.probe && !tags[tag] {
			return false
		}
	}
	return true
}

// ApplyOutbounds stages desired sidecars only after the candidate has passed
// validation. Finish(true) retires obsolete children; Finish(false) restores
// replaced ones and removes newly started children. Callers serialize applies.
func ApplyOutbounds(raws []map[string]any) (finish func(bool) error, err error) {
	previous := map[string]*managedOutbound{}
	tags := map[string]bool{}
	outboundMu.Lock()
	for _, raw := range raws {
		tag, _ := raw["tag"].(string)
		tags[tag] = true
		previous[tag] = outboundProcesses[tag]
	}
	outboundMu.Unlock()
	var once sync.Once
	var finishErr error
	finish = func(commit bool) error {
		once.Do(func() {
			if commit {
				KeepOutbounds(tags)
				return
			}
			for tag, old := range previous {
				if old != nil {
					_, restoreErr := EnsureOutbound(old.raw)
					if restoreErr == nil {
						outboundMu.Lock()
						restored := outboundProcesses[tag]
						outboundMu.Unlock()
						if restored != old {
							restoreErr = waitOutboundStartup(tag, restored)
						}
					}
					finishErr = errors.Join(finishErr, restoreErr)
				} else {
					outboundMu.Lock()
					if proc := outboundProcesses[tag]; proc != nil {
						stopOutbound(proc)
						delete(outboundProcesses, tag)
					}
					outboundMu.Unlock()
				}
			}
		})
		return finishErr
	}
	for _, raw := range raws {
		if _, err := EnsureOutbound(raw); err != nil {
			return nil, errors.Join(err, finish(false))
		}
		tag, _ := raw["tag"].(string)
		outboundMu.Lock()
		proc := outboundProcesses[tag]
		outboundMu.Unlock()
		if proc != previous[tag] {
			if err := waitOutboundStartup(tag, proc); err != nil {
				return nil, errors.Join(err, finish(false))
			}
		}
	}
	return finish, nil
}

func waitOutboundStartup(tag string, proc *managedOutbound) error {
	if proc == nil || proc.done == nil {
		return fmt.Errorf("managed outbound %q disappeared during apply", tag)
	}
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-proc.done:
		return fmt.Errorf("managed outbound %q exited during startup: %s", tag, proc.cmd.ProcessState)
	case <-timer.C:
		return nil
	}
}

func EnsureOutbound(raw map[string]any) (map[string]any, error) { return ensureOutbound(raw, false) }

func ensureOutbound(raw map[string]any, probe bool) (map[string]any, error) {
	if err := ValidateAdditionalOutbound(raw); err != nil {
		return nil, err
	}
	tag, _ := raw["tag"].(string)
	if strings.TrimSpace(tag) == "" {
		return nil, fmt.Errorf("managed outbound requires tag")
	}
	outboundMu.Lock()
	defer outboundMu.Unlock()
	data, _ := json.Marshal(raw)
	sum := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(sum[:])
	old := outboundProcesses[tag]
	if old != nil {
		alive := true
		select {
		case <-old.done:
			alive = false
		default:
		}
		if old.fingerprint == fingerprint && alive {
			return outboundBridge(tag, old), nil
		}
		stopOutbound(old)
	}
	proc := &managedOutbound{fingerprint: fingerprint, probe: probe}
	if !probe {
		if plan := outboundPlans[tag]; plan != nil && plan.fingerprint == fingerprint {
			proc.port, proc.password = plan.port, plan.password
		}
	}
	if err := json.Unmarshal(data, &proc.raw); err != nil {
		return nil, err
	}
	if old != nil {
		proc.port, proc.password = old.port, old.password
	}
	if proc.port == 0 {
		listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		proc.port = listener.Addr().(*net.TCPAddr).Port
		listener.Close()
	}
	if proc.password == "" {
		proc.password = uuid.NewString()
	}
	if err := startOutbound(tag, proc); err != nil {
		delete(outboundProcesses, tag)
		return nil, err
	}
	outboundProcesses[tag] = proc
	return outboundBridge(tag, proc), nil
}

func outboundBridge(tag string, proc *managedOutbound) map[string]any {
	server := map[string]any{"address": "127.0.0.1", "port": proc.port}
	// OpenFlux exposes an unauthenticated SOCKS listener, restricted to loopback.
	p, _ := proc.raw["protocol"].(string)
	if strings.EqualFold(p, "fptn") {
		return map[string]any{"tag": tag, "protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": proc.port, "users": []any{map[string]any{"id": proc.password, "encryption": "none"}}}}}, "streamSettings": map[string]any{"network": "tcp", "security": "none"}}
	}
	return map[string]any{"tag": tag, "protocol": "socks", "settings": map[string]any{"servers": []any{server}}}
}

func startOutbound(tag string, proc *managedOutbound) error {
	p, _ := proc.raw["protocol"].(string)
	data, _ := json.Marshal(proc.raw["settings"])
	var s OutboundSettings
	_ = json.Unmarshal(data, &s)
	sum := sha256.Sum256([]byte(tag))
	suffix := hex.EncodeToString(sum[:8])
	folder := filepath.Join(directory(), "outbound-"+suffix)
	if err := os.MkdirAll(folder, 0o700); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	proc.cancel = cancel
	if strings.EqualFold(p, "vetroff") {
		connection := filepath.Join(folder, "connection.txt")
		if err := writePrivate(connection, []byte(s.Link+"\n")); err != nil {
			cancel()
			return err
		}
		command := binary(model.VetroffProxy)
		if _, err := os.Stat(command); err != nil {
			cancel()
			return fmt.Errorf("VetroffProxy outbound requires an installed VetroffProxy bundle: %w", err)
		}
		args := []string{"client", "--link-file", connection, "--listen", net.JoinHostPort("127.0.0.1", strconv.Itoa(proc.port))}
		if s.EnableUDP {
			args = append(args, "--enable-udp")
		}
		if s.UDPStream {
			args = append(args, "--udp-stream")
		}
		proc.cmd = exec.CommandContext(ctx, command, args...)
		proc.cmd.Dir = filepath.Dir(command)
		// The core uses a local unauthenticated SOCKS bridge. Do not inherit
		// unrelated host SOCKS credentials, and never expose this listener on LAN.
		proc.cmd.Env = append(os.Environ(), "VETROFF_SOCKS_USER=", "VETROFF_SOCKS_PASSWORD=")
		prepareVetroffCommand(proc.cmd)
	} else if strings.EqualFold(p, "openflux") {
		secret := filepath.Join(folder, "secret")
		if strings.ContainsAny(secret, "\r\n#;") {
			cancel()
			return fmt.Errorf("unsupported runtime directory")
		}
		if err := writePrivate(secret, []byte(s.Secret)); err != nil {
			cancel()
			return err
		}
		var conf strings.Builder
		fmt.Fprintf(&conf, "[Interface]\nRole = client\nInbound = socks5\nSocks5 = 127.0.0.1:%d\nCodec = batched\nEncryptionKeyFile = %s\nSessionContext = %s\nCookieStore = %s\n", proc.port, secret, s.Context, filepath.Join(folder, "cookies.json"))
		for _, t := range s.Transports {
			fmt.Fprintf(&conf, "\n[Transport \"%s\"]\nType = %s\nPriority = %d\n", t.Type, t.Type, t.Priority)
			if t.Type == "direct" {
				fmt.Fprintf(&conf, "Dial = %s\n", t.Dial)
			} else {
				fmt.Fprintf(&conf, "URL = %s\n", t.URL)
			}
		}
		path := filepath.Join(folder, "client.conf")
		if err := writePrivate(path, []byte(conf.String())); err != nil {
			cancel()
			return err
		}
		proc.cmd = exec.CommandContext(ctx, binary(model.OpenFlux), "--config", path, "--negotiate")
	} else {
		checkCtx, checkCancel := context.WithTimeout(ctx, 10*time.Second)
		checkErr := exec.CommandContext(checkCtx, "docker", "image", "inspect", "3x-ui-fptn-client:0.4.6").Run()
		checkCancel()
		if checkErr != nil {
			cancel()
			return fmt.Errorf("FPTN outbound requires a running Docker daemon and the installed FPTN client image: %w", checkErr)
		}
		if err := writePrivate(filepath.Join(folder, "token"), []byte(s.Token)); err != nil {
			cancel()
			return err
		}
		if s.MTU == 0 {
			s.MTU = 1400
		}
		if s.SNI == "" {
			s.SNI = "www.bing.com"
		}
		if s.Bypass == "" {
			s.Bypass = "sni-spoofing"
		}
		// The VLESS bridge can dial only via fptn0; DNS takes the same outbound.
		// A dead tunnel therefore fails closed instead of falling back to eth0.
		socks := fptnBridgeConfig(proc.password)
		cfg, _ := json.Marshal(socks)
		if err := writePrivate(filepath.Join(folder, "socks.json"), cfg); err != nil {
			cancel()
			return err
		}

		if err := writePrivate(filepath.Join(folder, "run.sh"), []byte(fptnOutboundScript)); err != nil {
			cancel()
			return err
		}
		sb, err := filepath.Abs(config.OwnerCoreBinaryPath())
		if err != nil {
			cancel()
			return err
		}
		if _, err := os.Stat(sb); err != nil {
			cancel()
			return fmt.Errorf("FPTN outbound requires installed owner hiddify-core: %w", err)
		}
		proc.container = containerName(0) + "-out-" + suffix
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		_ = exec.CommandContext(cleanupCtx, "docker", "rm", "-f", proc.container).Run()
		done()
		proc.cmd = exec.CommandContext(ctx, "docker", "run", "--rm", "--name", proc.container, "--cap-add", "NET_ADMIN", "--device", "/dev/net/tun:/dev/net/tun", "--publish", fmt.Sprintf("127.0.0.1:%d:1080/tcp", proc.port), "--mount", "type=bind,src="+folder+",dst=/run/panel,readonly", "--mount", "type=bind,src="+sb+",dst=/usr/bin/panel-core,readonly", "3x-ui-fptn-client:0.4.6", "/bin/sh", "/run/panel/run.sh", strconv.Itoa(s.MTU), s.SNI, s.Bypass)
	}
	proc.cmd.Stdout, proc.cmd.Stderr = os.Stdout, os.Stderr
	if err := proc.cmd.Start(); err != nil {
		cancel()
		return err
	}
	proc.done = make(chan struct{})
	go func() { _ = proc.cmd.Wait(); close(proc.done) }()
	return nil
}

func stopOutbound(proc *managedOutbound) {
	if proc.container != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		_ = exec.CommandContext(ctx, "docker", "rm", "-f", proc.container).Run()
		cancel()
	}
	if proc.cancel != nil {
		proc.cancel()
	}
	if proc.done != nil {
		select {
		case <-proc.done:
		case <-time.After(3 * time.Second):
		}
	}
}

func KeepOutbounds(tags map[string]bool) {
	outboundMu.Lock()
	defer outboundMu.Unlock()
	for tag := range outboundPlans {
		if !tags[tag] {
			delete(outboundPlans, tag)
		}
	}
	for tag, proc := range outboundProcesses {
		if !tags[tag] && (!proc.probe || tags == nil) {
			stopOutbound(proc)
			delete(outboundProcesses, tag)
		}
	}
}

func RefreshOutbounds() {
	outboundMu.Lock()
	defer outboundMu.Unlock()
	for tag, proc := range outboundProcesses {
		if proc.probe {
			continue
		}
		select {
		case <-proc.done:
		default:
			continue
		}
		if time.Now().Before(proc.nextRetry) {
			continue
		}
		stopOutbound(proc)
		if err := startOutbound(tag, proc); err != nil {
			logger.Warningf("managed VPN outbound %q: %v", tag, err)
		}
		// Do not run a tight restart loop for an invalid config or unreachable
		// runtime. Keep the same bridge address through every recovery attempt.
		proc.failures++
		if proc.failures > 6 {
			proc.failures = 6
		}
		proc.nextRetry = time.Now().Add(time.Duration(1<<proc.failures) * time.Second)
	}
}

// RemoveOutboundProtocol stops an uninstalled sidecar and drops its plans
// so the periodic supervisor cannot resurrect it while its executable is
// being removed. Other managed outbound protocols are unaffected.
func RemoveOutboundProtocol(protocol model.Protocol) {
	outboundMu.Lock()
	defer outboundMu.Unlock()
	for tag, proc := range outboundProcesses {
		p, _ := proc.raw["protocol"].(string)
		if strings.EqualFold(p, string(protocol)) {
			stopOutbound(proc)
			delete(outboundProcesses, tag)
		}
	}
	for tag, plan := range outboundPlans {
		p, _ := plan.raw["protocol"].(string)
		if strings.EqualFold(p, string(protocol)) {
			delete(outboundPlans, tag)
		}
	}
}

func stopOutboundProtocol(protocol model.Protocol) {
	outboundMu.Lock()
	defer outboundMu.Unlock()
	for _, proc := range outboundProcesses {
		p, _ := proc.raw["protocol"].(string)
		if strings.EqualFold(p, string(protocol)) {
			stopOutbound(proc)
			proc.nextRetry = time.Time{}
		}
	}
}

// Probe processes use separate identities so a temporary test cannot replace
// the live tunnel or be restarted indefinitely by the regular supervisor.
func EnsureProbeOutbound(raw map[string]any) (map[string]any, func(), error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, nil, err
	}
	var isolated map[string]any
	if err := json.Unmarshal(data, &isolated); err != nil {
		return nil, nil, err
	}
	tag, _ := raw["tag"].(string)
	probeTag := "__probe_" + uuid.NewString()
	isolated["tag"] = probeTag
	bridge, err := ensureOutbound(isolated, true)
	if err != nil {
		return nil, nil, err
	}
	bridge["tag"] = tag
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			outboundMu.Lock()
			defer outboundMu.Unlock()
			if proc := outboundProcesses[probeTag]; proc != nil {
				stopOutbound(proc)
				delete(outboundProcesses, probeTag)
			}
			sum := sha256.Sum256([]byte(probeTag))
			_ = os.RemoveAll(filepath.Join(directory(), "outbound-"+hex.EncodeToString(sum[:8])))
		})
	}
	return bridge, cleanup, nil
}

func fptnBridgeConfig(password string) map[string]any {
	return map[string]any{"log": map[string]any{"level": "warn"}, "inbounds": []any{map[string]any{"type": "vless", "tag": "panel-in", "listen": "0.0.0.0", "listen_port": 1080, "users": []any{map[string]any{"name": "panel", "uuid": password}}}}, "outbounds": []any{map[string]any{"type": "direct", "tag": "vpn-out", "bind_interface": "fptn0"}}, "dns": map[string]any{"servers": []any{map[string]any{"type": "udp", "tag": "vpn-dns", "server": "1.1.1.1", "detour": "vpn-out"}}, "final": "vpn-dns", "strategy": "ipv4_only"}, "route": map[string]any{"final": "vpn-out", "default_domain_resolver": "vpn-dns"}}
}

const fptnOutboundScript = `#!/bin/sh
set -eu
/usr/bin/fptn-client-cli --access-token "$(cat /run/panel/token)" --tun-interface-name fptn0 --mtu-size "$1" --sni "$2" --bypass-method "$3" --enable-ad-block false &
client=$!
trap 'kill "$client" 2>/dev/null || true' EXIT INT TERM
n=0
while ! ip link show fptn0 >/dev/null 2>&1; do
 kill -0 "$client" || exit 1
 n=$((n+1)); [ "$n" -le 60 ] || exit 1
 sleep 1
done
/usr/bin/panel-core srun -c /run/panel/socks.json &
socks=$!
while kill -0 "$client" 2>/dev/null && kill -0 "$socks" 2>/dev/null; do sleep 1; done
kill "$client" "$socks" 2>/dev/null || true
exit 1
`
