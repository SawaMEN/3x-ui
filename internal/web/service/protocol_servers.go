package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/protocolserver"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func prepareProtocolServers(cfg *singbox.Config, reserved ...int) error {
	return prepareProtocolServersWithPolicy(cfg, "", reserved...)
}

func prepareProtocolServersWithPolicy(cfg *singbox.Config, strict model.Protocol, reserved ...int) error {
	occupied := map[int]bool{10086: true, 9090: true}
	for _, port := range reserved {
		occupied[port] = true
	}
	for _, inbound := range append(append([]map[string]any{}, cfg.Inbounds...), cfg.Endpoints...) {
		var port int
		_, _ = fmt.Sscan(fmt.Sprint(inbound["listen_port"]), &port)
		occupied[port] = true
	}
	for i, inbound := range cfg.Inbounds {
		protocol := model.NaiveProxy
		switch inbound["type"] {
		case "naive":
		case "hysteria2":
			protocol = model.Hysteria
		default:
			continue
		}
		if !protocolserver.Installed(protocol) {
			continue
		}
		tag, _ := inbound["tag"].(string)
		if reason := cfg.ProtocolServerFallbacks[tag]; reason != "" {
			if strict == protocol {
				return fmt.Errorf("%s", reason)
			}
			continue
		}
		ports := [3]int{}
		for j := range ports {
			port, err := protocolserver.Port(fmt.Sprintf("%s:%d", tag, j), occupied)
			if err != nil {
				return err
			}
			ports[j] = port
		}
		plan, bridge, err := protocolserver.Build(inbound, ports)
		if err == nil {
			candidate := *cfg
			candidate.ProtocolServers = []protocolserver.Plan{plan}
			err = validateHysteriaUserRoutes(&candidate)
		}
		if err != nil {
			if strict == protocol {
				return err
			}
			if cfg.ProtocolServerFallbacks == nil {
				cfg.ProtocolServerFallbacks = map[string]string{}
			}
			cfg.ProtocolServerFallbacks[tag] = err.Error()
			continue
		}
		cfg.Inbounds[i] = bridge
		cfg.ProtocolServers = append(cfg.ProtocolServers, plan)
	}
	return nil
}

func (s *SingBoxService) restoreProtocolServerRuntime(previous []byte, plans []protocolserver.Plan, wasRunning bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result := protocolserver.StopAll()
	result = errors.Join(result, s.stopProcess(ctx))
	if !wasRunning || len(previous) == 0 {
		return result
	}
	if err := writeNativeConfigBytes(s.ProcessConfigPath(), previous); err != nil {
		return errors.Join(result, err)
	}
	if err := s.startProcess(ctx); err != nil {
		return errors.Join(result, err)
	}
	return errors.Join(result, protocolserver.Start(ctx, plans))
}

func (s *SingBoxService) pollProtocolServerTraffic(ctx context.Context) error {
	snapshots, err := protocolserver.Snapshots(ctx)
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		stats := make([]singbox.V2RayStat, 0, len(snapshot.Counters)*2)
		for email, traffic := range snapshot.Counters {
			stats = append(stats, singbox.V2RayStat{Name: "user>>>" + email + ">>>traffic>>>uplink", Value: traffic.Tx}, singbox.V2RayStat{Name: "user>>>" + email + ">>>traffic>>>downlink", Value: traffic.Rx})
		}
		if err := s.commitHiddifySnapshot(snapshot.Epoch, stats, snapshot.Namespace); err != nil {
			return err
		}
	}
	return nil
}

var protocolInstallMu sync.Mutex

// Restore the old binary and runtime if the new listener cannot serve its configuration.
func InstallProtocolServer(ctx context.Context, protocol model.Protocol, install func(context.Context) error) error {
	return mutateProtocolServer(ctx, protocol, install, true)
}

func mutateProtocolServer(ctx context.Context, protocol model.Protocol, install func(context.Context) error, strict bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	protocolInstallMu.Lock()
	defer protocolInstallMu.Unlock()
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	path := protocolserver.BinaryPath(protocol)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var backup string
	original, statErr := os.Stat(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}
	if statErr == nil {
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		target, err := os.CreateTemp(filepath.Dir(path), ".protocol-previous-*")
		if err != nil {
			_ = source.Close()
			return err
		}
		backup = target.Name()
		defer os.Remove(backup)
		_, copyErr := io.Copy(target, source)
		err = errors.Join(copyErr, source.Close(), target.Close(), os.Chmod(backup, original.Mode().Perm()))
		if err != nil {
			return err
		}
		if err := os.Chtimes(backup, original.ModTime(), original.ModTime()); err != nil {
			return err
		}
	}
	rollback := func() error {
		if backup != "" {
			return os.Rename(backup, path)
		}
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	previouslyInstalled := protocolserver.Installed(protocol)
	if err := install(ctx); err != nil {
		return errors.Join(err, rollback())
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, rollback())
	}
	nativeCopy := *SelectedNativeCore()
	if strict && !previouslyInstalled {
		nativeCopy.strictProtocolServers = protocol
	} else {
		nativeCopy.strictProtocolServers = ""
	}
	native := &nativeCopy
	if !native.IsRunning() {
		cfg, err := native.GetConfig()
		if err == nil {
			err = protocolserver.Check(ctx, cfg.ProtocolServers)
		}
		if err != nil {
			return errors.Join(err, rollback())
		}
		return nil
	}
	previous := append([]byte(nil), native.process().AppliedConfig()...)
	if err := native.restartLocked(ctx); err != nil {
		restoreErr := rollback()
		if restoreErr != nil {
			return errors.Join(err, restoreErr)
		}
		if native.IsRunning() && bytes.Equal(previous, native.process().AppliedConfig()) && !protocolserver.Changed() {
			return err
		}
		recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		native.strictProtocolServers = ""
		return errors.Join(err, native.restartLocked(recovery))
	}
	return nil
}

func validateHysteriaUserRoutes(cfg *singbox.Config) error {
	var rules []map[string]any
	data, _ := json.Marshal(cfg.Route["rules"])
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}
	for _, plan := range cfg.ProtocolServers {
		if hasSourceRule(rules, plan.Tag, "source_ip_cidr", "source_ip_is_private", "source_port", "source_port_range") {
			return fmt.Errorf("standalone servers cannot preserve routing by original client source IP or port; use sing-box")
		}
		if plan.Protocol != model.Hysteria {
			continue
		}
		if hasSourceRule(rules, plan.Tag, "auth_user") {
			return fmt.Errorf("standalone Hysteria cannot preserve routing by authenticated user; use inbound tags or use sing-box")
		}
	}
	return nil
}

func hasSourceRule(rules []map[string]any, tag string, fields ...string) bool {
	for _, rule := range rules {
		if inbound := rule["inbound"]; inbound != nil {
			data, _ := json.Marshal(inbound)
			var tags []string
			if err := json.Unmarshal(data, &tags); err == nil && len(tags) > 0 {
				matched := false
				for _, value := range tags {
					if value == tag {
						matched = true
					}
				}
				if !matched && rule["invert"] != true {
					continue
				}
			}
		}
		for _, field := range fields {
			if user := rule[field]; user != nil && user != false {
				data, _ := json.Marshal(user)
				if string(data) != "[]" && string(data) != `""` {
					return true
				}
			}
		}
		data, _ := json.Marshal(rule["rules"])
		var nested []map[string]any
		if json.Unmarshal(data, &nested) == nil && hasSourceRule(nested, tag, fields...) {
			return true
		}
	}
	return false
}

// Report the planned listener owner, including compatibility fallbacks on upgrade.
func ProtocolServerStatus(protocol model.Protocol) (string, string) {
	cfg, err := SelectedNativeCore().GetConfig()
	if err != nil {
		return "sing-box", err.Error()
	}
	count := 0
	for _, plan := range cfg.ProtocolServers {
		if plan.Protocol == protocol {
			count++
		}
	}
	kind := "naive"
	if protocol == model.Hysteria {
		kind = "hysteria2"
	}
	var reasons []string
	for _, inbound := range cfg.Inbounds {
		if inbound["type"] != kind {
			continue
		}
		tag, _ := inbound["tag"].(string)
		if reason := cfg.ProtocolServerFallbacks[tag]; reason != "" {
			reasons = append(reasons, tag+": "+reason)
		}
	}
	if len(reasons) > 0 {
		source := "sing-box"
		if count > 0 {
			source = "mixed"
		}
		return source, strings.Join(reasons, "; ")
	}
	if protocolserver.Installed(protocol) {
		return "standalone", ""
	}
	return "sing-box", ""
}
