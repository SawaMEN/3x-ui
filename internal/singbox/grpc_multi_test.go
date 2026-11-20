package singbox

import "testing"

func TestXrayGRPCMultiModeCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name      string
		multiMode bool
		direction string
	}{
		{name: "Xray inbound TunMulti", multiMode: true, direction: "inbound"},
		{name: "Xray outbound TunMulti", multiMode: true, direction: "outbound"},
		{name: "standard Tun", direction: "inbound"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := map[string]any{"type": "grpc", "service_name": "service"}
			out := map[string]any{"tag": "in-54732-tcp", "transport": transport}
			stream := map[string]any{"grpcSettings": map[string]any{"multiMode": tc.multiMode}}
			if err := translateGRPCTransportCompatibility(out, stream, "in-54732-tcp", tc.direction); err != nil {
				t.Fatal(err)
			}
			if got := transport["multi_mode"] == true; got != tc.multiMode {
				t.Fatalf("multi_mode=%v, want %v: %v", got, tc.multiMode, transport)
			}
			if !tc.multiMode {
				if _, ok := transport["multi_mode"]; ok {
					t.Fatal("disabled multiMode should not appear in native transport")
				}
			}
		})
	}
}

func TestXrayGRPCMultiModeDoesNotBypassUnsupportedOptions(t *testing.T) {
	transport := map[string]any{"type": "grpc"}
	out := map[string]any{"transport": transport}
	stream := map[string]any{"grpcSettings": map[string]any{
		"multiMode": true, "authority": "other.example",
	}}
	if err := translateGRPCTransportCompatibility(out, stream, "grpc-in", "inbound"); err == nil {
		t.Fatal("unsupported gRPC authority unexpectedly accepted")
	}
}
