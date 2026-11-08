package mieru

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/enfein/mieru/v3/pkg/appctl/appctlpb"
	"github.com/enfein/mieru/v3/pkg/egress"
	"github.com/enfein/mieru/v3/pkg/socks5"
	"google.golang.org/protobuf/encoding/protojson"
)

// Exercise the actual Mita rule evaluator with rendered production settings:
// both commands and all supported destination address forms must use the core.
func TestMieruEgressRoutesTCPAndUDPToPanel(t *testing.T) {
	raw, err := json.Marshal(renderConfig(Instance{Id: 42, MTU: 1400, LoggingLevel: "INFO", PortBindings: []PortBinding{{Port: 8443, Protocol: "TCP"}}, Users: []User{{Name: "alice", Password: "secret"}}}))
	if err != nil {
		t.Fatal(err)
	}
	var cfg appctlpb.ServerConfig
	if err := protojson.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	server, err := socks5.New(&socks5.Config{Egress: cfg.Egress})
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []byte{1, 3} {
		for _, address := range [][]byte{{1, 8, 8, 8, 8}, {4, 0x26, 0x06, 0x47, 0, 0x47, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x11, 0x11}, {3, 11, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm'}} {
			req := append([]byte{5, command, 0}, address...)
			req = append(req, 0, 53)
			action := server.FindAction(context.Background(), egress.Input{Protocol: appctlpb.ProxyProtocol_SOCKS5_PROXY_PROTOCOL, Data: req})
			if action.Action != appctlpb.EgressAction_PROXY || action.Proxy == nil {
				t.Fatalf("command %d address %v bypasses panel: %v", command, address, action)
			}
			if action.Proxy.GetHost() != "127.0.0.1" || int(action.Proxy.GetPort()) != SOCKSPortForInbound(42) || action.Proxy.GetSocks5Authentication().GetPassword() != SocksPassword() {
				t.Fatal("wrong bridge destination or authentication")
			}
		}
	}
}
