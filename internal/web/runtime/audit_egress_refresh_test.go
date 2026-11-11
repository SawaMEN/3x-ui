package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestAuditCachedRemoteObservesEgressResolverReplacement(t *testing.T) {
	manager := NewManager(LocalDeps{})
	remote, err := manager.RemoteFor(&model.Node{
		Id: 8101, Name: "egress-refresh", Address: "127.0.0.1", Port: 1,
		Scheme: "http", OutboundTag: "tunnel", AllowPrivateAddress: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.httpClient(); err == nil {
		t.Fatal("missing egress unexpectedly accepted")
	}
	manager.SetNodeEgressResolver(constantNodeEgressResolver("socks5://127.0.0.1:11080"))
	first, err := remote.httpClient()
	if err != nil {
		t.Fatalf("cached remote did not observe newly configured resolver: %v", err)
	}
	manager.SetNodeEgressResolver(nil)
	if _, err := remote.httpClient(); err == nil || !strings.Contains(err.Error(), "refusing direct connection") {
		t.Fatalf("cached remote retained cleared egress: %v", err)
	}
	manager.SetNodeEgressResolver(constantNodeEgressResolver("socks5://127.0.0.1:11081"))
	second, err := remote.httpClient()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || remote.clientProxyURL != "socks5://127.0.0.1:11081" {
		t.Fatal("cached remote kept previous egress transport after resolver replacement")
	}
}

func TestAuditRemoteEgressResolverMayReenterManager(t *testing.T) {
	manager := NewManager(LocalDeps{})
	manager.SetNodeEgressResolver(reentrantNodeEgressResolver{manager: manager})
	remote, err := manager.RemoteFor(&model.Node{
		Id: 7, Name: "reentrant", Address: "127.0.0.1", Port: 1,
		Scheme: "http", OutboundTag: "tunnel", AllowPrivateAddress: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := remote.httpClient()
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("remote egress callback deadlocked while replacing resolver")
	}
	if _, err := remote.httpClient(); err == nil {
		t.Fatal("remote retained resolver cleared by the callback")
	}
}
