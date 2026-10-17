package protocolserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/network"
)

type process struct {
	plan        Plan
	cmd         *exec.Cmd
	done        chan struct{}
	auth        *http.Server
	epoch       string
	fingerprint string
}

var state = struct {
	sync.Mutex
	processes map[string]*process
	selected  [2]bool
}{processes: map[string]*process{}}

func selection() [2]bool { return [2]bool{Installed(model.NaiveProxy), Installed(model.Hysteria)} }
func identifier(tag string) string {
	sum := sha256.Sum256([]byte(tag))
	return hex.EncodeToString(sum[:12])
}

func fingerprint(plan Plan) string {
	data, _ := json.Marshal(plan)
	if info, err := os.Stat(plan.Binary); err == nil {
		data = append(data, []byte(fmt.Sprintf(":%d:%d", info.Size(), info.ModTime().UnixNano()))...)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func alive(p *process) bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func Changed() bool {
	state.Lock()
	defer state.Unlock()
	if state.selected != selection() {
		return true
	}
	for _, p := range state.processes {
		if !alive(p) || p.fingerprint != fingerprint(p.plan) {
			return true
		}
	}
	return false
}

func Applied(plans []Plan) bool {
	state.Lock()
	defer state.Unlock()
	if state.selected != selection() || len(plans) != len(state.processes) {
		return false
	}
	for _, plan := range plans {
		p := state.processes[plan.Tag]
		if p == nil || !alive(p) || p.fingerprint != fingerprint(plan) {
			return false
		}
	}
	return true
}

func Running(protocol model.Protocol) bool {
	state.Lock()
	defer state.Unlock()
	for _, p := range state.processes {
		if p.plan.Protocol == protocol && alive(p) {
			return true
		}
	}
	return false
}

func Suspend() ([]Plan, error) {
	state.Lock()
	defer state.Unlock()
	plans := make([]Plan, 0, len(state.processes))
	var result error
	for tag, p := range state.processes {
		plans = append(plans, p.plan)
		result = errors.Join(result, stop(p))
		delete(state.processes, tag)
	}
	return plans, result
}

func StopAll() error { _, err := Suspend(); return err }

func Start(ctx context.Context, plans []Plan) error {
	state.Lock()
	defer state.Unlock()
	for _, plan := range plans {
		if current := state.processes[plan.Tag]; current != nil && alive(current) && current.fingerprint == fingerprint(plan) {
			continue
		}
		if current := state.processes[plan.Tag]; current != nil {
			if err := stop(current); err != nil {
				return err
			}
			delete(state.processes, plan.Tag)
		}
		p, err := start(ctx, plan)
		if err != nil {
			for tag, running := range state.processes {
				_ = stop(running)
				delete(state.processes, tag)
			}
			return err
		}
		state.processes[plan.Tag] = p
	}
	state.selected = selection()
	return nil
}

// Check validates Caddy before the currently serving core releases its ports.
func Check(ctx context.Context, plans []Plan) error {
	for _, plan := range plans {
		if plan.Protocol != model.NaiveProxy {
			continue
		}
		dir, err := os.MkdirTemp("", "3x-ui-naive-check-*")
		if err != nil {
			return err
		}
		path := filepath.Join(dir, "config.json")
		err = os.WriteFile(path, plan.Config, 0o600)
		if err == nil {
			checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			output, checkErr := exec.CommandContext(checkCtx, plan.Binary, "validate", "--config", path).CombinedOutput()
			cancel()
			if checkErr != nil {
				err = fmt.Errorf("validate standalone Naive %s: %w: %s", plan.Tag, checkErr, output)
			}
		}
		_ = os.RemoveAll(dir)
		if err != nil {
			return err
		}
	}
	return nil
}

func start(ctx context.Context, plan Plan) (*process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := filepath.Join(config.GetBinFolderPath(), "protocol-servers", identifier(plan.Tag))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	for name, contents := range plan.Files {
		if name != filepath.Base(name) {
			return nil, fmt.Errorf("invalid protocol certificate filename")
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
			return nil, err
		}
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, plan.Config, 0o600); err != nil {
		return nil, err
	}
	p := &process{plan: plan, done: make(chan struct{}), fingerprint: fingerprint(plan)}
	if plan.AuthListen != "" {
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", plan.AuthListen)
		if err != nil {
			return nil, err
		}
		p.auth = &http.Server{Handler: authHandler(plan), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: 5 * time.Second}
		go func() { _ = network.ServeHTTP(p.auth, listener, "Hysteria authentication "+plan.Tag) }()
	}
	args := []string{"server", "--disable-update-check", "--config", path}
	if plan.Protocol == model.NaiveProxy {
		args = []string{"run", "--config", path}
	}
	p.cmd = exec.CommandContext(context.Background(), plan.Binary, args...)
	prepareCommand(p.cmd)
	log, err := os.OpenFile(filepath.Join(dir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		if p.auth != nil {
			_ = p.auth.Close()
		}
		return nil, err
	}
	p.cmd.Stdout, p.cmd.Stderr = log, log
	if err := p.cmd.Start(); err != nil {
		_ = log.Close()
		if p.auth != nil {
			_ = p.auth.Close()
		}
		return nil, err
	}
	_ = log.Close()
	p.epoch = fmt.Sprintf("%d:%d", p.cmd.Process.Pid, time.Now().UnixNano())
	go func() {
		_ = p.cmd.Wait()
		close(p.done)
		if p.auth != nil {
			_ = p.auth.Close()
		}
	}()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	readyChecks := 0
	for {
		select {
		case <-ctx.Done():
			_ = stop(p)
			return nil, ctx.Err()
		case <-p.done:
			return nil, fmt.Errorf("standalone %s %s exited; see %s", plan.Protocol, plan.Tag, filepath.Join(dir, "server.log"))
		case <-deadline.C:
			_ = stop(p)
			return nil, fmt.Errorf("standalone %s %s did not become ready; see %s", plan.Protocol, plan.Tag, filepath.Join(dir, "server.log"))
		case <-tick.C:
			readyCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
			var readyErr error
			if plan.Protocol == model.Hysteria {
				var online map[string]int
				readyErr = getStats(readyCtx, plan, "/online", &online)
			} else if plan.ReadyUDP {
				packet, err := (&net.ListenConfig{}).ListenPacket(readyCtx, "udp", plan.ReadyListen)
				if packet != nil {
					_ = packet.Close()
				}
				readyErr = err
				if errors.Is(err, syscall.EADDRINUSE) {
					readyErr = nil
				} else if err == nil {
					readyErr = fmt.Errorf("QUIC listener is not ready")
				}
			} else {
				var conn net.Conn
				conn, readyErr = (&net.Dialer{}).DialContext(readyCtx, "tcp", plan.ReadyListen)
				if conn != nil {
					_ = conn.Close()
				}
			}
			cancel()
			if readyErr == nil && alive(p) {
				readyChecks++
				if readyChecks >= 2 {
					return p, nil
				}
			} else {
				readyChecks = 0
			}
		}
	}
}

func stop(p *process) error {
	if p.auth != nil {
		_ = p.auth.Close()
	}
	if !alive(p) {
		return nil
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-p.done:
		return nil
	case <-timer.C:
	}
	if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	<-p.done
	return nil
}

func authHandler(plan Plan) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != plan.AuthPath {
			http.NotFound(w, r)
			return
		}
		var request struct {
			Auth string `json:"auth"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&request); err != nil {
			http.Error(w, "invalid authentication", http.StatusBadRequest)
			return
		}
		name, ok := plan.Users[request.Auth]
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": ok, "id": name})
	})
}

type Traffic struct {
	Tx int64 `json:"tx"`
	Rx int64 `json:"rx"`
}
type Snapshot struct {
	Namespace, Epoch, Tag string
	Counters              map[string]Traffic
	Online                map[string]int
}

var statsClient = &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}

func Snapshots(ctx context.Context) ([]Snapshot, error) {
	state.Lock()
	var sources []*process
	for _, p := range state.processes {
		if p.plan.Protocol == model.Hysteria && alive(p) {
			sources = append(sources, p)
		}
	}
	state.Unlock()
	var snapshots []Snapshot
	for _, p := range sources {
		snapshot := Snapshot{Namespace: "hysteria:" + identifier(p.plan.Tag) + ":", Epoch: p.epoch, Tag: p.plan.Tag}
		if err := getStats(ctx, p.plan, "/traffic", &snapshot.Counters); err != nil {
			return nil, err
		}
		if err := getStats(ctx, p.plan, "/online", &snapshot.Online); err != nil {
			return nil, err
		}
		state.Lock()
		current := state.processes[p.plan.Tag]
		unchanged := current == p && alive(p)
		state.Unlock()
		if !unchanged {
			return nil, fmt.Errorf("Hysteria restarted during statistics query")
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func getStats(ctx context.Context, plan Plan, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+plan.StatsListen+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", plan.StatsSecret)
	resp, err := statsClient.Do(req)
	if err != nil {
		return fmt.Errorf("Hysteria %s statistics: %w", plan.Tag, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Hysteria %s statistics: HTTP %d", plan.Tag, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(target)
}

func Presence(ctx context.Context) (map[string]map[string]struct{}, []string, error) {
	snapshots, err := Snapshots(ctx)
	if err != nil {
		return nil, nil, err
	}
	online := map[string]map[string]struct{}{}
	var tags []string
	for _, snapshot := range snapshots {
		active := false
		for email, count := range snapshot.Online {
			if count > 0 {
				online[email] = map[string]struct{}{}
				active = true
			}
		}
		if active {
			tags = append(tags, snapshot.Tag)
		}
	}
	return online, tags, nil
}

func Port(seed string, occupied map[int]bool) (int, error) {
	hash := sha256.Sum256([]byte(seed))
	start := 20000 + (int(hash[0])<<8|int(hash[1]))%40000
	for offset := 0; offset < 40000; offset++ {
		port := 20000 + (start-20000+offset)%40000
		if !occupied[port] {
			occupied[port] = true
			return port, nil
		}
	}
	return 0, fmt.Errorf("no internal listener port available for %s", strconv.Quote(seed))
}
