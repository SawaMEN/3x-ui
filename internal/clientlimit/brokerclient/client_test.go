package brokerclient

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestHalfCloseFallsBackToClosingUnsupportedStream(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	client, err := NewWithConfig(Config{Address: "127.0.0.1:1", Password: "private-test", Users: map[string]string{"alice": "route"}}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &limitedConn{Conn: a, client: client}
	if err = wrapped.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	_ = b.SetReadDeadline(time.Now().Add(time.Second))
	if n, err := b.Read(make([]byte, 1)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("unsupported half-close left a live tunnel: %d %v", n, err)
	}
}
