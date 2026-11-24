package externalvpn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

var trustTunnelCapability struct {
	sync.Mutex
	path    string
	size    int64
	modTime time.Time
	checked time.Time
	err     error
}

// CheckTrustTunnelQUICOnly runs an isolated localhost capability test. An
// upstream endpoint may bind TCP for pings even with HTTP/1 and HTTP/2 absent.
// Never move production listeners unless the actual installed binary proves
// it can run QUIC while the same TCP port is already occupied.
func CheckTrustTunnelQUICOnly(ctx context.Context) error {
	path, err := filepath.Abs(binary(model.TrustTunnel))
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("TrustTunnel endpoint unavailable: %w", err)
	}
	trustTunnelCapability.Lock()
	defer trustTunnelCapability.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	cache := &trustTunnelCapability
	if cache.path == path && cache.size == info.Size() && cache.modTime.Equal(info.ModTime()) &&
		time.Since(cache.checked) < time.Minute {
		return cache.err
	}
	err = probeTrustTunnelQUICOnly(ctx, path)
	if ctx.Err() == nil {
		cache.path, cache.size, cache.modTime, cache.checked, cache.err = path, info.Size(), info.ModTime(), time.Now(), err
	}
	return err
}

func probeTrustTunnelQUICOnly(ctx context.Context, path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	folder, err := os.MkdirTemp("", "xui-trusttunnel-quic-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(folder)
	// The guard models the HTTPS frontend; a QUIC-only process must never
	// need this TCP socket. All addresses and credentials are test-only.
	guard, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer guard.Close()
	bind := guard.Addr().String()
	udpAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: guard.Addr().(*net.TCPAddr).Port}
	udp, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return err
	}
	udp.Close()
	metrics, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer metrics.Close()
	metricsAddr := metrics.Addr().String()
	inst := Instance{Protocol: model.TrustTunnel, Settings: Settings{Hostname: "quic-probe.invalid"}}
	cert, key, err := certificate(inst, folder)
	if err != nil {
		return err
	}
	credentials := filepath.Join(folder, "credentials.toml")
	if err := writePrivate(credentials, []byte("[[client]]\nusername = \"quic-probe\"\npassword = \"local-fixture-only\"\n")); err != nil {
		return err
	}
	config, err := renderTrustTunnelEndpointConfig(bind, credentials, "127.0.0.1:1", metricsAddr, trustTunnelQUIC)
	if err != nil {
		return err
	}
	vpn, hosts := filepath.Join(folder, "vpn.toml"), filepath.Join(folder, "hosts.toml")
	if err := writePrivate(vpn, []byte(config)); err != nil {
		return err
	}
	if err := writePrivate(hosts, []byte(fmt.Sprintf("[[main_hosts]]\nhostname = %s\ncert_chain_path = %s\nprivate_key_path = %s\n", tomlString(inst.Settings.Hostname), tomlString(cert), tomlString(key)))); err != nil {
		return err
	}
	metrics.Close()
	cmd := exec.CommandContext(ctx, path, vpn, hosts, "--jobs", "1")
	cmd.Dir = folder
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	// Read captured logs only once Wait has joined the output-copy goroutines.
	diagnostic := func() string {
		message := strings.TrimSpace(output.String())
		if len(message) > 1000 {
			message = message[len(message)-1000:]
		}
		return message
	}
	lastCheck := "waiting for metrics"
	for {
		select {
		case <-done:
			return fmt.Errorf("independent QUIC endpoint exited: %s", diagnostic())
		case <-ctx.Done():
			cancel()
			<-done
			return fmt.Errorf("independent QUIC endpoint readiness timed out (%s): %s", lastCheck, diagnostic())
		case <-ticker.C:
			rows, err := readTrustTunnelMetrics(ctx, metricsAddr)
			if err != nil {
				lastCheck = err.Error()
				continue
			}
			identified := false
			for _, row := range rows {
				identified = identified || row.Username == "quic-probe"
			}
			if !identified {
				lastCheck = "test client not present in metrics"
				continue
			}
			if probe, err := net.ListenUDP("udp4", udpAddr); err == nil {
				probe.Close()
				lastCheck = "UDP listener not bound"
				continue
			}
			select {
			case <-done:
				return errors.New("independent QUIC endpoint exited during verification")
			default:
				return nil
			}
		}
	}
}
