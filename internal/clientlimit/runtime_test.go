package clientlimit

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

func bridge(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(15 * time.Second))
				var greeting [3]byte
				if _, err := io.ReadFull(c, greeting[:]); err != nil {
					return
				}
				if greeting != [3]byte{5, 1, 2} {
					return
				}
				_, _ = c.Write([]byte{5, 2})
				var version [1]byte
				if _, err = io.ReadFull(c, version[:]); err != nil {
					return
				}
				if _, err = readString(c); err != nil {
					return
				}
				if _, err = readString(c); err != nil {
					return
				}
				_, _ = c.Write([]byte{1, 0})
				var request [4]byte
				if _, err = io.ReadFull(c, request[:]); err != nil {
					return
				}
				if _, err = readAddress(c, request[3]); err != nil {
					return
				}
				if request[1] == 1 {
					_, _ = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
					_, _ = io.Copy(c, c)
					return
				}
				u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
				if err != nil {
					return
				}
				defer u.Close()
				p := u.LocalAddr().(*net.UDPAddr).Port
				_, _ = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, byte(p >> 8), byte(p)})
				go func() {
					b := make([]byte, 65536)
					for {
						n, a, e := u.ReadFromUDP(b)
						if e != nil {
							return
						}
						_, _ = u.WriteToUDP(b[:n], a)
					}
				}()
				_, _ = io.Copy(io.Discard, c)
			}()
		}
	}()
	return ln.Addr().String()
}

func start(t *testing.T, routes []Route) *Runtime {
	t.Helper()
	s := New("127.0.0.1:0", bridge(t))
	t.Cleanup(s.Close)
	finish, err := s.Apply(Plan{routes})
	if err != nil {
		t.Fatal(err)
	}
	finish(true)
	return s
}

func tcp(t *testing.T, s *Runtime, id string) net.Conn {
	t.Helper()
	dial, err := proxy.SOCKS5("tcp", s.Address(), &proxy.Auth{User: id, Password: s.Password()}, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	c, err := dial.Dial("tcp", "example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	return c
}

func TestSharedBudgetAcrossConnectionsAndRoutes(t *testing.T) {
	s := start(t, []Route{{"one", "alice", 1, 0, nil}, {"two", "alice", 1, 0, nil}, {"other", "bob", 0, 0, nil}})
	connections := []net.Conn{tcp(t, s, "one"), tcp(t, s, "two"), tcp(t, s, "one")}
	data := bytes.Repeat([]byte{'x'}, 128*1024)
	started := time.Now()
	var wg sync.WaitGroup
	for _, c := range connections {
		wg.Add(1)
		go func() {
			defer wg.Done()
			go func() { _, _ = c.Write(data) }()
			received := make([]byte, len(data))
			if _, err := io.ReadFull(c, received); err != nil {
				t.Error(err)
			}
			if !bytes.Equal(data, received) {
				t.Error("payload changed")
			}
		}()
	}
	wg.Wait()
	if elapsed := time.Since(started); elapsed < 2300*time.Millisecond {
		t.Fatalf("parallel flows bypassed shared 1 Mbps upload cap: %v", elapsed)
	}
	c := tcp(t, s, "other")
	started = time.Now()
	go func() { _, _ = c.Write(data) }()
	received := make([]byte, len(data))
	if _, err := io.ReadFull(c, received); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("unlimited client was throttled: %v", elapsed)
	}
}

func TestDownloadBudgetAndCancellation(t *testing.T) {
	s := start(t, []Route{{"one", "alice", 0, 1, nil}})
	c := tcp(t, s, "one")
	data := bytes.Repeat([]byte{'d'}, 256*1024)
	started := time.Now()
	go func() { _, _ = c.Write(data) }()
	if _, err := io.ReadFull(c, make([]byte, len(data))); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 1400*time.Millisecond {
		t.Fatalf("download was not limited: %v", elapsed)
	}
	finished, err := s.Apply(Plan{})
	if err != nil {
		t.Fatal(err)
	}
	finished(true)
	if s.Address() != "" {
		t.Fatal("disabled limiter kept listener open")
	}
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err = c.Read(make([]byte, 1)); err == nil {
		t.Fatal("old limited connection survived removal")
	}
}

func TestAuthenticationAndRollback(t *testing.T) {
	s := start(t, []Route{{"old", "alice", 1, 1, nil}})
	dial, err := proxy.SOCKS5("tcp", s.Address(), &proxy.Auth{User: "old", Password: "bad"}, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	if c, err := dial.Dial("tcp", "example.com:80"); err == nil {
		_ = c.Close()
		t.Fatal("invalid credentials accepted")
	}
	finish, err := s.Apply(Plan{[]Route{{"new", "alice", 2, 2, nil}}})
	if err != nil {
		t.Fatal(err)
	}
	finish(false)
	c := tcp(t, s, "old")
	if _, err = c.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	received := make([]byte, 2)
	if _, err = io.ReadFull(c, received); err != nil || string(received) != "ok" {
		t.Fatalf("rollback did not restore old route: %q %v", received, err)
	}
}

func TestUDPAssociateLimitedBothWays(t *testing.T) {
	s := start(t, []Route{{"udp", "alice", 1, 1, nil}})
	c, err := net.Dial("tcp", s.Address())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	_, _ = c.Write([]byte{5, 1, 2})
	var two [2]byte
	if _, err = io.ReadFull(c, two[:]); err != nil {
		t.Fatal(err)
	}
	auth := append([]byte{1, 3}, []byte("udp")...)
	auth = append(auth, byte(len(s.Password())))
	auth = append(auth, s.Password()...)
	_, _ = c.Write(auth)
	if _, err = io.ReadFull(c, two[:]); err != nil || two != [2]byte{1, 0} {
		t.Fatalf("auth: %v %v", two, err)
	}
	_, _ = c.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0})
	reply := make([]byte, 10)
	if _, err = io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	u, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(reply[8])<<8 | int(reply[9])})
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	_ = u.SetDeadline(time.Now().Add(10 * time.Second))
	packet := append([]byte{0, 0, 0, 1, 1, 1, 1, 1, 0, 53}, bytes.Repeat([]byte{'u'}, 16384)...)
	started := time.Now()
	buf := make([]byte, 65536)
	for range 16 {
		if _, err = u.Write(packet); err != nil {
			t.Fatal(err)
		}
		n, err := u.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf[:n], packet) {
			t.Fatal("UDP payload or destination changed")
		}
	}
	if elapsed := time.Since(started); elapsed < 1400*time.Millisecond {
		t.Fatalf("UDP bypassed limiter: %v", elapsed)
	}
	s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for len(s.conns) > 0 {
			s.mu.Unlock()
			time.Sleep(time.Millisecond)
			s.mu.Lock()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("UDP control session leaked after close")
	}
}
