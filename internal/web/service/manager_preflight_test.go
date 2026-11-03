package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestManagerCertificateCheckIgnoresInactiveSecuritySettings(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "old-certificate.pem")
	for _, security := range []string{"none", "reality", "tls"} {
		t.Run(security, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"security": security, "tlsSettings": map[string]any{"certificates": []map[string]any{{"certificateFile": missing, "keyFile": missing}}}})
			if err != nil {
				t.Fatal(err)
			}
			ib := &model.Inbound{Protocol: model.VLESS, Settings: `{}`, StreamSettings: string(raw)}
			err = managerCheckCertificates(ib)
			if security == "tls" {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("active TLS certificate was not checked: %v", err)
				}
			} else if err != nil {
				t.Fatalf("inactive TLS settings blocked %s: %v", security, err)
			}
		})
	}
	raw, err := json.Marshal(map[string]any{"tls": map[string]any{"enabled": false, "certificatePath": missing, "keyPath": missing}})
	if err != nil {
		t.Fatal(err)
	}
	if err := managerCheckCertificates(&model.Inbound{Protocol: model.Mieru, Settings: string(raw), StreamSettings: `{}`}); err != nil {
		t.Fatalf("disabled native TLS checked: %v", err)
	}
}

func TestManagerPreflightPortTransports(t *testing.T) {
	setupConflictDB(t)
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	port := tcp.Addr().(*net.TCPAddr).Port
	existing := &model.Inbound{Tag: "manager-tcp", Enable: true, Listen: "127.0.0.1", Port: port, Protocol: model.VLESS, Settings: `{}`, StreamSettings: `{"network":"tcp"}`}
	if err := database.GetDB().Create(existing).Error; err != nil {
		t.Fatal(err)
	}
	candidate := &model.Inbound{Listen: "127.0.0.1", Port: port, Protocol: model.Hysteria, Settings: `{}`, StreamSettings: `{}`}
	if err := managerPreflightPorts(context.Background(), candidate); err != nil {
		t.Fatalf("TCP and UDP may share a port: %v", err)
	}
	candidate.Protocol = model.VLESS
	if err := managerPreflightPorts(context.Background(), candidate); err == nil {
		t.Fatal("TCP collision accepted")
	}
}

func TestManagerPreflightChecksSnellQUICListener(t *testing.T) {
	setupConflictDB(t)
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	ib := &model.Inbound{Listen: "127.0.0.1", Port: udp.LocalAddr().(*net.UDPAddr).Port, Protocol: model.Snell, Settings: `{"version":5,"quicProxy":true}`, StreamSettings: `{}`}
	if err := managerPreflightPorts(context.Background(), ib); err == nil {
		t.Fatal("occupied QUIC UDP listener was not checked")
	}
}

func TestManagerGenerationPortsRespectTransportAndDestination(t *testing.T) {
	tcp := &model.Inbound{Protocol: model.VLESS, Port: 443, Listen: "0.0.0.0", StreamSettings: `{"network":"tcp"}`}
	udp := &model.Inbound{Protocol: model.Hysteria, Port: 443, Listen: "0.0.0.0"}
	if managerGenerationPortConflict(tcp, udp) {
		t.Fatal("TCP and UDP incorrectly overlap")
	}
	clone := *tcp
	if !managerGenerationPortConflict(tcp, &clone) {
		t.Fatal("TCP collision not detected")
	}
	node := 1
	clone.NodeID = &node
	if managerGenerationPortConflict(tcp, &clone) {
		t.Fatal("different nodes incorrectly overlap")
	}
	snell := &model.Inbound{Protocol: model.Snell, Port: 443, Settings: `{"version":5,"quicProxy":true}`}
	if !managerGenerationPortConflict(snell, udp) {
		t.Fatal("Snell QUIC collision not detected")
	}
}
