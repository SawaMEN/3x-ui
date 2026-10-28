package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// Runtime pauses keep operator intent: a crash/restart must not permanently
// disable Gateway, while Suspend used by Disable/core-switch changes intent.
func (s *GatewayNetworkService) pauseRuntime(ctx context.Context) error {
	if _, err := s.loadGatewayNetworkState(); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return s.removeGatewayTables(ctx)
}

func (s *GatewayNetworkService) resumeRuntime(ctx context.Context) error {
	state, err := s.loadGatewayNetworkState()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !state.DesiredActive {
		return nil
	}
	if err := s.Enable(ctx, state.Config); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return errors.Join(fmt.Errorf("restore Gateway network: %w", err), s.pauseRuntime(cleanup))
	}
	return nil
}

func (s *SingBoxService) gatewayNetwork() *GatewayNetworkService {
	if s.gateway != nil {
		return s.gateway
	}
	return &GatewayNetworkService{}
}

// Called after candidate validation, before giving up listeners. Restoration
// also covers rollback to the previous process after failed startup.
func (s *SingBoxService) pauseGateway(ctx context.Context) (func() error, error) {
	network := s.gatewayNetwork()
	release, err := network.runtimeLease(ctx)
	if err != nil {
		return nil, err
	}
	if err := network.pauseRuntime(ctx); err != nil {
		release()
		return nil, err
	}
	return func() error {
		defer release()
		if !s.IsRunning() {
			return nil
		}
		recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.resumeGatewayNetwork(recovery, network)
	}, nil
}

// The watchdog also repairs network state after a crash, an interrupted panel
// restart or a failed firewall restore. It never enables an operator suspension.
func (s *SingBoxService) ReconcileGateway(ctx context.Context) error {
	if s.coreType() == CoreTypeXray {
		return nil
	}
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	selected, err := singBoxSettingService.GetCoreType()
	if err != nil {
		return err
	}
	if selected != s.coreType() {
		return nil
	}
	network := s.gatewayNetwork()
	release, err := network.runtimeLease(ctx)
	if err != nil {
		return err
	}
	defer release()
	state, err := network.loadGatewayNetworkState()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !s.IsRunning() || !state.DesiredActive {
		return network.pauseRuntime(ctx)
	}
	if !s.hasAppliedGatewayListener() {
		return s.resumeGatewayNetwork(ctx, network)
	}
	status := network.Status(ctx)
	if !status.ListenerReady {
		return errors.Join(fmt.Errorf("Gateway TCP/UDP listener is not ready"), network.pauseRuntime(ctx))
	}
	if status.NFTables && status.PolicyRoute && status.Forwarding && status.RPFilter && status.NAT && status.Persistent {
		return nil
	}
	return s.resumeGatewayNetwork(ctx, network)
}

func (s *GatewayNetworkService) runtimeLease(ctx context.Context) (func(), error) {
	if _, err := s.loadGatewayNetworkState(); os.IsNotExist(err) {
		return func() {}, nil
	} else if err != nil {
		return nil, err
	}
	return gatewayRuntimeLock(ctx, s.networkPaths().state+".runtime.lock")
}

func (s *SingBoxService) resumeGatewayNetwork(ctx context.Context, network *GatewayNetworkService) error {
	state, err := network.loadGatewayNetworkState()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !state.DesiredActive {
		return nil
	}
	if !s.hasAppliedGatewayListener() {
		return errors.Join(fmt.Errorf("active Gateway has no TPROXY listener in applied %s configuration", s.coreType()), network.pauseRuntime(ctx))
	}
	return network.resumeRuntime(ctx)
}

func (s *SingBoxService) resumeGateway(ctx context.Context) error {
	network := s.gatewayNetwork()
	release, err := network.runtimeLease(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.resumeGatewayNetwork(ctx, network)
}

func (s *SingBoxService) hasAppliedGatewayListener() bool {
	var applied struct {
		Inbounds []map[string]any `json:"inbounds"`
	}
	_ = json.Unmarshal(s.process().AppliedConfig(), &applied)
	listener := false
	for _, ib := range applied.Inbounds {
		if ib["tag"] == "in-tproxy" && ib["type"] == "tproxy" && ib["listen_port"] == float64(52345) {
			listener = true
			break
		}
	}

	return listener
}

// A failed candidate must not leave interception pointing at an already dead
// process while waiting for the next watchdog tick.
func (s *SingBoxService) guardStoppedGateway(ctx context.Context) error {
	finish, err := s.pauseGateway(ctx)
	if err != nil {
		return err
	}
	return finish()
}
