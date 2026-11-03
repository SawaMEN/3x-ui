package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

type ManagerPreflightEntry struct {
	Index   int    `json:"index"`
	Ready   bool   `json:"ready"`
	Message string `json:"message"`
}
type ManagerPreflight struct {
	Ready   bool                    `json:"ready"`
	Entries []ManagerPreflightEntry `json:"entries"`
}

func (s *ManagerService) Preflight(ctx context.Context, owner int, sub model.ManagerSubscription) ManagerPreflight {
	sub.UserID = owner
	report := ManagerPreflight{Ready: true, Entries: []ManagerPreflightEntry{}}
	if err := s.validate(&sub); err != nil {
		return ManagerPreflight{Entries: []ManagerPreflightEntry{{Message: err.Error()}}}
	}
	if !sub.Enabled {
		return ManagerPreflight{Entries: []ManagerPreflightEntry{{Message: "enable the subscription before generating connections"}}}
	}
	ports := map[string]bool{}
	for i, e := range sub.Entries {
		result := ManagerPreflightEntry{Index: i + 1, Ready: true, Message: "Ready"}
		if e.Mode != "custom" {
			ib, err := managerEntrySource(e, owner)
			if err == nil {
				if e.NodeID != nil {
					ib.NodeID = e.NodeID
					if *e.NodeID == 0 {
						ib.NodeID = nil
					}
				}
				if e.Mode == "existing" {
					if !ib.Enable {
						err = fmt.Errorf("existing connection is disabled")
					} else {
						ib.Port = 0
						err = s.preflightTarget(ctx, ib)
					}
				}
				if e.Mode != "existing" {
					ib.Port, _ = strconv.Atoi(e.Port)
					if err = s.applySNI(owner, ib, e.SNI); err == nil {
						err = s.preflightTarget(ctx, ib)
					}
					if ib.Port > 0 {
						node := 0
						if ib.NodeID != nil {
							node = *ib.NodeID
						}
						key := fmt.Sprintf("%d/%d", node, ib.Port)
						if ports[key] {
							err = fmt.Errorf("port %d is used more than once in this generation", ib.Port)
						}
						ports[key] = true
					}
				}
			}
			if err != nil {
				result.Ready = false
				result.Message = err.Error()
				report.Ready = false
			}
		}
		report.Entries = append(report.Entries, result)
	}
	return report
}

func (s *ManagerService) preflightTarget(ctx context.Context, ib *model.Inbound) error {
	rt, err := (&InboundService{}).runtimeFor(ib)
	if err != nil {
		return err
	}
	if ib.NodeID != nil {
		var node model.Node
		if err := database.GetDB().First(&node, *ib.NodeID).Error; err != nil {
			return err
		}
		if !node.Enable || node.Status == "offline" {
			return fmt.Errorf("selected node is disabled or offline")
		}
		if err := checkNodeCanHostProtocol(database.GetDB(), node.Id, ib.Protocol); err != nil {
			return err
		}
		if remote, ok := rt.(interface {
			PreflightInbound(context.Context, *model.Inbound) error
		}); ok && node.ProtocolAvailability != nil {
			return remote.PreflightInbound(ctx, ib)
		}
		return nil
	}
	if _, ok := rt.(*runtime.Local); !ok {
		return nil
	}
	return PreflightInbound(ctx, ib)
}

func PreflightInbound(ctx context.Context, ib *model.Inbound) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	kind, err := (&SettingService{}).GetCoreType()
	if err != nil {
		return err
	}
	version, _ := NativeCore(kind).CachedVersion(ctx)
	availability := ProtocolAvailability(kind, version, NativeCore(kind).Installed())
	if c := availability[string(ib.Protocol)]; !c.Ready {
		return fmt.Errorf("%s component is not installed or incompatible with %s", ib.Protocol, kind)
	}
	if err := validateInboundTLSCertificates(ib.StreamSettings); err != nil {
		return err
	}
	if err := managerProbeReality(ctx, ib); err != nil {
		return err
	}
	if err := managerCheckCertificates(ib); err != nil {
		return err
	}
	if ib.Port < 0 || ib.Port > 65535 {
		return fmt.Errorf("invalid port")
	}
	if ib.Port == 0 {
		return nil
	}
	var count int64
	if err := database.GetDB().Model(&model.Inbound{}).Where("port = ? AND node_id IS NULL AND enable = ?", ib.Port, true).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("port %d is already assigned", ib.Port)
	}
	address := net.JoinHostPort(strings.Trim(ib.Listen, "[]"), strconv.Itoa(ib.Port))
	udp := ib.Protocol == model.Hysteria || ib.Protocol == model.TUIC || ib.Protocol == model.WireGuard || ib.Protocol == model.AmneziaWG
	if udp {
		c, err := (&net.ListenConfig{}).ListenPacket(ctx, "udp", address)
		if err != nil {
			return fmt.Errorf("UDP port %d: %w", ib.Port, err)
		}
		return c.Close()
	}
	c, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("TCP port %d: %w", ib.Port, err)
	}
	return c.Close()
}

func managerCheckCertificates(ib *model.Inbound) error {
	var native struct {
		Hostname    string `json:"hostname"`
		Certificate string `json:"certificate"`
		PrivateKey  string `json:"privateKey"`
		TLS         struct {
			Enabled         *bool  `json:"enabled"`
			ServerName      string `json:"serverName"`
			CertificatePath string `json:"certificatePath"`
			KeyPath         string `json:"keyPath"`
		} `json:"tls"`
	}
	if json.Unmarshal([]byte(ib.Settings), &native) == nil {
		certificate, key, name := native.TLS.CertificatePath, native.TLS.KeyPath, native.TLS.ServerName
		if ib.Protocol == model.VetroffProxy {
			certificate, key, name = native.Certificate, native.PrivateKey, native.Hostname
		}
		if (ib.Protocol == model.VetroffProxy || native.TLS.Enabled == nil || *native.TLS.Enabled) && (certificate != "" || key != "") {
			certBytes, err := recoveryReadFile(certificate)
			if err != nil {
				return fmt.Errorf("certificate: %w", err)
			}
			keyBytes, err := recoveryReadFile(key)
			if err != nil {
				return fmt.Errorf("private key: %w", err)
			}
			if err := managerCertificatePair(certBytes, keyBytes, name); err != nil {
				return err
			}
		}
	}

	var stream struct {
		Security string `json:"security"`
		TLS      struct {
			ServerName   string `json:"serverName"`
			Certificates []struct {
				CertificateFile string   `json:"certificateFile"`
				KeyFile         string   `json:"keyFile"`
				Certificate     []string `json:"certificate"`
				Key             []string `json:"key"`
				Usage           string   `json:"usage"`
			} `json:"certificates"`
		} `json:"tlsSettings"`
	}
	if strings.TrimSpace(ib.StreamSettings) != "" {
		if err := json.Unmarshal([]byte(ib.StreamSettings), &stream); err != nil {
			return err
		}
	}
	if !strings.EqualFold(stream.Security, "tls") {
		return nil
	}
	for _, cert := range stream.TLS.Certificates {
		if strings.EqualFold(cert.Usage, "verify") {
			continue
		}
		certificate, key := []byte(strings.Join(cert.Certificate, "\n")), []byte(strings.Join(cert.Key, "\n"))
		var err error
		if cert.CertificateFile != "" {
			certificate, err = recoveryReadFile(cert.CertificateFile)
			if err != nil {
				return fmt.Errorf("certificate: %w", err)
			}
		}
		if cert.KeyFile != "" {
			key, err = recoveryReadFile(cert.KeyFile)
			if err != nil {
				return fmt.Errorf("private key: %w", err)
			}
		}
		if err := managerCertificatePair(certificate, key, stream.TLS.ServerName); err != nil {
			return err
		}
	}
	return nil
}

func managerCertificatePair(certificate, key []byte, name string) error {
	pair, err := tls.X509KeyPair(certificate, key)
	if err != nil {
		return fmt.Errorf("TLS certificate pair: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return err
	}
	if time.Now().Before(leaf.NotBefore) || time.Now().After(leaf.NotAfter) {
		return fmt.Errorf("TLS certificate is not valid at the current time")
	}
	if name != "" {
		return leaf.VerifyHostname(name)
	}
	return nil
}

func managerProbeReality(ctx context.Context, ib *model.Inbound) error {
	var stream struct {
		Security string `json:"security"`
		Reality  struct {
			Target      string   `json:"target"`
			Dest        string   `json:"dest"`
			ServerNames []string `json:"serverNames"`
		} `json:"realitySettings"`
	}
	if strings.TrimSpace(ib.StreamSettings) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(ib.StreamSettings), &stream); err != nil {
		return err
	}
	if stream.Security != "reality" {
		return nil
	}
	target := stream.Reality.Target
	if target == "" {
		target = stream.Reality.Dest
	}
	if target == "" {
		return fmt.Errorf("Reality requires a target")
	}
	host, port, err := splitRealityTarget(target)
	if err != nil {
		return err
	}
	history, _ := SNIScanHistory()
	for _, scan := range history {
		for _, row := range scan.Results {
			checked, err := time.Parse(time.RFC3339, row.CheckedAt)
			if err == nil && checked.After(time.Now().Add(-time.Hour)) && row.Host == host && row.Port == port && row.Feasible && row.CertValid {
				return nil
			}
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, err := (&ServerService{}).ScanRealityTargetsContext(probeCtx, target)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Feasible && row.CertValid {
			return nil
		}
	}
	return fmt.Errorf("Reality target %s is not ready from this server; check it in the SNI scanner", target)
}
