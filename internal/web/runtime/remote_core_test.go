package runtime

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestRemoteRestartXrayUsesCoreAwareEndpoint(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"msg":"","obj":null}`))
	}))
	defer server.Close()

	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	r := NewRemote(&model.Node{
		Id:                  1,
		Name:                "singbox-node",
		Address:             host,
		Port:                port,
		Scheme:              "http",
		BasePath:            "/",
		ApiToken:            "test-token",
		AllowPrivateAddress: true,
	}, nil)

	if err := r.RestartXray(context.Background()); err != nil {
		t.Fatalf("RestartXray() error = %v", err)
	}
	if gotPath != "/panel/api/server/restartCoreService" {
		t.Fatalf("path = %q, want %q", gotPath, "/panel/api/server/restartCoreService")
	}
}
