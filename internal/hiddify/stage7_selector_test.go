package hiddify

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestStage7SelectorTraffic(t *testing.T) {
	root := os.Getenv("XUI_STAGE4_CORE_DIR")
	if root == "" {
		t.Skip("requires pinned native routing engines")
	}
	for _, engine := range []string{"owner", "upstream"} {
		t.Run(engine, func(t *testing.T) {
			t.Setenv("STAGE5_NATIVE_PROFILE", engine)
			binary := filepath.Join(root, "owner", "x-ui", "bin", "hiddify-core-linux-"+runtime.GOARCH)
			if engine == "upstream" {
				binary = filepath.Join(root, "upstream", "sing-box")
			}
			a, b := stage7Marker(t, 'A'), stage7Marker(t, 'B')
			in, api, dead := stage5Port(t), stage5Port(t), stage5Port(t)
			outs := []any{map[string]any{"type": "selector", "tag": "selection", "outbounds": []string{"A", "B", "all-dead"}, "default": "A"}}
			for _, v := range []struct {
				tag  string
				port int
			}{{"A", a}, {"B", b}, {"all-dead", dead}} {
				outs = append(outs, map[string]any{"type": "socks", "tag": v.tag, "server": "127.0.0.1", "server_port": v.port})
			}
			cfg := map[string]any{"inbounds": []any{map[string]any{"type": "socks", "tag": "in", "listen": "127.0.0.1", "listen_port": in}}, "outbounds": outs, "route": map[string]any{"final": "selection"}, "experimental": map[string]any{"clash_api": map[string]any{"external_controller": fmt.Sprintf("127.0.0.1:%d", api), "secret": "stage7-secret"}}}
			stage5Start(t, binary, t.TempDir(), "selector", cfg, in, true)
			client, err := singbox.NewSelectorClient(fmt.Sprintf("127.0.0.1:%d", api), "stage7-secret")
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			for _, target := range []string{"A", "B", "all-dead", "A"} {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				selected, err := client.Select(ctx, "selection", target)
				cancel()
				if err != nil || selected != target {
					t.Fatal(selected, err)
				}
				for _, network := range []string{"tcp", "udp"} {
					reply, err := stage7Exchange(net.JoinHostPort("127.0.0.1", fmt.Sprint(in)), "198.18.0.9", 443, network)
					if target == "all-dead" {
						if err == nil || len(reply) > 0 {
							t.Fatalf("dead upstream leaked %s payload %q", network, reply)
						}
						continue
					}
					if err != nil || !bytes.Equal(reply, append([]byte{target[0]}, []byte("route-payload")...)) {
						t.Fatalf("selected %s %s: %q %v", target, network, reply, err)
					}
				}
			}
		})
	}
}
