package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestNodeCertProbeDoesNotSendHTTPToUntrustedPeer(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	host, portString, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(portString)
	node := &model.Node{Address: host, Port: port, Scheme: "https", AllowPrivateAddress: true}
	got, err := (&NodeService{}).FetchCertFingerprint(context.Background(), node)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(server.Certificate().Raw)
	if got != base64.StdEncoding.EncodeToString(sum[:]) {
		t.Fatalf("incorrect peer fingerprint: %q", got)
	}
	if requests.Load() != 0 {
		t.Fatal("an untrusted peer received an HTTP request")
	}
	node.AllowPrivateAddress = false
	if _, err := (&NodeService{}).FetchCertFingerprint(context.Background(), node); err == nil {
		t.Fatal("certificate probe bypassed the private-address guard")
	}
}
