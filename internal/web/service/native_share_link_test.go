package service

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/enfein/mieru/v3/pkg/appctl"
	pb "github.com/enfein/mieru/v3/pkg/appctl/appctlpb"
	"google.golang.org/protobuf/proto"
)

func TestNativeMieruShareLinks(t *testing.T) {
	profile := &pb.ClientProfile{ProfileName: proto.String("test"), User: &pb.User{Name: proto.String("user"), Password: proto.String("p:@ss")}, Servers: []*pb.ServerEndpoint{{DomainName: proto.String("proxy.test"), PortBindings: []*pb.PortBinding{{Protocol: pb.TransportProtocol_TCP.Enum(), PortRange: proto.String("2012-2022")}, {Protocol: pb.TransportProtocol_UDP.Enum(), Port: proto.Int32(2023)}}}}, Mtu: proto.Int32(1400), Multiplexing: &pb.MultiplexingConfig{Level: pb.MultiplexingLevel_MULTIPLEXING_HIGH.Enum()}, HandshakeMode: pb.HandshakeMode_HANDSHAKE_NO_WAIT.Enum()}
	native, err := appctl.ClientConfigToURL(&pb.ClientConfig{Profiles: []*pb.ClientProfile{profile}})
	if err != nil {
		t.Fatal(err)
	}
	simple, err := appctl.ClientProfileToMultiURLs(profile)
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]any
	for _, link := range append(simple, native) {
		out, err := ParseNativeMieruShareLink(link)
		if err != nil {
			t.Fatal(err)
		}
		if out["password"] != "p:@ss" || out["handshake_mode"] != "NO_WAIT" || out["multiplexing"] != "HIGH" {
			t.Fatalf("lost settings: %v", out)
		}
		if _, ok := out["server_port"]; ok {
			t.Fatal("range must not have a conflicting server_port")
		}
		if expected != nil && !reflect.DeepEqual(expected, out) {
			t.Fatalf("different representations: %v / %v", expected, out)
		}
		expected = out
	}
	if binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY"); binary != "" {
		data, _ := json.Marshal(map[string]any{"outbounds": []any{expected}})
		file := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command(binary, "check", "-c", file).CombinedOutput(); err != nil {
			t.Fatalf("pinned core rejected Mieru: %v %s", err, output)
		}
	}
	for _, mutate := range []func(*pb.ClientProfile){
		func(p *pb.ClientProfile) { p.Servers = append(p.Servers, p.Servers[0]) },
		func(p *pb.ClientProfile) { p.User.Password = nil; p.User.HashedPassword = proto.String("hash") },
		func(p *pb.ClientProfile) { p.Multiplexing.Level = pb.MultiplexingLevel_MULTIPLEXING_OFF.Enum() },
		func(p *pb.ClientProfile) { p.Servers[0].PortBindings[0].PortRange = proto.String("65536-70000") },
		func(p *pb.ClientProfile) { p.TrafficPattern = &pb.TrafficPattern{} },
		func(p *pb.ClientProfile) { p.Dialer = &pb.ClientDialer{} },
	} {
		p := proto.Clone(profile).(*pb.ClientProfile)
		mutate(p)
		link, _ := appctl.ClientConfigToURL(&pb.ClientConfig{Profiles: []*pb.ClientProfile{p}})
		if _, err := ParseNativeMieruShareLink(link); err == nil {
			t.Fatal("accepted unsupported profile")
		}
	}
	for _, link := range []string{"", "hy2://pass@proxy.test", "mieru://broken", "mierus://u:p@host?profile=x&port=0&protocol=TCP"} {
		if _, err := ParseNativeMieruShareLink(link); err == nil {
			t.Fatalf("accepted %q", link)
		}
	}
}
