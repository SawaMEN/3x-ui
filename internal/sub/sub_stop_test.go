package sub

import (
	"net"
	"testing"
)

func TestStopAfterListenerIsClosed(t *testing.T) {
	srv := NewServer()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.listener = listener
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop reported a normal listener closure as an error: %v", err)
	}
}
