package hiddify

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"golang.org/x/net/proxy"
)

// Read a SOCKS address without resolving it: the marker peers also prove that
// the core selected the intended proxy rather than sending data directly.
func stage7Address(r io.Reader) ([]byte, error) {
	var kind [1]byte
	if _, err := io.ReadFull(r, kind[:]); err != nil {
		return nil, err
	}
	n := 0
	switch kind[0] {
	case 1:
		n = 4
	case 4:
		n = 16
	case 3:
		var size [1]byte
		if _, err := io.ReadFull(r, size[:]); err != nil {
			return nil, err
		}
		kindBytes := append(kind[:], size[0])
		rest := make([]byte, int(size[0])+2)
		_, err := io.ReadFull(r, rest)
		return append(kindBytes, rest...), err
	default:
		return nil, fmt.Errorf("invalid address type %d", kind[0])
	}
	b := make([]byte, n+2)
	_, err := io.ReadFull(r, b)
	return append(kind[:], b...), err
}

func stage7Marker(t *testing.T, marker byte) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	u, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	clients := map[net.Conn]bool{}
	t.Cleanup(func() {
		_ = l.Close()
		_ = u.Close()
		mu.Lock()
		defer mu.Unlock()
		for c := range clients {
			_ = c.Close()
		}
	})
	go func() {
		buf := make([]byte, 65535)
		for {
			n, a, e := u.ReadFrom(buf)
			if e != nil {
				return
			}
			if n < 4 {
				continue
			}
			r := bytes.NewReader(buf[3:n])
			if _, e = stage7Address(r); e != nil {
				continue
			}
			header := n - r.Len()
			reply := append(append([]byte{}, buf[:header]...), marker)
			reply = append(reply, buf[header:n]...)
			_, _ = u.WriteTo(reply, a)
		}
	}()
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			mu.Lock()
			clients[c] = true
			mu.Unlock()
			go func() {
				defer func() { _ = c.Close(); mu.Lock(); delete(clients, c); mu.Unlock() }()
				_ = c.SetDeadline(time.Now().Add(15 * time.Second))
				var h [2]byte
				if _, e := io.ReadFull(c, h[:]); e != nil {
					return
				}
				methods := make([]byte, int(h[1]))
				if _, e := io.ReadFull(c, methods); e != nil {
					return
				}
				_, _ = c.Write([]byte{5, 0})
				var cmd [3]byte
				if _, e := io.ReadFull(c, cmd[:]); e != nil {
					return
				}
				if _, e := stage7Address(c); e != nil {
					return
				}
				p := 0
				if cmd[1] == 3 {
					p = port
				}
				_, _ = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, byte(p >> 8), byte(p)})
				if cmd[1] == 3 {
					_, _ = io.Copy(io.Discard, c)
					return
				}
				data := make([]byte, len("route-payload"))
				if _, e := io.ReadFull(c, data); e == nil {
					_, _ = c.Write(append([]byte{marker}, data...))
				}
			}()
		}
	}()
	return port
}

func stage7Exchange(address, host string, port int, network string) ([]byte, error) {
	payload := []byte("route-payload")
	if network == "tcp" {
		d, err := proxy.SOCKS5("tcp", address, nil, &net.Dialer{Timeout: time.Second})
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		c, err := d.(proxy.ContextDialer).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			return nil, err
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err = c.Write(payload); err != nil {
			return nil, err
		}
		reply := make([]byte, len(payload)+1)
		n, readErr := io.ReadFull(c, reply)
		return reply[:n], readErr
	}
	c, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = c.Write([]byte{5, 1, 0})
	var h [2]byte
	if _, err = io.ReadFull(c, h[:]); err != nil {
		return nil, err
	}
	_, _ = c.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0})
	var prefix [3]byte
	if _, err = io.ReadFull(c, prefix[:]); err != nil {
		return nil, err
	}
	if prefix[1] != 0 {
		return nil, fmt.Errorf("UDP denied")
	}
	bind, err := stage7Address(c)
	if err != nil {
		return nil, err
	}
	if bind[0] != 1 {
		return nil, fmt.Errorf("unexpected UDP bind %v", bind)
	}
	relay := net.JoinHostPort(net.IP(bind[1:5]).String(), strconv.Itoa(int(binary.BigEndian.Uint16(bind[5:]))))
	u, err := net.Dial("udp", relay)
	if err != nil {
		return nil, err
	}
	defer u.Close()
	_ = u.SetDeadline(time.Now().Add(2 * time.Second))
	packet := []byte{0, 0, 0, 3, byte(len(host))}
	packet = append(packet, host...)
	packet = append(packet, byte(port>>8), byte(port))
	packet = append(packet, payload...)
	if _, err = u.Write(packet); err != nil {
		return nil, err
	}
	buf := make([]byte, 2048)
	n, err := u.Read(buf)
	if err != nil {
		return nil, err
	}
	if n < 4 {
		return nil, fmt.Errorf("short UDP response")
	}
	r := bytes.NewReader(buf[3:n])
	if _, err = stage7Address(r); err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

func TestStage7RoutingTraffic(t *testing.T) {
	root := os.Getenv("XUI_STAGE4_CORE_DIR")
	if root == "" {
		t.Skip("requires pinned routing engines")
	}
	for _, engine := range []string{"owner", "upstream", "xray"} {
		t.Run(engine, func(t *testing.T) {
			binary := filepath.Join(root, "owner", "x-ui", "bin", "hiddify-core-linux-"+runtime.GOARCH)
			if engine == "upstream" {
				binary = filepath.Join(root, "upstream", "sing-box")
			}
			if engine == "xray" {
				binary = filepath.Join(root, "xray", "xray")
			}
			t.Setenv("STAGE5_NATIVE_PROFILE", engine)
			a, b := stage7Marker(t, 'A'), stage7Marker(t, 'B')
			port, other := stage5Port(t), stage5Port(t)
			outs := []any{map[string]any{"protocol": "blackhole", "tag": "blocked"}}
			for i, p := range []int{a, b} {
				outs = append(outs, map[string]any{"protocol": "socks", "tag": string(rune('A' + i)), "settings": map[string]any{"servers": []any{map[string]any{"address": "127.0.0.1", "port": p}}}})
			}
			rules := []any{
				map[string]any{"type": "field", "inboundTag": []any{"test"}, "domain": []any{"full:one.invalid"}, "port": "443", "outboundTag": "A"},
				map[string]any{"type": "field", "inboundTag": []any{"test"}, "domain": []any{"full:one.invalid", "full:two.invalid"}, "outboundTag": "B"},
				map[string]any{"type": "field", "network": "tcp,udp", "outboundTag": "blocked"},
			}
			cfg := map[string]any{"log": map[string]any{"loglevel": "warning"}, "outbounds": outs, "routing": map[string]any{"domainStrategy": "AsIs", "rules": rules}, "dns": map[string]any{"hosts": map[string]any{"one.invalid": "198.18.0.9", "two.invalid": "198.18.0.10"}}}
			var ins []any
			for i, p := range []int{port, other} {
				tag := "test"
				if i == 1 {
					tag = "other"
				}
				ins = append(ins, map[string]any{"protocol": "socks", "tag": tag, "listen": "127.0.0.1", "port": p, "settings": map[string]any{"auth": "noauth", "udp": true}})
			}
			cfg["inbounds"] = ins
			if engine != "xray" {
				nativeOuts := []any{}
				for _, raw := range outs {
					ob, err := TranslateOutbound(raw.(map[string]any))
					if engine == "upstream" {
						ob, err = singbox.TranslateXrayOutbound(raw.(map[string]any))
					}
					if err != nil {
						t.Fatal(err)
					}
					nativeOuts = append(nativeOuts, ob)
				}
				nativeIns := []any{}
				for _, raw := range ins {
					ib, err := TranslateInbound(raw.(map[string]any))
					if err != nil {
						t.Fatal(err)
					}
					nativeIns = append(nativeIns, ib)
				}
				route, err := singbox.TranslateXrayRouting(cfg["routing"].(map[string]any))
				if err != nil {
					t.Fatal(err)
				}
				route["final"] = "blocked"
				route["default_domain_resolver"] = "hosts"
				cfg = map[string]any{"inbounds": nativeIns, "outbounds": nativeOuts, "route": route, "dns": map[string]any{"servers": []any{map[string]any{"type": "hosts", "tag": "hosts", "predefined": map[string]any{"one.invalid": []string{"198.18.0.9"}, "two.invalid": []string{"198.18.0.10"}}}}, "final": "hosts"}}
			}
			stage5Start(t, binary, t.TempDir(), "routing", cfg, port, true)
			for _, network := range []string{"tcp", "udp"} {
				for _, tc := range []struct {
					name, host string
					port, in   int
					want       byte
				}{{"first-match", "one.invalid", 443, port, 'A'}, {"AND-port", "one.invalid", 444, port, 'B'}, {"OR-domain", "two.invalid", 445, port, 'B'}, {"unmatched-fail-closed", "unmatched.invalid", 443, port, 0}, {"inbound-scope", "one.invalid", 443, other, 0}} {
					t.Run(network+"/"+tc.name, func(t *testing.T) {
						reply, err := stage7Exchange(net.JoinHostPort("127.0.0.1", strconv.Itoa(tc.in)), tc.host, tc.port, network)
						if tc.want == 0 {
							if err == nil || len(reply) > 0 {
								t.Fatalf("policy leaked to %q", reply)
							}
							return
						}
						if err != nil || !bytes.Equal(reply, append([]byte{tc.want}, []byte("route-payload")...)) {
							t.Fatalf("route marker: %q error=%v", reply, err)
						}
					})
				}
			}
		})
	}
}
