package mieru

import (
	"encoding/json"
	"testing"

	"github.com/enfein/mieru/v3/pkg/appctl"
	pb "github.com/enfein/mieru/v3/pkg/appctl/appctlpb"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

func TestServerEgressContractUsesPanelForEveryDestination(t *testing.T) {
	inst := Instance{Id: 42, Users: []User{{Name: "alice", Password: "secret"}}, PortBindings: []PortBinding{{Port: 2443, Protocol: "TCP"}}, LoggingLevel: "INFO", MTU: 1400}
	data, err := json.Marshal(renderConfig(inst))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &pb.ServerConfig{}
	if err := protojson.Unmarshal(data, cfg); err != nil {
		t.Fatal(err)
	}
	if err := appctl.ValidateServerConfigPatch(cfg); err != nil {
		t.Fatal(err)
	}
	proxy := cfg.GetEgress().GetProxies()[0]
	rule := cfg.GetEgress().GetRules()[0]
	if proxy.GetPort() != int32(sidecaregress.Port(42)) || proxy.GetHost() != "127.0.0.1" || rule.GetAction() != pb.EgressAction_PROXY || rule.GetIpRanges()[0] != "*" || rule.GetDomainNames()[0] != "*" {
		t.Fatal("incomplete egress policy")
	}
}
