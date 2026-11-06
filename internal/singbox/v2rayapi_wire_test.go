package singbox

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	stats "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
)

// Use the stock protobuf server codec: this catches a private content subtype
// that a mocked ClientConn or a custom-codec server would incorrectly accept.
func TestV2RayStatsClientPinnedMethodUsesProtobuf(t *testing.T) {
	for _, method := range []string{v2rayStatsMethod, "/v2ray.core.app.stats.command.StatsService/QueryStats"} {
		t.Run(method, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer()
			reset := make(chan bool, 1)
			server.RegisterService(&grpc.ServiceDesc{
				ServiceName: strings.TrimSuffix(strings.TrimPrefix(method, "/"), "/QueryStats"),
				HandlerType: (*interface{})(nil),
				Methods: []grpc.MethodDesc{{MethodName: "QueryStats", Handler: func(_ any, ctx context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
					request := &stats.QueryStatsRequest{}
					if err := decode(request); err != nil {
						return nil, err
					}
					reset <- request.Reset_
					return &stats.QueryStatsResponse{Stat: []*stats.Stat{{Name: "user>>>alice>>>traffic>>>uplink", Value: 73}}}, nil
				}}},
			}, struct{}{})
			go server.Serve(listener)
			t.Cleanup(server.Stop)
			client := NewV2RayStatsClient(listener.Addr().String(), method)
			t.Cleanup(client.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			values, err := client.Query(ctx, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			if len(values) != 1 || values[0].Name != "user>>>alice>>>traffic>>>uplink" || values[0].Value != 73 || !<-reset {
				t.Fatalf("protobuf stats/reset contract lost: %#v", values)
			}
		})
	}
}
