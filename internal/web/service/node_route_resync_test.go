package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// A matching master-side fingerprint says nothing about a credential removed
// directly on the destination. Explicit cascade preparation must repair it.
func TestNodeRouteExplicitSyncRepairsUnchangedDestination(t *testing.T) {
	setupBulkDB(t)
	manager := useTestRuntimeManager(t)
	pushes, restarts := 0, 0
	remoteSettings := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/panel/api/inbounds/list":
			_, _ = w.Write([]byte(`{"success":true,"obj":[{"id":7,"tag":"foreign"}]}`))
		case "/panel/api/inbounds/update/7":
			pushes++
			remoteSettings = r.FormValue("settings")
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/panel/api/server/restartCoreService":
			restarts++
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			t.Errorf("unexpected RPC: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	host, portText, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	node := &model.Node{Id: 123, Address: host, Port: port, Scheme: "http", BasePath: "/", ApiToken: "test-token", AllowPrivateAddress: true}
	inbound := &model.Inbound{Tag: "n123-foreign", Protocol: model.VLESS, Enable: true, Port: 443,
		Settings:       `{"clients":[{"email":"node-route@internal","id":"11111111-1111-4111-8111-111111111111","enable":true}],"decryption":"none"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`, NodeID: &node.Id}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	remote, err := manager.RemoteFor(node)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.ReconcileInbound(context.Background(), inbound, false); err != nil {
		t.Fatal(err)
	}
	if pushes != 1 {
		t.Fatalf("initial pushes = %d", pushes)
	}
	// Model a remote restore/edit that retains the tag but loses the service user.
	// The unchanged master snapshot still matches the cached fingerprint.
	remoteSettings = `{"clients":[]}`
	if err := syncNodeRouteInbound(inbound, node); err != nil {
		t.Fatal(err)
	}
	if pushes != 2 || restarts != 1 {
		t.Fatalf("explicit repair skipped destination credentials: pushes=%d restarts=%d", pushes, restarts)
	}
	if !strings.Contains(remoteSettings, "node-route@internal") {
		t.Fatal("service credential was not restored on destination")
	}
}
