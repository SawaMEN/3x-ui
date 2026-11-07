package service

import (
	"context"
	"net"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestProbeNodeRouteTCPDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if elapsed, err := probeNodeRouteTCP(context.Background(), "127.0.0.1", port); err != nil || elapsed < 0 {
		t.Fatalf("TCP listener could not be reached: duration=%s err=%v", elapsed, err)
	}
	if _, err := probeNodeRouteTCP(context.Background(), "127.0.0.1", 0); err == nil {
		t.Fatal("invalid port accepted")
	}
}

func TestNodeRouteIsUDP(t *testing.T) {
	tests := []struct {
		protocol model.Protocol
		settings string
		want bool
	}{
		{model.VLESS, "{}", false},
		{model.Hysteria, "{}", true},
		{model.TUIC, "{}", true},
		{model.VetroffProxy, `{"vetroff":{"transport":"h3-connect"}}`, true},
		{model.VetroffProxy, `{"vetroff":{"transport":"h2-connect"}}`, false},
	}
	for _, tt := range tests {
		ib := &model.Inbound{Protocol: tt.protocol, Settings: tt.settings}
		if got := nodeRouteIsUDP(ib); got != tt.want {
			t.Errorf("protocol=%s settings=%s: got %v want %v", tt.protocol, tt.settings, got, tt.want)
		}
	}
}
