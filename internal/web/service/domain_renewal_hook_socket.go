package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

type externalRenewalHookRequest struct {
	Action  string `json:"action"`
	Manager string `json:"manager"`
}

type externalRenewalHookResponse struct {
	Error string `json:"error,omitempty"`
}

var externalRenewalHookMu sync.Mutex
var externalRenewalHookSessions = map[string][]string{}

func certificateRenewalSocketPath() string {
	return filepath.Join(config.GetDBFolderPath(), "certificate-renewal.sock")
}

// Only certificates owned by the local host may request firewall access.
// DNS-01, imported files, and remotely owned SNI never open incoming ports.
func externalCertificateHookPorts(items []CertificateInventoryItem, manager string) ([]int, error) {
	if manager != "certbot" && manager != "acme.sh" {
		return nil, errors.New("unsupported certificate renewal manager")
	}
	seen := map[int]bool{}
	for _, item := range items {
		if item.Manager != manager || item.Source == "remote" {
			continue
		}
		for _, port := range item.Ports {
			if port > 0 && port <= 65535 {
				seen[port] = true
			}
		}
	}
	ports := make([]int, 0, len(seen))
	for port := range seen {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports, nil
}

func externalCertificateHookPortsCurrent(ctx context.Context, manager string) ([]int, error) {
	status, err := DomainFrontStatus()
	if err != nil {
		return nil, err
	}
	var telemt *TelemtWebProxyState
	if state, readErr := readTelemtWebState(); readErr == nil && state.Enabled {
		telemt = &state
	}
	items := makeCertificateInventory(status, domainACMEStates(domainACMEHomes()), telemt)
	return externalCertificateHookPorts(items, manager)
}

func executeExternalCertificateHook(ctx context.Context, request externalRenewalHookRequest) error {
	if request.Action != "begin" && request.Action != "end" {
		return errors.New("unknown certificate renewal action")
	}
	if request.Manager != "certbot" && request.Manager != "acme.sh" {
		return errors.New("unknown certificate renewal manager")
	}
	externalRenewalHookMu.Lock()
	defer externalRenewalHookMu.Unlock()

	if request.Action == "end" {
		ids := externalRenewalHookSessions[request.Manager]
		if len(ids) == 0 {
			return nil
		}
		id := ids[len(ids)-1]
		if len(ids) == 1 {
			delete(externalRenewalHookSessions, request.Manager)
		} else {
			externalRenewalHookSessions[request.Manager] = ids[:len(ids)-1]
		}
		return RenewalFirewallEnd(id)
	}
	ports, err := externalCertificateHookPortsCurrent(ctx, request.Manager)
	if err != nil {
		return err
	}
	id, err := RenewalFirewallBegin(ctx, "external-hook:"+request.Manager, ports)
	if err != nil {
		return err
	}
	if id != "" {
		externalRenewalHookSessions[request.Manager] = append(externalRenewalHookSessions[request.Manager], id)
	}
	return nil
}

// ServeCertificateRenewalHookSocket uses a local, root-only Unix socket, not a
// public HTTP API. Native scheduled Certbot/acme.sh hooks contact the already
// running panel and share its firewall lock, avoiding cross-process races.
func ServeCertificateRenewalHookSocket(ctx context.Context) {
	if os.Geteuid() != 0 {
		return
	}
	socket := certificateRenewalSocketPath()
	dir := filepath.Dir(socket)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		logger.Warning("renewal hook socket directory:", err)
		return
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm()&0o022 != 0 {
		logger.Warning("renewal hook socket directory has unsafe permissions")
		return
	}
	if info, statErr := os.Lstat(socket); statErr == nil {
		if info.Mode()&os.ModeSocket == 0 {
			logger.Warning("renewal socket pathname is not a socket; refusing to replace it")
			return
		}
		probe, dialErr := net.DialTimeout("unix", socket, 200*time.Millisecond)
		if dialErr == nil {
			_ = probe.Close()
			logger.Warning("renewal socket is already active")
			return
		}
		if err := os.Remove(socket); err != nil {
			logger.Warning("cannot replace inactive renewal socket:", err)
			return
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		logger.Warning("cannot inspect renewal socket:", statErr)
		return
	}
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "unix", socket)
	if err != nil {
		logger.Warning("renewal hook socket:", err)
		return
	}
	defer listener.Close()
	defer os.Remove(socket)
	if err := os.Chmod(socket, 0o600); err != nil {
		logger.Warning("renewal hook socket permissions:", err)
		return
	}
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
		case <-stop:
		}
	}()
	defer close(stop)

	for {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			if ctx.Err() == nil {
				logger.Warning("renewal hook listener:", acceptErr)
			}
			return
		}
		go func() {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
			var req externalRenewalHookRequest
			if err := json.NewDecoder(conn).Decode(&req); err != nil {
				_ = json.NewEncoder(conn).Encode(externalRenewalHookResponse{Error: "invalid request"})
				return
			}
			actionCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			resp := externalRenewalHookResponse{}
			if err := executeExternalCertificateHook(actionCtx, req); err != nil {
				resp.Error = err.Error()
			}
			_ = json.NewEncoder(conn).Encode(resp)
		}()
	}
}

// InvokeCertificateRenewalHook is called by a root-owned native scheduler hook
// through the x-ui CLI. It never opens a firewall or database in a new process.
func InvokeCertificateRenewalHook(ctx context.Context, action, manager string) error {
	if (action != "begin" && action != "end") || (manager != "certbot" && manager != "acme.sh") {
		return errors.New("usage: certificate-hook begin|end certbot|acme.sh")
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", certificateRenewalSocketPath())
	if err != nil {
		return fmt.Errorf("panel certificate renewal socket unavailable: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	if err := json.NewEncoder(conn).Encode(externalRenewalHookRequest{Action: action, Manager: manager}); err != nil {
		return err
	}
	var result externalRenewalHookResponse
	if err := json.NewDecoder(conn).Decode(&result); err != nil {
		return err
	}
	if result.Error != "" {
		return errors.New(result.Error)
	}
	return nil
}
