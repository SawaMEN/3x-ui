package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
)

func validateTelemtTrustTunnelMove(move telemtVPNMigrationPlan) error {
	host := strings.Trim(move.OldListen, "[]")
	if host == "" {
		host = "0.0.0.0"
	}
	if move.Protocol != string(model.TrustTunnel) || move.OldPort != 443 || net.ParseIP(host) == nil ||
		move.TrustTunnelQUICBind != net.JoinHostPort(host, strconv.Itoa(move.OldPort)) {
		return errors.New("invalid TrustTunnel QUIC migration journal entry")
	}
	return nil
}

func patchTelemtTrustTunnelBind(settings string, move telemtVPNMigrationPlan, restore bool) (string, error) {
	if move.TrustTunnelQUICBind == "" {
		return settings, nil
	}
	if err := validateTelemtTrustTunnelMove(move); err != nil {
		return "", err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(settings), &fields); err != nil || fields == nil {
		return "", errors.New("invalid TrustTunnel settings")
	}
	var current string
	if raw, ok := fields["trustTunnelQuicBind"]; ok {
		if err := json.Unmarshal(raw, &current); err != nil {
			return "", errors.New("invalid TrustTunnel QUIC bind")
		}
	}
	if restore {
		if current != move.TrustTunnelQUICBind {
			return "", errors.New("TrustTunnel QUIC bind changed; refusing unsafe rollback")
		}
		delete(fields, "trustTunnelQuicBind")
	} else {
		var tcpOnly bool
		if raw, ok := fields["trustTunnelTcpOnly"]; ok {
			if err := json.Unmarshal(raw, &tcpOnly); err != nil {
				return "", errors.New("invalid TrustTunnel transport mode")
			}
		}
		if tcpOnly || current != "" {
			return "", errors.New("TrustTunnel transport mode changed")
		}
		raw, err := json.Marshal(move.TrustTunnelQUICBind)
		if err != nil {
			return "", err
		}
		fields["trustTunnelQuicBind"] = raw
	}
	raw, err := json.Marshal(fields)
	return string(raw), err
}

func verifyTelemtTrustTunnelTransport(ctx context.Context, move telemtVPNMigrationPlan, migrated bool) error {
	port := move.OldPort
	if migrated {
		port = move.NewPort
	}
	if move.TrustTunnelQUICBind == "" {
		return verifyTelemtTrustTunnelTCPOnly(ctx, port)
	}
	if err := validateTelemtTrustTunnelMove(move); err != nil {
		return err
	}
	pid, err := externalvpn.GetManager().TrustTunnelQUICPID(move.ID, move.TrustTunnelQUICBind)
	if err != nil {
		return err
	}
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probe, "ss", "-lunpH", "sport = :443").Output()
	if err != nil {
		return fmt.Errorf("inspect managed TrustTunnel QUIC socket: %w", err)
	}
	if err := validateTelemtTrustTunnelQUICSocket(string(output), move.TrustTunnelQUICBind, pid); err != nil {
		return err
	}
	if migrated {
		return verifyTelemtTrustTunnelTCPOnly(ctx, move.NewPort)
	}
	return nil
}

func validateTelemtTrustTunnelQUICSocket(output, bind string, pid int) error {
	host, port, err := net.SplitHostPort(bind)
	if err != nil || net.ParseIP(host) == nil || pid <= 0 {
		return errors.New("invalid QUIC verification target")
	}
	expected := net.ParseIP(host)
	found := false
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		actualHost, actualPort, err := net.SplitHostPort(fields[3])
		if err != nil || actualPort != port || (!expected.Equal(net.ParseIP(actualHost)) && (!expected.IsUnspecified() || actualHost != "*")) {
			continue
		}
		matches := telemtMigrationPIDPattern.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			return errors.New("QUIC socket owner cannot be verified")
		}
		for _, match := range matches {
			if match[1] != strconv.Itoa(pid) {
				return errors.New("QUIC socket is owned by an unexpected process")
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("managed TrustTunnel QUIC listener %s not found", bind)
	}
	return nil
}
