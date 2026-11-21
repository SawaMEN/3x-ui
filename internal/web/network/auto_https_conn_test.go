package network

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"testing"
	"time"
)

func TestAutoHttpsConnRedirectsRequests(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parts []string
	}{
		{"fragmented origin form", []string{
			"GET /a%",
			"2Fb?q=1 HTTP/1.1\r\nHost: example.test:2053\r\n",
			"\r\n",
		}},
		{"absolute form", []string{
			"GET http://example.test:2053/a%2Fb?q=1 HTTP/1.1\r\nHost: example.test:2053\r\n\r\n",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, client := net.Pipe()
			t.Cleanup(func() { _ = server.Close(); _ = client.Close() })
			deadline := time.Now().Add(2 * time.Second)
			_ = server.SetDeadline(deadline)
			_ = client.SetDeadline(deadline)
			conn := NewAutoHttpsConn(server)
			finished := make(chan error, 1)
			go func() {
				var b [1]byte
				_, err := conn.Read(b[:])
				finished <- err
			}()
			for _, part := range tc.parts {
				if _, err := io.WriteString(client, part); err != nil {
					t.Fatal(err)
				}
			}
			res, err := http.ReadResponse(bufio.NewReader(client), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusTemporaryRedirect || res.Header.Get("Location") != "https://example.test:2053/a%2Fb?q=1" {
				t.Fatalf("redirect was %d %q", res.StatusCode, res.Header.Get("Location"))
			}
			if err := <-finished; !errors.Is(err, io.EOF) {
				t.Fatalf("Read after the redirect returned %v, want EOF", err)
			}
		})
	}
}

func TestAutoHttpsConnPassesThroughIncompleteTLSRecord(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close(); _ = client.Close() })
	deadline := time.Now().Add(2 * time.Second)
	_ = server.SetDeadline(deadline)
	_ = client.SetDeadline(deadline)
	conn := NewAutoHttpsConn(server)
	go func() { _, _ = client.Write([]byte{0x16}) }()
	var b [1]byte
	n, err := conn.Read(b[:])
	if err != nil || n != 1 || b[0] != 0x16 {
		t.Fatalf("Read returned %x (%d bytes), %v", b, n, err)
	}
}

func TestAutoHttpsConnConcurrentReadsPreserveBufferedTLSBytes(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = server.Close(); _ = client.Close() })
	deadline := time.Now().Add(2 * time.Second)
	_ = server.SetDeadline(deadline)
	_ = client.SetDeadline(deadline)
	conn := NewAutoHttpsConn(server)
	want := []byte{0x16, 0x17, 0x18, 0x19}
	go func() { _, _ = client.Write(want) }()
	type readResult struct {
		data byte
		err  error
	}
	result := make(chan readResult, len(want))
	for range want {
		go func() {
			var b [1]byte
			_, err := io.ReadFull(conn, b[:])
			result <- readResult{b[0], err}
		}()
	}
	got := make([]byte, 0, len(want))
	for range want {
		read := <-result
		if read.err != nil {
			t.Fatal(read.err)
		}
		got = append(got, read.data)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("got %x, want %x", got, want)
	}
}
