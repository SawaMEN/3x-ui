package hiddify

import (
	"reflect"
	"testing"
)

func TestKCPTranslationBothDirections(t *testing.T) {
	for _, inbound := range []bool{true, false} {
		raw := vlessInbound()
		object(raw, "streamSettings")["network"] = "kcp"
		object(raw, "streamSettings")["kcpSettings"] = map[string]any{"mtu": 1350, "tti": 20, "uplinkCapacity": 5, "downlinkCapacity": 20, "cwndMultiplier": 1, "maxSendingWindow": 2097152, "seed": "secret", "header": map[string]any{"type": "srtp"}}
		if !inbound {
			raw["settings"] = map[string]any{"address": "127.0.0.1", "port": 443, "id": "11111111-1111-4111-8111-111111111111"}
		}
		var out map[string]any
		var err error
		if inbound {
			out, err = TranslateInbound(raw)
		} else {
			out, err = TranslateOutbound(raw)
		}
		if err != nil {
			t.Fatal(err)
		}
		direction := "outbounds"
		if inbound {
			direction = "inbounds"
		}
		checkProtectionConfig(t, direction, out)
		transport := object(out, "transport")
		if transport["type"] != "kcp" || transport["seed"] != "secret" || transport["header"] != "srtp" || transport["legacy"] != true || transport["tti"] != uint32(20) {
			t.Fatalf("mKCP options lost: %#v", transport)
		}
	}
}

func TestDynamicBalancerTranslation(t *testing.T) {
	available := []map[string]any{{"protocol": "vless", "tag": "proxy-a"}, {"protocol": "vless", "tag": "proxy-b"}, {"protocol": "freedom", "tag": "backup"}}
	config := map[string]any{"observatory": map[string]any{"subjectSelector": []any{"proxy-"}, "probeURL": "http://127.0.0.1/probe", "probeInterval": "10s", "enableConcurrency": true}}
	for _, kind := range []string{"random", "roundRobin", "leastPing", "leastLoad"} {
		routing := map[string]any{"balancers": []any{map[string]any{"tag": "group", "selector": []any{"proxy-"}, "fallbackTag": "backup", "strategy": map[string]any{"type": kind}}}}
		out, err := TranslateBalancers(routing, config, available...)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 1 || !reflect.DeepEqual(out[0]["outbounds"], []string{"proxy-a", "proxy-b"}) {
			t.Fatalf("candidates lost: %#v", out)
		}
		options := object(out[0], "balancer")
		if options["fallback"] != "backup" || options["probe_url"] != "http://127.0.0.1/probe" || options["concurrent"] != true {
			t.Fatalf("dynamic options lost: %#v", options)
		}
	}
}

func TestDownloadExplicitModesPreserved(t *testing.T) {
	for _, mode := range []string{"auto", "packet-up", "stream-up", "stream-one"} {
		out, err := translateDownload(map[string]any{"address": "127.0.0.1", "port": 443, "network": "xhttp", "xhttpSettings": map[string]any{"path": "/down", "mode": mode}}, "download")
		if err != nil {
			t.Fatal(err)
		}
		if out["mode"] != mode {
			t.Fatal("download mode lost")
		}
	}
}

func TestNestedDownloadTranslation(t *testing.T) {
	leaf := map[string]any{"address": "127.0.0.1", "port": 443, "network": "xhttp", "xhttpSettings": map[string]any{"path": "/leaf"}}
	root := map[string]any{"address": "127.0.0.1", "port": 444, "network": "xhttp", "xhttpSettings": map[string]any{"downloadSettings": leaf}}
	out, err := translateDownload(root, "nested")
	if err != nil {
		t.Fatal(err)
	}
	checkProtectionConfig(t, "outbounds", map[string]any{"type": "vless", "tag": "nested-test", "server": "127.0.0.1", "server_port": 443, "uuid": "11111111-1111-4111-8111-111111111111", "transport": map[string]any{"type": "xhttp", "mode": "packet-up", "download": out}})
	if object(out, "download")["path"] != "/leaf" {
		t.Fatal(out)
	}
	for i := 0; i < 8; i++ {
		root = map[string]any{"address": "127.0.0.1", "port": 444, "network": "xhttp", "xhttpSettings": map[string]any{"downloadSettings": root}}
	}
	if _, err := translateDownload(root, "deep"); err == nil {
		t.Fatal("excessive depth accepted")
	}
}
