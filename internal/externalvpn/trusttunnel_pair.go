package externalvpn

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// The split runtime uses one inbound identity. The original address is
// retained separately so QUIC keeps its socket and its public certificate.
func trustTunnelOriginalInstance(inst Instance) (Instance, error) {
	host, port, err := net.SplitHostPort(inst.TrustTunnelQUICBind)
	number, parseErr := strconv.Atoi(port)
	if inst.Protocol != model.TrustTunnel || inst.Settings.TrustTunnelTCPOnly ||
		inst.Listen != "127.0.0.1" || err != nil || parseErr != nil ||
		net.ParseIP(host) == nil || number < 1 || number > 65535 || number == inst.Port {
		return Instance{}, errors.New("invalid split TrustTunnel listener")
	}
	original := inst
	original.Listen, original.Port = host, number
	original.TrustTunnelQUICBind = ""
	original.Settings.TrustTunnelQUICBind = ""
	return original, nil
}

func allocateTrustTunnelMetrics() (string, string, error) {
	first, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", err
	}
	defer first.Close()
	second, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", err
	}
	defer second.Close()
	return first.Addr().String(), second.Addr().String(), nil
}

func prepareTrustTunnelPair(inst Instance, tcpMetrics, quicMetrics string) (*exec.Cmd, *exec.Cmd, context.CancelFunc, error) {
	original, err := trustTunnelOriginalInstance(inst)
	if err != nil {
		return nil, nil, nil, err
	}
	plan, err := planTrustTunnelSplit(original, inst.Port, tcpMetrics, quicMetrics)
	if err != nil {
		return nil, nil, nil, err
	}
	// Keep the combined vpn.toml for the official client link generator.
	// Only the runtime processes consume the two transport-specific files.
	if _, err := files(original, tcpMetrics); err != nil {
		return nil, nil, nil, err
	}
	folder := filepath.Dir(plan.HostsPath)
	tcpPath, quicPath := filepath.Join(folder, "vpn-tcp.toml"), filepath.Join(folder, "vpn-quic.toml")
	if err := writePrivate(tcpPath, []byte(plan.TCPConfig)); err != nil {
		return nil, nil, nil, err
	}
	if err := writePrivate(quicPath, []byte(plan.QUICConfig)); err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	tcp := exec.CommandContext(ctx, binary(inst.Protocol), tcpPath, plan.HostsPath)
	quic := exec.CommandContext(ctx, binary(inst.Protocol), quicPath, plan.HostsPath)
	for _, cmd := range []*exec.Cmd{tcp, quic} {
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		prepareVetroffCommand(cmd)
	}
	return tcp, quic, cancel, nil
}

// If the second child cannot start, synchronously reap the first. Never leave
// an untracked endpoint accepting old credentials.
func startExternalVPNChildren(primary, companion *exec.Cmd, cancel context.CancelFunc) error {
	if err := primary.Start(); err != nil {
		cancel()
		return err
	}
	if companion != nil {
		if err := companion.Start(); err != nil {
			cancel()
			_ = primary.Wait()
			return err
		}
	}
	return nil
}

// A split inbound is healthy only while both children run. On either exit,
// terminate the peer and wait for both before releasing the manager's slot.
func waitExternalVPNChildren(primary, companion *exec.Cmd, cancel context.CancelFunc) error {
	if companion == nil {
		return primary.Wait()
	}
	results := make(chan error, 2)
	go func() { results <- primary.Wait() }()
	go func() { results <- companion.Wait() }()
	first := <-results
	cancel()
	second := <-results
	return errors.Join(first, second)
}

func waitTrustTunnelPairReady(proc *running) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-proc.done:
			return errors.New("TrustTunnel split endpoint exited during startup")
		case <-ctx.Done():
			return errors.New("TrustTunnel split metrics did not become ready")
		case <-ticker.C:
			if _, err := readTrustTunnelMetrics(ctx, proc.metricsAddr); err != nil {
				continue
			}
			if _, err := readTrustTunnelMetrics(ctx, proc.quicMetricsAddr); err == nil && ctx.Err() == nil {
				select {
				case <-proc.done:
					return errors.New("TrustTunnel split endpoint exited during startup")
				default:
					return nil
				}
			}
		}
	}
}

// TrustTunnelQUICPID identifies the actual managed process, not just another
// executable with the same name. Combined mode uses the primary child.
func (m *Manager) TrustTunnelQUICPID(id int, bind string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	proc := m.procs[id]
	if proc == nil || proc.protocol != model.TrustTunnel || proc.stopping.Load() ||
		proc.instance.Settings.TrustTunnelTCPOnly {
		return 0, errors.New("managed TrustTunnel QUIC endpoint is not running")
	}
	select {
	case <-proc.done:
		return 0, errors.New("managed TrustTunnel endpoint exited")
	default:
	}
	cmd := proc.cmd
	expected := proc.instance.Bind()
	if proc.instance.TrustTunnelQUICBind != "" {
		cmd = proc.quicCmd
		expected = proc.instance.TrustTunnelQUICBind
	}
	if bind != expected || cmd == nil || cmd.Process == nil {
		return 0, errors.New("managed TrustTunnel QUIC endpoint does not match the saved bind")
	}
	return cmd.Process.Pid, nil
}
