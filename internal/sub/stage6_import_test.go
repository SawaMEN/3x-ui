package sub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
)

// Import the actual service output with the pinned client engine, then use
// its parsed proxy (without rewriting credentials/transport) for TCP and UDP.
func TestStage6SubscriptionImports(t *testing.T) {
	binary := os.Getenv("HIDDIFY_TEST_BINARY")
	if binary == "" {
		t.Skip("requires pinned import-capable native client")
	}
	initSubDB(t)
	const subID = "stage6-import"
	const uuid = "11111111-1111-4111-8111-111111111111"
	const email = "import-client"
	port := anyTLSFreePort(t)
	clients := []model.Client{{ID: uuid, Email: email, Enable: true, SubID: subID}}
	settings, _ := json.Marshal(map[string]any{"clients": clients})
	ib := &model.Inbound{Enable: true, Protocol: model.VMESS, Tag: "import-vmess", Remark: "VPN :/#?&+ русский", Listen: "127.0.0.1", Port: port, Settings: string(settings), StreamSettings: `{"network":"tcp","security":"none"}`}
	db := database.GetDB()
	if e := db.Create(ib).Error; e != nil {
		t.Fatal(e)
	}
	rec := &model.ClientRecord{Email: email, UUID: uuid, SubID: subID, Enable: true}
	if e := db.Create(rec).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: ib.Id}).Error; e != nil {
		t.Fatal(e)
	}
	raw := map[string]any{"protocol": "vmess", "tag": ib.Tag, "listen": "127.0.0.1", "port": port, "settings": map[string]any{"clients": []any{map[string]any{"id": uuid, "email": email, "enable": true}}}, "streamSettings": map[string]any{"network": "tcp", "security": "none"}}
	server, e := hiddify.TranslateInbound(raw)
	if e != nil {
		t.Fatal(e)
	}
	serverLog := runAnyTLSCore(t, binary, map[string]any{"inbounds": []any{server}, "outbounds": []any{map[string]any{"type": "direct"}}})
	echo, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer echo.Close()
	go func() {
		for {
			c, e := echo.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	targetPort := echo.Addr().(*net.TCPAddr).Port
	udp, e := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", targetPort))
	if e != nil {
		t.Fatal(e)
	}
	defer udp.Close()
	go func() {
		buf := make([]byte, 65535)
		for {
			n, a, e := udp.ReadFrom(buf)
			if e != nil {
				return
			}
			_, _ = udp.WriteTo(buf[:n], a)
		}
	}()
	subSvc := NewSubService("")
	jsonSvc := NewSubJsonService("", "", "", "", subSvc)
	links, _, _, _, e := subSvc.GetSubs(subID, "127.0.0.1")
	if e != nil || len(links) != 1 {
		t.Fatal("raw links", links, e)
	}
	body := buildRawSubscriptionBody(links)
	type profile struct{ name, body string }
	profiles := []profile{{"uri", body}, {"base64", base64.StdEncoding.EncodeToString([]byte(body))}}
	for _, kind := range []string{"xray-json", "singbox-json", "hiddify-json", "clash-yaml", "clash-legacy-yaml"} {
		var output string
		switch kind {
		case "xray-json":
			output, _, e = jsonSvc.GetJson(subID, "127.0.0.1", false)
		case "singbox-json":
			output, _, e = jsonSvc.GetSingBoxJson(subID, "127.0.0.1", false)
		case "hiddify-json":
			output, _, e = jsonSvc.GetHiddifySingBoxJson(subID, "127.0.0.1")
		case "clash-yaml":
			output, _, e = NewSubClashService(false, "", subSvc).GetClash(subID, "127.0.0.1")
		case "clash-legacy-yaml":
			output, _, e = NewSubClashService(false, "", subSvc).GetClashLegacy(subID, "127.0.0.1")
		}
		if e != nil || strings.TrimSpace(output) == "" {
			t.Fatalf("%s export: %v", kind, e)
		}
		profiles = append(profiles, profile{kind, output})
	}
	for _, p := range profiles {
		t.Run(p.name, func(t *testing.T) {
			dir := t.TempDir()
			input, parsed := filepath.Join(dir, "profile.txt"), filepath.Join(dir, "parsed.json")
			if e := os.WriteFile(input, []byte(p.body), 0600); e != nil {
				t.Fatal(e)
			}
			cmd := exec.Command(binary, "parse", "-o", parsed, input)
			cmd.Dir = filepath.Dir(binary)
			if out, e := cmd.CombinedOutput(); e != nil {
				t.Fatalf("real client import: %v %s", e, out)
			}
			data, e := os.ReadFile(parsed)
			if e != nil {
				t.Fatal(e)
			}
			var cfg struct {
				Outbounds []map[string]any `json:"outbounds"`
			}
			if e := json.Unmarshal(data, &cfg); e != nil {
				t.Fatal(e)
			}
			var ob map[string]any
			for _, v := range cfg.Outbounds {
				if v["type"] == "vmess" {
					if ob != nil {
						t.Fatal("duplicated proxy")
					}
					ob = v
				}
			}
			if ob == nil || ob["uuid"] != uuid || ob["server"] != "127.0.0.1" || ob["server_port"] != float64(port) {
				t.Fatalf("import lost identity or endpoint: %v", ob)
			}
			ob["tag"] = "proxy"
			relay := anyTLSFreePort(t)
			clientLog := runAnyTLSCore(t, binary, map[string]any{"inbounds": []any{map[string]any{"type": "direct", "listen": "127.0.0.1", "listen_port": relay, "override_address": "127.0.0.1", "override_port": targetPort}}, "outbounds": []any{ob}})
			for _, network := range []string{"tcp", "udp"} {
				t.Run(network, func(t *testing.T) {
					var last error
					for until := time.Now().Add(8 * time.Second); time.Now().Before(until); {
						c, e := net.DialTimeout(network, net.JoinHostPort("127.0.0.1", strconv.Itoa(relay)), time.Second)
						if e == nil {
							_ = c.SetDeadline(time.Now().Add(time.Second))
							payload := []byte("stage6-import-echo")
							_, e = c.Write(payload)
							if e == nil {
								got := make([]byte, len(payload))
								if network == "tcp" {
									_, e = io.ReadFull(c, got)
								} else {
									var n int
									n, e = c.Read(got)
									if n != len(payload) {
										e = fmt.Errorf("short datagram %d", n)
									}
								}
								if e == nil && !bytes.Equal(got, payload) {
									e = fmt.Errorf("corrupted payload")
								}
							}
							_ = c.Close()
						}
						if e == nil {
							return
						}
						last = e
						time.Sleep(40 * time.Millisecond)
					}
					a, _ := os.ReadFile(serverLog)
					b, _ := os.ReadFile(clientLog)
					t.Fatalf("imported profile failed %s: %v server:%s client:%s", network, last, a, b)
				})
			}
		})
	}
}
