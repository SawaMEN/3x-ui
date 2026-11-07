package service

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

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
		want     bool
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

func TestProbeNodeRouteQUICVersionNegotiation(t *testing.T) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		request := make([]byte, 1500)
		_ = listener.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, peer, err := listener.ReadFromUDP(request)
		if err != nil {
			done <- err
			return
		}
		if n != 1200 || request[5] != 8 || request[14] != 8 {
			done <- fmt.Errorf("unexpected QUIC probe packet")
			return
		}
		response := make([]byte, 27)
		response[0] = 0xc0
		// Version negotiation sets version=0 and echoes connection IDs
		// in the inverse order, followed by a supported version.
		response[5] = 8
		copy(response[6:14], request[15:23])
		response[14] = 8
		copy(response[15:23], request[6:14])
		binary.BigEndian.PutUint32(response[23:27], 1)
		_, err = listener.WriteToUDP(response, peer)
		done <- err
	}()
	duration, err := probeNodeRouteQUIC(context.Background(), "127.0.0.1", listener.LocalAddr().(*net.UDPAddr).Port)
	if err != nil || duration < 0 {
		t.Fatalf("QUIC version negotiation failed: duration=%s err=%v", duration, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestProbeNodeRouteQUICRejectsWrongConnectionIDs(t *testing.T) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		request := make([]byte, 1500)
		_ = listener.SetReadDeadline(time.Now().Add(time.Second))
		_, peer, err := listener.ReadFromUDP(request)
		if err != nil {
			return
		}
		response := make([]byte, 27)
		response[0] = 0xc0
		response[5] = 8
		response[14] = 8
		binary.BigEndian.PutUint32(response[23:], 1)
		_, _ = listener.WriteToUDP(response, peer)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if _, err := probeNodeRouteQUIC(ctx, "127.0.0.1", listener.LocalAddr().(*net.UDPAddr).Port); err == nil {
		t.Fatal("accepted a response from the wrong QUIC connection")
	}
	<-done
}
