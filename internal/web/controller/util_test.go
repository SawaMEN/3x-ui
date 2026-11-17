package controller

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetRemoteIpIgnoresForwardedHeadersFromUntrustedRemote(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.RemoteAddr = "203.0.113.10:12345"
	c.Request.Header.Set("X-Real-IP", "198.51.100.9")
	c.Request.Header.Set("X-Forwarded-For", "198.51.100.8")

	if got := getRemoteIp(c); got != "203.0.113.10" {
		t.Fatalf("remote IP = %q, want request remote address", got)
	}
}

func TestGetRemoteIpHonorsForwardedHeadersFromTrustedLoopbackProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.RemoteAddr = "127.0.0.1:12345"
	c.Request.Header.Set("X-Forwarded-For", "198.51.100.8, 127.0.0.1")

	if got := getRemoteIp(c); got != "198.51.100.8" {
		t.Fatalf("remote IP = %q, want forwarded client IP", got)
	}
}

// Caddy and HAProxy set X-Forwarded-For but pass the client's own X-Real-IP
// through, so a client could pick its login-limiter bucket per request.
func TestGetRemoteIpDistrustsRealIPThatContradictsForwardedFor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.RemoteAddr = "127.0.0.1:12345"
	c.Request.Header.Set("X-Forwarded-For", "203.0.113.5")
	c.Request.Header.Set("X-Real-IP", "198.51.100.1")

	if got := getRemoteIp(c); got != "127.0.0.1" {
		t.Fatalf("remote IP = %q, want the proxy address when the forwarded headers disagree", got)
	}
}

// An appending proxy keeps whatever X-Forwarded-For the client sent in front of
// the address it saw, so only the hop next to the trusted proxies is real.
func TestGetRemoteIpReadsForwardedForFromTheProxyEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.RemoteAddr = "127.0.0.1:12345"
	c.Request.Header.Set("X-Forwarded-For", "198.51.100.1, 203.0.113.5")

	if got := getRemoteIp(c); got != "203.0.113.5" {
		t.Fatalf("remote IP = %q, want the hop the trusted proxy appended", got)
	}
}

func TestPublicHostFromRequestOnlyTrustsConfiguredProxies(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		want       string
	}{
		{name: "untrusted forwarded host", remoteAddr: "203.0.113.10:3000", forwarded: "attacker.example", want: "panel.example.com"},
		{name: "trusted forwarded host", remoteAddr: "127.0.0.1:3000", forwarded: "sub.example.net:443", want: "sub.example.net"},
		{name: "trusted forwarded list", remoteAddr: "127.0.0.1:3000", forwarded: "sub.example.net:443, other.example", want: "sub.example.net"},
		{name: "untrusted forwarded list", remoteAddr: "203.0.113.10:3000", forwarded: "attacker.example, sub.example.net", want: "panel.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testHostRequest(tt.remoteAddr, tt.forwarded, "")
			if got := publicHostFromRequest(c); got != tt.want {
				t.Fatalf("publicHostFromRequest() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestResolveHostNeverUsesClientRealIPAsServer(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		realIP     string
		want       string
	}{
		{name: "trusted proxy only real IP", remoteAddr: "127.0.0.1:3000", realIP: "198.51.100.7", want: "panel.example.com"},
		{name: "untrusted spoofed host", remoteAddr: "203.0.113.10:3000", forwarded: "attacker.example", want: "panel.example.com"},
		{name: "trusted forwarded host", remoteAddr: "127.0.0.1:3000", forwarded: "sub.example.net:443", realIP: "198.51.100.7", want: "sub.example.net"},
		{name: "trusted bracketed IPv6 host", remoteAddr: "127.0.0.1:3000", forwarded: "[2001:db8::1]", want: "2001:db8::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testHostRequest(tt.remoteAddr, tt.forwarded, tt.realIP)
			if got := resolveHost(c); got != tt.want {
				t.Fatalf("resolveHost() = %q; want %q", got, tt.want)
			}
		})
	}
}

func testHostRequest(remoteAddr, forwardedHost, realIP string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "http://panel.example.com:2096/", nil)
	c.Request.RemoteAddr = remoteAddr
	if forwardedHost != "" {
		c.Request.Header.Set("X-Forwarded-Host", forwardedHost)
	}
	if realIP != "" {
		c.Request.Header.Set("X-Real-IP", realIP)
	}
	return c
}

func TestProxyAddrInCIDRsIPv4Mapped(t *testing.T) {
	tests := []struct {
		name  string
		addr  string
		cidrs string
		want  bool
	}{
		{name: "native IPv4 in mapped prefix", addr: "10.1.2.3", cidrs: "::ffff:10.0.0.0/104", want: true},
		{name: "mapped IPv4 in mapped prefix", addr: "::ffff:10.1.2.3", cidrs: "::ffff:10.0.0.0/104", want: true},
		{name: "outside mapped prefix", addr: "203.0.113.10", cidrs: "::ffff:10.0.0.0/104", want: false},
		{name: "ordinary IPv4 CIDR", addr: "10.1.2.3", cidrs: "10.0.0.0/8", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := proxyAddrInCIDRs(netip.MustParseAddr(tt.addr), tt.cidrs); got != tt.want {
				t.Fatalf("proxyAddrInCIDRs(%q, %q) = %v; want %v", tt.addr, tt.cidrs, got, tt.want)
			}
		})
	}
}
