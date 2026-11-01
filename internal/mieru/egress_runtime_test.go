package mieru

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	client "github.com/enfein/mieru/v3/apis/client"
	common "github.com/enfein/mieru/v3/apis/common"
	model "github.com/enfein/mieru/v3/apis/model"
	"github.com/enfein/mieru/v3/pkg/appctl"
	pb "github.com/enfein/mieru/v3/pkg/appctl/appctlpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

func TestMieruPayloadAcrossTCPAndUDP(t *testing.T) {
	// Echo-only SOCKS fixture never contacts the public destination. Successful
	// responses therefore demonstrate egress forwarding rather than direct access.
	socks, err := net.Listen("tcp", sidecaregress.Address(999))
	if err != nil {
		t.Fatal(err)
	}
	defer socks.Close()
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	commands := make(chan byte, 8)
	go func() {
		b := make([]byte, 65535)
		for {
			n, addr, e := udp.ReadFromUDP(b)
			if e != nil {
				return
			}
			udp.WriteToUDP(b[:n], addr)
		}
	}()
	go func() {
		for {
			c, e := socks.Accept()
			if e != nil {
				return
			}
			go func() {
				defer c.Close()
				c.SetDeadline(time.Now().Add(15 * time.Second))
				var hello [2]byte
				if _, e := io.ReadFull(c, hello[:]); e != nil {
					return
				}
				io.CopyN(io.Discard, c, int64(hello[1]))
				c.Write([]byte{5, 2})
				var auth [2]byte
				if _, e := io.ReadFull(c, auth[:]); e != nil {
					return
				}
				user := make([]byte, int(auth[1]))
				io.ReadFull(c, user)
				var plen [1]byte
				io.ReadFull(c, plen[:])
				pass := make([]byte, int(plen[0]))
				io.ReadFull(c, pass)
				if string(user) != SocksUsername() || string(pass) != SocksPassword() {
					c.Write([]byte{1, 1})
					return
				}
				c.Write([]byte{1, 0})
				var header [4]byte
				if _, e := io.ReadFull(c, header[:]); e != nil {
					return
				}
				var n int
				switch header[3] {
				case 1:
					n = 6
				case 4:
					n = 18
				case 3:
					var length [1]byte
					io.ReadFull(c, length[:])
					n = int(length[0]) + 2
				default:
					return
				}
				if _, e := io.CopyN(io.Discard, c, int64(n)); e != nil {
					return
				}
				commands <- header[1]
				reply := []byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}
				binary.BigEndian.PutUint16(reply[8:], uint16(udp.LocalAddr().(*net.UDPAddr).Port))
				c.Write(reply)
				if header[1] == 1 {
					io.Copy(c, c)
				} else {
					io.Copy(io.Discard, c)
				}
			}()
		}
	}()
	for _, transport := range []string{"TCP", "UDP"} {
		t.Run(transport, func(t *testing.T) {
			free, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := free.Addr().(*net.TCPAddr).Port
			free.Close()
			inst := Instance{Id: 999, Tag: "test", Listen: "127.0.0.1", PortBindings: []PortBinding{{Port: port, Protocol: transport}}, Users: []User{{Name: "alice", Password: "secret"}}, MTU: 1400, LoggingLevel: "ERROR"}
			data, _ := json.Marshal(renderConfig(inst))
			serverConfig := &pb.ServerConfig{}
			if err := protojson.Unmarshal(data, serverConfig); err != nil {
				t.Fatal(err)
			}
			if err := appctl.StartServerProxy(serverConfig); err != nil {
				t.Fatal(err)
			}
			defer appctl.NewServerManagementService().Stop(context.Background(), &emptypb.Empty{})

			protocol := pb.TransportProtocol_TCP
			if transport == "UDP" {
				protocol = pb.TransportProtocol_UDP
			}
			c := client.NewClient()
			if err := c.Store(&client.ClientConfig{Profile: &pb.ClientProfile{ProfileName: proto.String("test"), User: &pb.User{Name: proto.String("alice"), Password: proto.String("secret")}, Servers: []*pb.ServerEndpoint{{IpAddress: proto.String("127.0.0.1"), PortBindings: []*pb.PortBinding{{Port: proto.Int32(int32(port)), Protocol: protocol.Enum()}}}}}}); err != nil {
				t.Fatal(err)
			}
			if err := c.Start(); err != nil {
				t.Fatal(err)
			}
			defer c.Stop()
			for _, network := range []string{"tcp", "udp"} {
				t.Run(network, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					target := model.NetAddrSpec{Net: network, AddrSpec: model.AddrSpec{IP: net.IPv4(8, 8, 8, 8), Port: 53}}
					var conn net.Conn
					var err error
					deadline := time.Now().Add(8 * time.Second)
					for {
						conn, err = c.DialContext(ctx, target)
						if err == nil || time.Now().After(deadline) {
							break
						}
						time.Sleep(50 * time.Millisecond)
					}
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					conn.SetDeadline(time.Now().Add(5 * time.Second))
					message := []byte("routed-payload")
					if network == "tcp" {
						conn.Write(message)
						b := make([]byte, len(message))
						if _, err := io.ReadFull(conn, b); err != nil || string(b) != string(message) {
							t.Fatalf("TCP payload: %q %v", b, err)
						}
					} else {
						wrapper := &common.UDPAssociateWrapper{PacketConn: common.NewPacketOverStreamTunnel(conn)}
						if _, err := wrapper.WriteTo(message, &net.UDPAddr{IP: net.IPv4(8, 8, 8, 8), Port: 53}); err != nil {
							t.Fatal(err)
						}
						b := make([]byte, 128)
						n, _, err := wrapper.ReadFrom(b)
						if err != nil || string(b[:n]) != string(message) {
							t.Fatalf("UDP payload: %q %v", b[:n], err)
						}
					}
					select {
					case command := <-commands:
						want := byte(1)
						if network == "udp" {
							want = 3
						}
						if command != want {
							t.Fatalf("wrong SOCKS command: %d", command)
						}
					case <-ctx.Done():
						t.Fatal("server bypassed payload router")
					}
				})
			}
		})
	}
}
