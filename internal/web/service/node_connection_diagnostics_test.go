package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestFriendlyProbeErrorDistinguishesConnectionAndBodyEOF(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"TLS or pre-response EOF", `Get "https://node.test:24760/badoo/panel/api/server/status": EOF`, "remote closed the connection before an HTTP response"},
		{"empty HTTP body", "decode response: EOF", "empty HTTP 200 response instead of JSON"},
		{"HTTP served over HTTPS", "http: server gave HTTP response to HTTPS client", "set the node scheme to http"},
		{"auth error unchanged", "HTTP 401 from remote panel", "HTTP 401 from remote panel"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FriendlyProbeError(tc.in); !strings.Contains(got, tc.want) {
				t.Fatalf("FriendlyProbeError(%q) = %q, want substring %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestProbePreservesDecodePhaseInReturnedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // no JSON body
	}))
	defer server.Close()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	node := &model.Node{
		Address: host, Port: port, Scheme: "http", BasePath: "/",
		AllowPrivateAddress: true,
	}
	patch, err := (&NodeService{}).probe(context.Background(), node, "")
	if err == nil || !strings.Contains(err.Error(), "decode response: EOF") {
		t.Fatalf("probe returned %v, want decode response: EOF", err)
	}
	if got := patch.ToUI(false).Error; !strings.Contains(got, "empty HTTP 200 response") {
		t.Fatalf("probe UI error %q does not explain an empty status body", got)
	}
}
