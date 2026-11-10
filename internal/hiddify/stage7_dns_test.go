package hiddify

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
)

type stage7DNSLog struct {
	mu   sync.Mutex
	hits map[string]int
}

func (l *stage7DNSLog) count(name string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.hits[dns.Fqdn(name)]
}

func (l *stage7DNSLog) answer(req *dns.Msg, marker int) *dns.Msg {
	l.mu.Lock()
	for _, q := range req.Question {
		l.hits[q.Name]++
	}
	l.mu.Unlock()
	out := new(dns.Msg)
	out.SetReply(req)
	if len(req.Question) != 1 || marker == 0 || strings.HasPrefix(req.Question[0].Name, "missing.") {
		out.Rcode = dns.RcodeNameError
		return out
	}
	q := req.Question[0]
	header := dns.RR_Header{Name: q.Name, Rrtype: q.Qtype, Class: dns.ClassINET, Ttl: 60}
	switch q.Qtype {
	case dns.TypeA:
		out.Answer = []dns.RR{&dns.A{Hdr: header, A: net.ParseIP(fmt.Sprintf("192.0.2.%d", marker))}}
	case dns.TypeAAAA:
		out.Answer = []dns.RR{&dns.AAAA{Hdr: header, AAAA: net.ParseIP(fmt.Sprintf("2001:db8::%d", marker))}}
	}
	return out
}

func stage7DNSUDP(t *testing.T, log *stage7DNSLog, port, marker int) int {
	t.Helper()
	pc, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	s := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, m *dns.Msg) { _ = w.WriteMsg(log.answer(m, marker)) })}
	go func() { _ = s.ActivateAndServe() }()
	t.Cleanup(func() { _ = s.Shutdown() })
	return pc.LocalAddr().(*net.UDPAddr).Port
}

func TestStage7DNSPolicy(t *testing.T) {
	root := os.Getenv("XUI_STAGE4_CORE_DIR")
	if root == "" {
		t.Skip("requires pinned DNS engines")
	}
	leaks := &stage7DNSLog{hits: map[string]int{}}
	if os.Getenv("STAGE7_ISOLATED") == "1" {
		stage7DNSUDP(t, leaks, 53, 0)
	}
	for _, engine := range []string{"owner", "upstream"} {
		t.Run(engine, func(t *testing.T) {
			private, secure, public := &stage7DNSLog{hits: map[string]int{}}, &stage7DNSLog{hits: map[string]int{}}, &stage7DNSLog{hits: map[string]int{}}
			udpPort := stage7DNSUDP(t, private, 0, 11)
			https := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var b []byte
				var err error
				if r.Method == http.MethodGet {
					b, err = base64.RawURLEncoding.DecodeString(r.URL.Query().Get("dns"))
				} else {
					b, err = io.ReadAll(io.LimitReader(r.Body, 4097))
				}
				m := new(dns.Msg)
				if err != nil || len(b) > 4096 || m.Unpack(b) != nil {
					http.Error(w, "invalid DNS request", http.StatusBadRequest)
					return
				}
				reply, err := public.answer(m, 33).Pack()
				if err != nil {
					http.Error(w, "DNS response", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/dns-message")
				_, _ = w.Write(reply)
			}))
			defer https.Close()
			cert := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: https.Certificate().Raw}))
			tlsConfig := map[string]any{"enabled": true, "server_name": "example.com", "certificate": []string{cert}}
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			dot := &dns.Server{Listener: tls.NewListener(ln, &tls.Config{Certificates: https.TLS.Certificates, MinVersion: tls.VersionTLS12}), Handler: dns.HandlerFunc(func(w dns.ResponseWriter, m *dns.Msg) { _ = w.WriteMsg(secure.answer(m, 22)) })}
			go func() { _ = dot.ActivateAndServe() }()
			defer dot.Shutdown()
			_, httpsPort, _ := net.SplitHostPort(https.Listener.Addr().String())
			dohPort, _ := strconv.Atoi(httpsPort)
			port := stage5Port(t)
			cfg := map[string]any{
				"inbounds":  []any{map[string]any{"type": "direct", "tag": "dns-in", "listen": "127.0.0.1", "listen_port": port, "override_address": "8.8.8.8", "override_port": 53}},
				"outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}},
				"route":     map[string]any{"default_domain_resolver": "public", "rules": []any{map[string]any{"inbound": []string{"dns-in"}, "action": "hijack-dns"}}},
				"dns": map[string]any{"servers": []any{
					map[string]any{"type": "udp", "tag": "private", "server": "127.0.0.1", "server_port": udpPort},
					map[string]any{"type": "tls", "tag": "secure", "server": "127.0.0.1", "server_port": ln.Addr().(*net.TCPAddr).Port, "tls": tlsConfig},
					map[string]any{"type": "https", "tag": "public", "server": "127.0.0.1", "server_port": dohPort, "path": "/dns-query", "tls": tlsConfig},
					map[string]any{"type": "hosts", "tag": "hosts", "predefined": map[string]any{"hosts.test": []string{"203.0.113.77", "2001:db8::77"}}},
					map[string]any{"type": "fakeip", "tag": "fake", "inet4_range": "198.18.0.0/15", "inet6_range": "fc00::/18"},
				}, "rules": []any{
					map[string]any{"domain_suffix": []string{"private.test"}, "action": "route", "server": "private"},
					map[string]any{"domain_suffix": []string{"secure.test"}, "action": "route", "server": "secure"},
					map[string]any{"domain": []string{"hosts.test"}, "action": "route", "server": "hosts"},
					map[string]any{"domain_suffix": []string{"fake.test"}, "action": "route", "server": "fake"},
				}, "final": "public", "independent_cache": true},
			}
			binary := filepath.Join(root, "owner", "x-ui", "bin", "hiddify-core-linux-"+runtime.GOARCH)
			if engine == "upstream" {
				binary = filepath.Join(root, "upstream", "sing-box")
			}
			t.Setenv("STAGE5_NATIVE_PROFILE", engine)
			stage5Start(t, binary, t.TempDir(), "dns-policy", cfg, port, true)
			query := func(t *testing.T, name string, qtype uint16, network string) *dns.Msg {
				t.Helper()
				m := new(dns.Msg)
				m.SetQuestion(dns.Fqdn(name), qtype)
				reply, _, err := (&dns.Client{Net: network, Timeout: 3 * time.Second}).Exchange(m, fmt.Sprintf("127.0.0.1:%d", port))
				if err != nil {
					t.Fatal(err)
				}
				return reply
			}
			for _, network := range []string{"udp", "tcp"} {
				for _, tc := range []struct{ name, ip4, ip6 string }{{"private.test", "192.0.2.11", "2001:db8::11"}, {"secure.test", "192.0.2.22", "2001:db8::22"}, {"public.test", "192.0.2.33", "2001:db8::33"}, {"hosts.test", "203.0.113.77", "2001:db8::77"}} {
					for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
						t.Run(network+"/"+tc.name+"/"+dns.TypeToString[qtype], func(t *testing.T) {
							reply := query(t, tc.name, qtype, network)
							want := tc.ip4
							if qtype == dns.TypeAAAA {
								want = tc.ip6
							}
							if reply.Rcode != dns.RcodeSuccess || len(reply.Answer) != 1 || !strings.Contains(reply.Answer[0].String(), want) {
								t.Fatalf("wrong split DNS response: %v", reply)
							}
						})
					}
				}
			}
			for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
				reply := query(t, "cache.fake.test", qtype, "udp")
				if len(reply.Answer) != 1 {
					t.Fatal("missing fake IP", reply)
				}
				ip := net.ParseIP(strings.Fields(reply.Answer[0].String())[4])
				prefix := "198.18.0.0/15"
				if qtype == dns.TypeAAAA {
					prefix = "fc00::/18"
				}
				_, cidr, _ := net.ParseCIDR(prefix)
				if ip == nil || !cidr.Contains(ip) {
					t.Fatal("fakeIP escaped configured range", reply)
				}
			}
			for range 3 {
				query(t, "public.test", dns.TypeA, "udp")
			}
			if public.count("public.test") != 2 {
				t.Fatalf("A/AAAA cache did not suppress duplicate upstream requests: %d", public.count("public.test"))
			}
			if r := query(t, "missing.public.test", dns.TypeA, "udp"); r.Rcode != dns.RcodeNameError {
				t.Fatal("NXDOMAIN was replaced by another upstream", r)
			}
			for _, name := range []string{"private.test", "secure.test", "hosts.test", "cache.fake.test", "public.test", "missing.public.test"} {
				if leaks.count(name) > 0 {
					t.Fatal("test DNS query leaked to system resolver", name)
				}
				for _, peer := range []struct {
					log     *stage7DNSLog
					allowed string
				}{{private, "private.test"}, {secure, "secure.test"}, {public, "public.test"}} {
					if name != peer.allowed && (peer.allowed != "public.test" || name != "missing.public.test") && peer.log.count(name) > 0 {
						t.Fatal("query leaked across split DNS policy", name, peer.allowed)
					}
				}
			}
			t.Log("captured A/AAAA over UDP/TCP, DoH/DoT upstreams, cache, hosts, fakeIP and NXDOMAIN; no test-domain query reached an unselected resolver")
		})
	}
}
